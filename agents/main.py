"""
Reveal DQ Analyser Agent — FastAPI + Claude + MySQL-over-MCP tool layer.

The agent inspects a workspace's MySQL warehouse and recommends Data Quality
rules. Data-set access is routed through a real MCP channel (see mcp_server.py /
mcp_client.py); if MCP cannot be established it transparently falls back to a
direct MySQL connection so the screen keeps working.

Flow for every question:
  1. Assess the COMPLETE metadata (all tables + their columns).
  2. Recommend DQ rules grounded in that metadata.
  3. Preview the impacted data and report the PERCENTAGE of impacted records
     for each recommended rule.

A dedicated "whole database" mode answers business questions like
"analyse the whole database and tell me where to focus" by profiling every
table and ranking the DQ hotspots by impact.
"""

from __future__ import annotations

import hashlib
import json
import os
import re
import threading
import time
from typing import Any, Optional

import pymysql
from dotenv import load_dotenv
from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel, Field

import etl_analysis
import etl_convert
import etl_graph
import voc_analysis
from mcp_client import McpBridge, McpUnavailable
from sql_guard import assert_readonly_select

load_dotenv(os.path.join(os.path.dirname(__file__), "..", ".env"))
load_dotenv()

ANTHROPIC_API_KEY = os.getenv("ANTHROPIC_API_KEY", "")
CLAUDE_MODEL = os.getenv("CLAUDE_MODEL", "claude-sonnet-4-20250514")
ALLOWED_MODELS = {
    "claude-sonnet-4-20250514": "Claude Sonnet 4 (default)",
    "claude-3-5-haiku-20241022": "Claude Haiku 3.5 (cheaper)",
    "claude-opus-4-20250514": "Claude Opus 4 (highest quality)",
}
DEFAULT_MYSQL = {
    "host": os.getenv("MYSQL_HOST", "127.0.0.1"),
    "port": int(os.getenv("MYSQL_PORT", "3306")),
    "user": os.getenv("MYSQL_USER", "root"),
    "password": os.getenv("MYSQL_PASSWORD", ""),
    "database": os.getenv("MYSQL_DATABASE", "RevealDQ"),
}
# MCP is the preferred data-access channel; set DQ_MCP_ENABLED=0 to force direct MySQL.
MCP_ENABLED = os.getenv("DQ_MCP_ENABLED", "1").strip().lower() not in {"0", "false", "no"}

# Metadata cache TTL (seconds) — schema changes rarely, so caching it makes
# interactive follow-up questions in a conversation far faster.
META_TTL = float(os.getenv("DQ_META_TTL", "120"))

# Profiling guard-rails for whole-database scans.
MAX_TABLES = int(os.getenv("DQ_MAX_TABLES", "60"))
MAX_COLS_PER_TABLE = int(os.getenv("DQ_MAX_COLS", "40"))
# A completeness "gap" is a partially-missing column: enough nulls to matter, but
# not so many that the column is clearly optional-by-design.
NULL_RATIO_THRESHOLD = float(os.getenv("DQ_NULL_THRESHOLD", "5.0"))  # percent (lower bound)
NULL_RATIO_UPPER = float(os.getenv("DQ_NULL_UPPER", "70.0"))  # percent (upper bound)
# Column names that are optional by nature — ignore them for generic completeness.
_OPTIONAL_COL = re.compile(
    r"(end|deactiv|deactivation|closed|cancel|cancelled|terminat|expir|expiry|"
    r"deleted|resolved|churn|closed_at|end_date|ended|inactive|discontinu)",
    re.IGNORECASE,
)

app = FastAPI(title="Reveal DQ Analyser Agent", version="0.2.0")
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
)


# ----------------------------------------------------------------------------
# Wire models
# ----------------------------------------------------------------------------
class ConnectionInfo(BaseModel):
    host: str = "127.0.0.1"
    port: int = 3306
    database: str = "RevealDQ"
    username: str = "root"
    password: str = ""
    # One-time id redeemed from the API. The chat body never carries the password.
    handoff: str = ""


class ExistingRule(BaseModel):
    ruleId: int
    ruleName: str
    dimension: str = ""
    status: str = ""
    businessDefinition: str = ""
    impactedTables: list[str] = Field(default_factory=list)
    checkSql: str = ""


class ChatRequest(BaseModel):
    message: str
    workspaceId: Optional[int] = None
    history: list[dict[str, str]] = Field(default_factory=list)
    connection: Optional[ConnectionInfo] = None
    existingRules: list[ExistingRule] = Field(default_factory=list)
    model: Optional[str] = None


class RuleImpact(BaseModel):
    impactedCount: int = 0
    totalCount: int = 0
    impactedPercent: float = 0.0


class RecommendedRule(BaseModel):
    ruleName: str
    dimension: str
    businessDefinition: str
    technicalDefinition: str
    impactedTables: list[str]
    impactedColumns: list[str]
    checkSql: str
    impact: Optional[RuleImpact] = None


class DataPreview(BaseModel):
    title: str = "Query preview"
    sql: str = ""
    columns: list[str] = Field(default_factory=list)
    rows: list[dict[str, Any]] = Field(default_factory=list)
    impact: Optional[RuleImpact] = None


class ChatResponse(BaseModel):
    answer: str
    recommendedRules: list[RecommendedRule] = Field(default_factory=list)
    toolTraces: list[dict[str, Any]] = Field(default_factory=list)
    dataPreview: Optional[DataPreview] = None
    modelUsed: str = CLAUDE_MODEL
    dataSource: str = "mysql"


# ----------------------------------------------------------------------------
# Data-source abstraction (MCP preferred, direct MySQL fallback)
# ----------------------------------------------------------------------------
_IDENT = re.compile(r"^[A-Za-z0-9_]+$")


def redeem_handoff(handoff_id: str) -> str:
    """Pull the warehouse password from the API. The id is single-use."""
    import urllib.error
    import urllib.request

    api = os.getenv("API_URL", "http://127.0.0.1:8080").rstrip("/")
    token = os.getenv("JWT_SECRET", "")
    req = urllib.request.Request(
        api + "/api/v1/internal/connection-handoff",
        data=json.dumps({"id": handoff_id}).encode(),
        headers={"Content-Type": "application/json", "X-Internal-Token": token},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=5) as resp:
            body = json.loads(resp.read().decode())
    except urllib.error.HTTPError as exc:
        raise RuntimeError(f"connection handoff failed ({exc.code})") from exc
    return str(body.get("password") or "")


def mysql_cfg(conn: Optional[ConnectionInfo]) -> dict:
    cfg = DEFAULT_MYSQL.copy()
    if conn:
        password = conn.password
        if conn.handoff and not password:
            password = redeem_handoff(conn.handoff)
        cfg.update(
            {
                "host": conn.host,
                "port": conn.port,
                "user": conn.username,
                "password": password,
                "database": conn.database,
            }
        )
    return cfg


def connect(conn: Optional[ConnectionInfo]):
    cfg = mysql_cfg(conn)
    return pymysql.connect(
        host=cfg["host"],
        port=int(cfg["port"]),
        user=cfg["user"],
        password=cfg["password"],
        database=cfg["database"],
        cursorclass=pymysql.cursors.DictCursor,
        autocommit=True,
    )


# Warm MCP bridges kept alive across requests, keyed by connection signature, so
# an interactive conversation does not pay the subprocess-spawn + MCP-handshake
# cost on every single message. Metadata is cached per connection with a TTL.
_BRIDGE_POOL: dict[str, "McpBridge"] = {}
_BRIDGE_LOCK = threading.Lock()
_META_CACHE: dict[str, tuple[float, dict]] = {}
_META_LOCK = threading.Lock()


def conn_signature(cfg: dict) -> str:
    raw = f"{cfg['host']}:{cfg['port']}/{cfg['database']}#{cfg['user']}"
    return hashlib.sha1(raw.encode()).hexdigest()[:16]


class DataSource:
    """Common surface used by all analysis routines."""

    kind = "mysql"

    def __init__(self) -> None:
        self.sig: str = ""
        self._tcount: dict[str, int] = {}  # per-request table-row-count cache

    def list_tables(self) -> list[str]:
        raise NotImplementedError

    def describe_table(self, table: str) -> list[dict]:
        raise NotImplementedError

    def _select(self, sql: str, limit: int = 50) -> list[dict]:
        raise NotImplementedError

    def run_select(self, sql: str, limit: int = 50) -> list[dict]:
        return self._select(sql, limit)

    def count(self, sql: str) -> int:
        inner = sql.strip().rstrip(";")
        rows = self._select(f"SELECT COUNT(*) AS _c FROM ({inner}) AS _dq_sub", limit=1)
        return int(rows[0]["_c"]) if rows else 0

    def table_count(self, table: str) -> int:
        if not _IDENT.match(table or ""):
            return 0
        if table in self._tcount:
            return self._tcount[table]
        rows = self._select(f"SELECT COUNT(*) AS _c FROM `{table}`", limit=1)
        val = int(rows[0]["_c"]) if rows else 0
        self._tcount[table] = val
        return val

    def close(self) -> None:
        pass


class MysqlDataSource(DataSource):
    kind = "mysql"

    def __init__(self, conn):
        super().__init__()
        self.db = conn

    def list_tables(self) -> list[str]:
        with self.db.cursor() as cur:
            cur.execute("SHOW TABLES")
            rows = cur.fetchall()
            if not rows:
                return []
            key = list(rows[0].keys())[0]
            return [r[key] for r in rows]

    def describe_table(self, table: str) -> list[dict]:
        if not _IDENT.match(table or ""):
            raise ValueError("invalid table name")
        with self.db.cursor() as cur:
            cur.execute(f"DESCRIBE `{table}`")
            return list(cur.fetchall())

    def _select(self, sql: str, limit: int = 50) -> list[dict]:
        cleaned = assert_readonly_select(sql)
        if " limit " not in cleaned.lower():
            cleaned = f"{cleaned} LIMIT {int(limit)}"
        with self.db.cursor() as cur:
            cur.execute(cleaned)
            return list(cur.fetchall())

    def close(self) -> None:
        try:
            self.db.close()
        except Exception:  # noqa: BLE001
            pass


class McpDataSource(DataSource):
    kind = "mcp"

    def __init__(self, bridge: McpBridge, pooled: bool = True):
        super().__init__()
        self.bridge = bridge
        self.pooled = pooled

    def list_tables(self) -> list[str]:
        res = self.bridge.call("list_tables")
        return res if isinstance(res, list) else []

    def describe_table(self, table: str) -> list[dict]:
        res = self.bridge.call("describe_table", {"table": table})
        if isinstance(res, dict) and res.get("error"):
            raise ValueError(res["error"])
        return res if isinstance(res, list) else []

    def _select(self, sql: str, limit: int = 50) -> list[dict]:
        res = self.bridge.call("run_select", {"sql": sql, "limit": int(limit)})
        if isinstance(res, dict) and res.get("error"):
            raise ValueError(res["error"])
        return res if isinstance(res, list) else []

    def close(self) -> None:
        # Pooled bridges are kept warm across requests; the pool owns their lifecycle.
        if self.pooled:
            return
        try:
            self.bridge.close()
        except Exception:  # noqa: BLE001
            pass


def _acquire_bridge(cfg: dict, sig: str, traces: list) -> McpBridge:
    """Get a warm pooled bridge for this connection, or start a new one."""
    env = {
        "MYSQL_HOST": str(cfg["host"]),
        "MYSQL_PORT": str(cfg["port"]),
        "MYSQL_USER": str(cfg["user"]),
        "MYSQL_PASSWORD": str(cfg["password"]),
        "MYSQL_DATABASE": str(cfg["database"]),
    }
    with _BRIDGE_LOCK:
        existing = _BRIDGE_POOL.get(sig)
        if existing is not None and existing.is_alive():
            traces.append(
                {"tool": "mcp.connect", "result": {"transport": "stdio", "reuse": True}}
            )
            return existing
        if existing is not None:
            try:
                existing.close()
            except Exception:  # noqa: BLE001
                pass
            _BRIDGE_POOL.pop(sig, None)
        bridge = McpBridge(env)
        bridge.start()
        tools = bridge.list_tool_names()
        _BRIDGE_POOL[sig] = bridge
        traces.append(
            {"tool": "mcp.connect", "result": {"transport": "stdio", "tools": tools, "reuse": False}}
        )
        return bridge


def build_source(conn: Optional[ConnectionInfo], traces: list) -> DataSource:
    """Prefer a warm MCP channel; fall back to a direct MySQL connection on any failure."""
    cfg = mysql_cfg(conn)
    sig = conn_signature(cfg)
    if MCP_ENABLED:
        try:
            bridge = _acquire_bridge(cfg, sig, traces)
            source: DataSource = McpDataSource(bridge, pooled=True)
            source.sig = sig
            return source
        except (McpUnavailable, Exception) as exc:  # noqa: BLE001
            traces.append(
                {"tool": "mcp.connect", "error": str(exc), "fallback": "direct-mysql"}
            )
    source = MysqlDataSource(connect(conn))
    source.sig = sig
    return source


# ----------------------------------------------------------------------------
# Metadata assessment + impact scoring
# ----------------------------------------------------------------------------
def assess_metadata(source: DataSource, traces: list) -> dict:
    """Assess the complete metadata: list every table and describe its columns.

    Results are cached per connection (TTL) so interactive follow-up questions do
    not re-run a full SHOW TABLES + DESCRIBE sweep every time.
    """
    sig = getattr(source, "sig", "") or ""
    if sig:
        with _META_LOCK:
            entry = _META_CACHE.get(sig)
        if entry and (time.time() - entry[0]) < META_TTL:
            traces.append(
                {"tool": "metadata", "result": "cache-hit", "tables": entry[1]["tables"]}
            )
            return entry[1]

    tables = source.list_tables()
    traces.append({"tool": "list_tables", "result": tables})
    schema: dict[str, list[dict]] = {}
    for table in tables[:MAX_TABLES]:
        try:
            cols = source.describe_table(table)
        except Exception as exc:  # noqa: BLE001
            cols = []
            traces.append({"tool": "describe_table", "args": {"table": table}, "error": str(exc)})
            continue
        schema[table] = cols
        traces.append(
            {
                "tool": "describe_table",
                "args": {"table": table},
                "columns": [c.get("Field") for c in cols],
            }
        )
    meta = {"tables": tables, "schema": schema}
    if sig:
        with _META_LOCK:
            _META_CACHE[sig] = (time.time(), meta)
    return meta


def summarize_schema(meta: dict) -> str:
    lines = []
    for table, cols in meta["schema"].items():
        col_desc = ", ".join(
            f"{c.get('Field')}:{c.get('Type')}" for c in cols[:MAX_COLS_PER_TABLE]
        )
        lines.append(f"- {table}({col_desc})")
    return "\n".join(lines) if lines else "(no tables found)"


def compute_impact(source: DataSource, rule: RecommendedRule) -> Optional[RuleImpact]:
    """Run the rule's check SQL as a COUNT and express it as a % of the base table."""
    if not rule.checkSql:
        return None
    try:
        impacted = source.count(rule.checkSql)
    except Exception:  # noqa: BLE001
        return None
    total = 0
    for table in rule.impactedTables or []:
        try:
            total = source.table_count(table)
        except Exception:  # noqa: BLE001
            total = 0
        if total:
            break
    pct = round((impacted / total) * 100, 2) if total else 0.0
    return RuleImpact(impactedCount=impacted, totalCount=total, impactedPercent=pct)


def finalize_rules(
    source: DataSource, rules: list[RecommendedRule], traces: list
) -> list[RecommendedRule]:
    for rule in rules:
        rule.impact = compute_impact(source, rule)
        if rule.impact:
            traces.append(
                {
                    "tool": "impact",
                    "rule": rule.ruleName,
                    "impacted": rule.impact.impactedCount,
                    "total": rule.impact.totalCount,
                    "percent": rule.impact.impactedPercent,
                }
            )
    rules.sort(
        key=lambda r: (r.impact.impactedPercent if r.impact else -1.0), reverse=True
    )
    return rules


def preview_from_rows(sql: str, rows: list[dict], title: str = "Query preview") -> DataPreview:
    columns = list(rows[0].keys()) if rows else []
    return DataPreview(title=title, sql=sql, columns=columns, rows=rows[:25])


def preview_for_rule(source: DataSource, rule: RecommendedRule) -> Optional[DataPreview]:
    """Preview a sample of the records impacted by a rule, carrying its impact %."""
    if not rule.checkSql:
        return None
    try:
        rows = source.run_select(rule.checkSql, limit=25)
    except Exception:  # noqa: BLE001
        return None
    preview = preview_from_rows(rule.checkSql, rows, f"Impacted records — {rule.ruleName}")
    preview.impact = rule.impact
    return preview


# ----------------------------------------------------------------------------
# Domain + generic rule generation
# ----------------------------------------------------------------------------
def domain_rules(source: DataSource, tables: list[str], traces: list) -> list[RecommendedRule]:
    """Telco warehouse heuristics (completeness / consistency / validity)."""
    rules: list[RecommendedRule] = []

    if "usage_cdr" in tables and "rated_usage" in tables:
        sql = (
            "SELECT c.cdr_id, c.usage_type, c.cdr_status "
            "FROM usage_cdr c LEFT JOIN rated_usage r ON r.cdr_id=c.cdr_id "
            "WHERE r.rated_usage_id IS NULL AND c.cdr_status<>'DUPLICATE'"
        )
        rules.append(
            RecommendedRule(
                ruleName="CDR without rated usage",
                dimension="COMPLETENESS",
                businessDefinition="Every mediated CDR that is not a duplicate must be rated before billing.",
                technicalDefinition="usage_cdr rows with no matching rated_usage.cdr_id (excluding DUPLICATE status).",
                impactedTables=["usage_cdr", "rated_usage"],
                impactedColumns=["usage_cdr.cdr_id", "rated_usage.cdr_id"],
                checkSql=sql,
            )
        )

    if "invoice" in tables and "invoice_line_item" in tables:
        sql = (
            "SELECT i.invoice_id, i.total_amount, COALESCE(SUM(li.amount),0) AS line_total "
            "FROM invoice i LEFT JOIN invoice_line_item li ON li.invoice_id=i.invoice_id "
            "GROUP BY i.invoice_id, i.total_amount "
            "HAVING ABS(i.total_amount - COALESCE(SUM(li.amount),0)) > 0.01"
        )
        rules.append(
            RecommendedRule(
                ruleName="Invoice balance mismatch",
                dimension="CONSISTENCY",
                businessDefinition="Invoice header amount must equal the sum of its line items.",
                technicalDefinition="ABS(invoice.total_amount - SUM(invoice_line_item.amount)) > 0.01",
                impactedTables=["invoice", "invoice_line_item"],
                impactedColumns=["invoice.total_amount", "invoice_line_item.amount"],
                checkSql=sql,
            )
        )

    if "service_subscription" in tables:
        sql = (
            "SELECT subscription_id, service_identifier FROM service_subscription "
            "WHERE service_identifier NOT REGEXP '^61[0-9]{9}$'"
        )
        rules.append(
            RecommendedRule(
                ruleName="Invalid MSISDN format",
                dimension="VALIDITY",
                businessDefinition="Service identifiers must be valid Australian MSISDNs.",
                technicalDefinition="service_identifier must match ^61[0-9]{9}$",
                impactedTables=["service_subscription"],
                impactedColumns=["service_subscription.service_identifier"],
                checkSql=sql,
            )
        )
    return rules


def generic_completeness_rules(
    source: DataSource, meta: dict, traces: list
) -> list[RecommendedRule]:
    """Profile null ratios across every table and flag the worst completeness gaps."""
    rules: list[RecommendedRule] = []
    for table, cols in meta["schema"].items():
        fields = [
            c.get("Field")
            for c in cols[:MAX_COLS_PER_TABLE]
            if c.get("Field") and _IDENT.match(c.get("Field"))
        ]
        if not fields:
            continue
        exprs = ["COUNT(*) AS _total"] + [
            f"SUM(CASE WHEN `{f}` IS NULL THEN 1 ELSE 0 END) AS `null__{f}`" for f in fields
        ]
        sql = f"SELECT {', '.join(exprs)} FROM `{table}`"
        try:
            rows = source.run_select(sql, limit=1)
        except Exception as exc:  # noqa: BLE001
            traces.append({"tool": "profile_nulls", "args": {"table": table}, "error": str(exc)})
            continue
        if not rows:
            continue
        row = rows[0]
        total = int(row.get("_total") or 0)
        if total == 0:
            continue
        # Pick the worst partially-missing column, ignoring optional-by-design fields
        # and columns that are almost entirely empty (likely optional, not a gap).
        worst_field, worst_nulls, worst_ratio = None, 0, 0.0
        for f in fields:
            if _OPTIONAL_COL.search(f):
                continue
            nulls = int(row.get(f"null__{f}") or 0)
            ratio = (nulls / total) * 100
            if ratio > NULL_RATIO_UPPER:
                continue
            if ratio > worst_ratio:
                worst_field, worst_nulls, worst_ratio = f, nulls, ratio
        if worst_field and worst_ratio >= NULL_RATIO_THRESHOLD:
            check_sql = f"SELECT * FROM `{table}` WHERE `{worst_field}` IS NULL"
            rules.append(
                RecommendedRule(
                    ruleName=f"Completeness gap: {table}.{worst_field}",
                    dimension="COMPLETENESS",
                    businessDefinition=(
                        f"'{worst_field}' in {table} is populated for critical reporting; "
                        "missing values weaken downstream trust."
                    ),
                    technicalDefinition=f"{table}.{worst_field} IS NULL",
                    impactedTables=[table],
                    impactedColumns=[f"{table}.{worst_field}"],
                    checkSql=check_sql,
                )
            )
    return rules


# ----------------------------------------------------------------------------
# Generic, column-aware rule generation (works for ANY table/schema)
# ----------------------------------------------------------------------------
_AMOUNT_TOK = (
    "amount", "amt", "price", "cost", "balance", "total", "fee", "charge", "rate",
    "salary", "revenue", "payment", "paid", "due", "qty", "quantity", "units", "credit", "debit",
)
_EMAIL_TOK = ("email", "e_mail", "mail")
_PHONE_TOK = ("msisdn", "phone", "mobile", "service_identifier", "telephone", "imsi", "contact_number")
_CREATED_TOK = (
    "created", "create_date", "load", "inserted", "txn", "transaction", "event",
    "start", "issue", "booking", "order_date", "record", "reg",
)
_REQUIRED_TOK = (
    "id", "name", "status", "code", "type", "email", "account", "customer",
    "amount", "date", "identifier", "number", "key", "ref", "msisdn",
)


def _col_type(c: dict) -> str:
    return (c.get("Type") or "").lower()


def _is_numeric(t: str) -> bool:
    return any(t.startswith(p) for p in (
        "int", "bigint", "smallint", "tinyint", "mediumint",
        "decimal", "numeric", "float", "double", "dec", "bit",
    ))


def _is_temporal(t: str) -> bool:
    return any(t.startswith(p) for p in ("date", "datetime", "timestamp", "time", "year"))


def _is_text(t: str) -> bool:
    return any(p in t for p in ("char", "text", "enum", "set"))


def _has_tok(name: str, tokens) -> bool:
    return any(tok in name for tok in tokens)


def _profile_nulls(source: DataSource, table: str, fields: list[str], traces: list) -> tuple[int, dict]:
    """One-shot null profile for a table: returns (total_rows, {field: null_count})."""
    if not (_IDENT.match(table) and fields):
        return 0, {}
    exprs = ["COUNT(*) AS _total"] + [
        f"SUM(CASE WHEN `{f}` IS NULL THEN 1 ELSE 0 END) AS `null__{f}`" for f in fields
    ]
    sql = f"SELECT {', '.join(exprs)} FROM `{table}`"
    try:
        rows = source.run_select(sql, limit=1)
    except Exception as exc:  # noqa: BLE001
        traces.append({"tool": "profile_nulls", "args": {"table": table}, "error": str(exc)})
        return 0, {}
    if not rows:
        return 0, {}
    row = rows[0]
    total = int(row.get("_total") or 0)
    nulls = {f: int(row.get(f"null__{f}") or 0) for f in fields}
    return total, nulls


def column_rules(
    source: DataSource, table: str, cols: list[dict], traces: list, cap: int = 10
) -> list[RecommendedRule]:
    """Infer DQ rules for a table purely from its column metadata + a data profile.

    Generic across schemas: uses column name/type/nullability/key to propose
    completeness, uniqueness and validity checks. This is what lets the agent
    respond to *the table the analyst named* instead of hardcoded domain rules.
    """
    if not _IDENT.match(table or ""):
        return []
    fields = [c for c in cols if c.get("Field") and _IDENT.match(c["Field"])]
    if not fields:
        return []

    total, null_counts = _profile_nulls(source, table, [c["Field"] for c in fields], traces)
    traces.append({
        "tool": "column_profile",
        "args": {"table": table},
        "columns": [c["Field"] for c in fields],
        "rows": total,
    })

    completeness: list[RecommendedRule] = []
    uniqueness: list[RecommendedRule] = []
    validity: list[RecommendedRule] = []

    for c in fields:
        name = c["Field"]
        lname = name.lower()
        t = _col_type(c)
        key = (c.get("Key") or "").upper()
        nulls = null_counts.get(name, 0)

        # --- COMPLETENESS: required-looking column that actually has NULLs ---
        required = key == "PRI" or _has_tok(lname, _REQUIRED_TOK)
        if nulls > 0 and required and not _OPTIONAL_COL.search(lname):
            completeness.append(RecommendedRule(
                ruleName=f"Completeness: {table}.{name}",
                dimension="COMPLETENESS",
                businessDefinition=f"`{name}` is a key attribute of {table} and should always be populated.",
                technicalDefinition=f"{table}.{name} IS NULL",
                impactedTables=[table],
                impactedColumns=[f"{table}.{name}"],
                checkSql=f"SELECT * FROM `{table}` WHERE `{name}` IS NULL",
            ))

        # --- UNIQUENESS: primary/unique keys, the table's own id, natural keys ---
        # NB: generic `*_id` columns are usually FOREIGN keys (legitimately
        # non-unique), so only the table's own identity column qualifies.
        singular = table[:-1] if table.lower().endswith("s") else table
        own_id = lname in ("id", f"{table.lower()}_id", f"{singular.lower()}_id")
        id_like = key in ("PRI", "UNI") or own_id \
            or _has_tok(lname, _EMAIL_TOK) or _has_tok(lname, _PHONE_TOK)
        if id_like:
            uniqueness.append(RecommendedRule(
                ruleName=f"Uniqueness: {table}.{name}",
                dimension="UNIQUENESS",
                businessDefinition=f"`{name}` should uniquely identify a row in {table}; duplicates indicate a data issue.",
                technicalDefinition=f"Duplicate values of {table}.{name}",
                impactedTables=[table],
                impactedColumns=[f"{table}.{name}"],
                checkSql=(
                    f"SELECT `{name}`, COUNT(*) AS dup_count FROM `{table}` "
                    f"WHERE `{name}` IS NOT NULL GROUP BY `{name}` HAVING COUNT(*) > 1"
                ),
            ))

        # --- VALIDITY: type/name-driven format & range checks ---
        if _has_tok(lname, _EMAIL_TOK) and _is_text(t):
            validity.append(RecommendedRule(
                ruleName=f"Validity: {table}.{name} email format",
                dimension="VALIDITY",
                businessDefinition=f"`{name}` must be a well-formed email address.",
                technicalDefinition=f"{table}.{name} not matching an email pattern",
                impactedTables=[table],
                impactedColumns=[f"{table}.{name}"],
                checkSql=(
                    f"SELECT * FROM `{table}` WHERE `{name}` IS NOT NULL AND `{name}` <> '' "
                    f"AND `{name}` NOT REGEXP '^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\\\\.[A-Za-z]{{2,}}$'"
                ),
            ))
        elif _has_tok(lname, _PHONE_TOK) and _is_text(t):
            validity.append(RecommendedRule(
                ruleName=f"Validity: {table}.{name} phone format",
                dimension="VALIDITY",
                businessDefinition=f"`{name}` must be a valid contact/service number (6–15 digits).",
                technicalDefinition=f"{table}.{name} not matching a phone-number pattern",
                impactedTables=[table],
                impactedColumns=[f"{table}.{name}"],
                checkSql=(
                    f"SELECT * FROM `{table}` WHERE `{name}` IS NOT NULL AND `{name}` <> '' "
                    f"AND `{name}` NOT REGEXP '^[+]?[0-9]{{6,15}}$'"
                ),
            ))
        elif _is_numeric(t) and _has_tok(lname, _AMOUNT_TOK):
            validity.append(RecommendedRule(
                ruleName=f"Validity: {table}.{name} non-negative",
                dimension="VALIDITY",
                businessDefinition=f"`{name}` represents a monetary/quantity value and should not be negative.",
                technicalDefinition=f"{table}.{name} < 0",
                impactedTables=[table],
                impactedColumns=[f"{table}.{name}"],
                checkSql=f"SELECT * FROM `{table}` WHERE `{name}` < 0",
            ))
        elif _is_temporal(t) and _has_tok(lname, _CREATED_TOK):
            validity.append(RecommendedRule(
                ruleName=f"Validity: {table}.{name} not in future",
                dimension="TIMELINESS",
                businessDefinition=f"`{name}` records when an event occurred and should not be in the future.",
                technicalDefinition=f"{table}.{name} > NOW()",
                impactedTables=[table],
                impactedColumns=[f"{table}.{name}"],
                checkSql=f"SELECT * FROM `{table}` WHERE `{name}` > NOW()",
            ))

    # Prioritise: completeness gaps, key uniqueness, then validity — capped.
    ordered = completeness + uniqueness + validity
    return ordered[:cap]


def build_candidate_rules(
    source: DataSource, meta: dict, target: Optional[str], traces: list
) -> list[RecommendedRule]:
    """Assemble domain + column-aware candidates for the table(s) in scope."""
    tables = meta["tables"]
    schema = meta["schema"]
    scope = [target] if target else tables[:MAX_TABLES]

    candidates: list[RecommendedRule] = []
    candidates += domain_rules(source, scope, traces)
    per_table_cap = 12 if target else 4
    for t in scope:
        cols = schema.get(t) or []
        if not cols:
            try:
                cols = source.describe_table(t)
            except Exception:  # noqa: BLE001
                cols = []
        candidates += column_rules(source, t, cols, traces, cap=per_table_cap)

    # De-duplicate by (dimension, checkSql).
    seen: set[tuple[str, str]] = set()
    unique: list[RecommendedRule] = []
    for r in candidates:
        sig = (r.dimension, r.checkSql.strip())
        if sig in seen:
            continue
        seen.add(sig)
        unique.append(r)
    return unique


# ----------------------------------------------------------------------------
# Whole-database "where to focus" analysis
# ----------------------------------------------------------------------------
_FOCUS_PATTERN = re.compile(
    r"(whole|entire|complete|full)\s+(data\s*base|database|db|schema|warehouse)"
    r"|all\s+(the\s+)?tables"
    r"|across\s+the\s+(database|db|schema|warehouse)"
    r"|where\s+(do|should)\s+i\s+focus"
    r"|where\s+to\s+focus"
    r"|scan\s+the\s+(database|db|schema|warehouse)"
    r"|analyse\s+the\s+(whole|entire|database|db|schema)"
    r"|analyze\s+the\s+(whole|entire|database|db|schema)",
    re.IGNORECASE,
)


def is_whole_db_request(message: str) -> bool:
    msg = (message or "").lower()
    if _FOCUS_PATTERN.search(msg):
        return True
    if "focus" in msg and any(k in msg for k in ("dq", "data quality", "quality", "issue")):
        return True
    return False


# The chat has two intents: (1) answer questions about the data, and
# (2) recommend DQ rules. Rules are only produced when the analyst asks for them.
_RULE_INTENT = re.compile(
    r"\b(recommend|suggest|propose|generate|create|draft|build|give\s+me)\b[^.]*\b(rule|rules|dq\s+rule|check|checks|control|controls)\b"
    r"|\b(rule|rules|dq\s+rule|dq\s+rules)\b[^.]*\b(recommend|suggest|propose|for|to)\b"
    r"|what\s+(rule|rules|dq\s+rule|dq\s+rules)"
    r"|which\s+(rule|rules)"
    r"|recommend\s+(some\s+)?(dq\s+)?rules?",
    re.IGNORECASE,
)


def wants_rules(message: str) -> bool:
    msg = (message or "").lower()
    if _RULE_INTENT.search(msg):
        return True
    verbs = ("recommend", "suggest", "propose", "create", "draft", "generate", "build", "give")
    # "recommend rules", etc.
    if "rule" in msg and any(v in msg for v in verbs):
        return True
    # "suggestions on DQ", "data quality checks", "quality suggestions", etc.
    dq_terms = ("dq", "data quality", "quality", "check", "control", "validation")
    if any(v in msg for v in verbs) and any(d in msg for d in dq_terms):
        return True
    if "suggestion" in msg and any(d in msg for d in dq_terms):
        return True
    return False


def whole_db_analyse(
    source: DataSource, message: str, traces: list, model: str
) -> ChatResponse:
    meta = assess_metadata(source, traces)
    tables = meta["tables"]

    candidates = build_candidate_rules(source, meta, None, traces)
    candidates += generic_completeness_rules(source, meta, traces)
    rules = finalize_rules(source, candidates, traces)

    # Build a ranked hotspot table for the data preview.
    hotspot_rows: list[dict[str, Any]] = []
    for rule in rules:
        if not rule.impact:
            continue
        hotspot_rows.append(
            {
                "focus_area": rule.ruleName,
                "dimension": rule.dimension,
                "table": ", ".join(rule.impactedTables),
                "impacted_records": rule.impact.impactedCount,
                "total_records": rule.impact.totalCount,
                "impacted_pct": f"{rule.impact.impactedPercent}%",
            }
        )

    flagged = [r for r in rules if r.impact and r.impact.impactedCount > 0]
    top = flagged[:5]
    answer_parts = [
        f"I connected via the **{source.kind.upper()}** channel and assessed the complete "
        f"metadata across **{len(tables)} table(s)**, then profiled the data for quality issues.",
        "",
    ]
    if top:
        answer_parts.append("**Where to focus (ranked by % of records impacted):**")
        for i, rule in enumerate(top, 1):
            im = rule.impact
            answer_parts.append(
                f"{i}. **{rule.ruleName}** ({rule.dimension}) — "
                f"{im.impactedPercent}% impacted ({im.impactedCount:,} of {im.totalCount:,} records) "
                f"on {', '.join(rule.impactedTables)}."
            )
        answer_parts.append("")
        answer_parts.append(
            "Each recommended rule on the right is ready to save to the Rules Engine. "
            "The preview below ranks every detected hotspot so you can prioritise remediation."
        )
    else:
        answer_parts.append(
            "Good news — I profiled every table and found no material DQ issues against the "
            "current heuristics. Ask about a specific domain (rating, billing, subscriptions) "
            "for a deeper drill-down."
        )

    preview = DataPreview(
        title=f"Database DQ hotspots — where to focus ({len(hotspot_rows)} flagged)",
        sql="",
        columns=[
            "focus_area",
            "dimension",
            "table",
            "impacted_records",
            "total_records",
            "impacted_pct",
        ]
        if hotspot_rows
        else [],
        rows=hotspot_rows[:25],
    )

    return ChatResponse(
        answer="\n".join(answer_parts),
        recommendedRules=rules,
        toolTraces=traces,
        dataPreview=preview if hotspot_rows else None,
        modelUsed=model,
        dataSource=source.kind,
    )


# ----------------------------------------------------------------------------
# Targeted offline analysis (no Claude key)
# ----------------------------------------------------------------------------
def offline_analyse(
    source: DataSource, message: str, traces: list, model: str
) -> ChatResponse:
    """Recommend DQ rules grounded in the actual columns of the table(s) in scope.

    - If the analyst names a table, focus on THAT table's columns.
    - Otherwise scan the whole database and surface the ranked hotspots.
    """
    meta = assess_metadata(source, traces)
    tables = meta["tables"]
    schema = meta["schema"]
    target = _match_table(message, tables)

    candidates = build_candidate_rules(source, meta, target, traces)
    rules = finalize_rules(source, candidates, traces)

    if target:
        # Named table → show every grounded candidate (0% impact = a clean guard).
        cols = [c.get("Field") for c in (schema.get(target) or [])]
        total = source.table_count(target)
        shown = rules[:12]
        flagged = [r for r in shown if r.impact and r.impact.impactedCount > 0]
        header = (
            f"I analysed `{target}` via the **{source.kind.upper()}** channel — "
            f"**{total:,} row(s)**, **{len(cols)} column(s)** ({', '.join(cols)}).\n\n"
            f"Based on the column metadata and a data profile, here are "
            f"**{len(shown)} recommended DQ rule(s)**"
            + (f" — {len(flagged)} already show issues" if flagged else "") + ":"
        )
        lines = [header, ""]
        for i, rule in enumerate(shown, 1):
            im = rule.impact
            impact_txt = (
                f"{im.impactedPercent}% impacted ({im.impactedCount:,}/{im.totalCount:,})"
                if im and im.totalCount else "no impact measured"
            )
            lines.append(f"{i}. **{rule.ruleName}** ({rule.dimension}) — {impact_txt}.")
        lines.append("")
        lines.append("Each rule on the right is ready to review and save to the Rules Engine.")
        preview = preview_for_rule(source, flagged[0]) if flagged else (
            preview_from_rows(f"SELECT * FROM `{target}` LIMIT 25",
                              _safe_sample(source, target), f"Sample of {target}")
        )
        return ChatResponse(
            answer="\n".join(lines),
            recommendedRules=shown,
            toolTraces=traces,
            dataPreview=preview,
            modelUsed=model,
            dataSource=source.kind,
        )

    # No table named → whole-database hotspot recommendation.
    flagged = [r for r in rules if r.impact and r.impact.impactedCount > 0]
    top = flagged[:10] if flagged else rules[:10]
    lines = [
        f"No specific table named, so I scanned the whole database via the "
        f"**{source.kind.upper()}** channel — **{len(tables)} table(s)** — and profiled "
        f"the data for quality issues.\n",
    ]
    if flagged:
        lines.append("**Recommended DQ rules (ranked by % of records impacted):**")
        for i, rule in enumerate(top, 1):
            im = rule.impact
            lines.append(
                f"{i}. **{rule.ruleName}** ({rule.dimension}) — "
                f"{im.impactedPercent}% impacted ({im.impactedCount:,}/{im.totalCount:,}) "
                f"on {', '.join(rule.impactedTables)}."
            )
        lines.append("")
        lines.append("Name a table (e.g. \"suggest DQ rules for customer\") for a deeper, column-level review.")
    else:
        lines.append(
            "I profiled every table and found no material issues against the current checks. "
            "Name a specific table for a deeper column-level review."
        )
    preview = preview_for_rule(source, top[0]) if top else None
    return ChatResponse(
        answer="\n".join(lines),
        recommendedRules=top,
        toolTraces=traces,
        dataPreview=preview,
        modelUsed=model,
        dataSource=source.kind,
    )


def _safe_sample(source: DataSource, table: str) -> list[dict]:
    try:
        return source.run_select(f"SELECT * FROM `{table}`", limit=25)
    except Exception:  # noqa: BLE001
        return []


# ----------------------------------------------------------------------------
# Conversational data Q&A (no rule recommendations)
# ----------------------------------------------------------------------------
def _match_table(message: str, tables: list[str]) -> Optional[str]:
    msg = (message or "").lower()
    # Longest table name first so 'invoice_line_item' wins over 'invoice'.
    for t in sorted(tables, key=len, reverse=True):
        if t.lower() in msg:
            return t
    return None


def offline_answer(source: DataSource, message: str, traces: list, model: str) -> ChatResponse:
    """Answer a business question about the data — WITHOUT recommending rules."""
    meta = assess_metadata(source, traces)
    tables = meta["tables"]
    schema = meta["schema"]
    target = _match_table(message, tables)
    preview: Optional[DataPreview] = None

    if target:
        total = source.table_count(target)
        cols = [c.get("Field") for c in (schema.get(target) or [])]
        try:
            rows = source.run_select(f"SELECT * FROM `{target}`", limit=25)
        except Exception:  # noqa: BLE001
            rows = []
        preview = preview_from_rows(f"SELECT * FROM `{target}` LIMIT 25", rows, f"Sample of {target}")
        answer = (
            f"Connected via **{source.kind.upper()}**. `{target}` holds **{total:,} row(s)** "
            f"across {len(cols)} column(s): {', '.join(cols)}.\n\n"
            f"A sample is shown in the preview. Ask me to **recommend DQ rules** for "
            f"`{target}` when you want quality checks proposed."
        )
    else:
        # General overview: table inventory with row counts.
        overview_rows = []
        for t in tables[:MAX_TABLES]:
            overview_rows.append({"table": t, "row_count": source.table_count(t)})
        overview_rows.sort(key=lambda r: r["row_count"], reverse=True)
        preview = DataPreview(
            title=f"Dataset overview — {len(tables)} table(s)",
            sql="",
            columns=["table", "row_count"] if overview_rows else [],
            rows=overview_rows[:25],
        )
        biggest = overview_rows[0]["table"] if overview_rows else "—"
        answer = (
            f"Connected via **{source.kind.upper()}** and assessed **{len(tables)} table(s)**. "
            f"The largest dataset is `{biggest}`. Ask about a specific table (e.g. its columns, "
            f"row counts, or a sample), or ask me to **recommend DQ rules** when you're ready."
        )

    return ChatResponse(
        answer=answer,
        recommendedRules=[],
        toolTraces=traces,
        dataPreview=preview,
        modelUsed=model,
        dataSource=source.kind,
    )


# ----------------------------------------------------------------------------
# Claude-driven analysis
# ----------------------------------------------------------------------------
SYSTEM_PROMPT = """You are the Reveal DQ Analyser Agent for an enterprise data quality platform.
You help business analysts explore MySQL warehouses (RevealDQ telco CRM/billing sample).

You serve TWO kinds of request:
A. DATA QUESTIONS — the analyst asks about the data (counts, samples, distributions, anomalies).
   Answer conversationally using the tools. DO NOT recommend rules unless explicitly asked;
   leave "recommended_rules" empty in this case.
B. RULE RECOMMENDATIONS — the analyst asks you to recommend/suggest DQ rules. Only then do you
   populate "recommended_rules". The current request's intent is stated at the end of the message.

WORKFLOW YOU MUST FOLLOW:
1. The complete metadata (all tables and their columns) is provided to you up front. Assess it.
2. Answer the analyst's actual question first.
3. When (and only when) rules are requested, recommend DQ rules grounded in that metadata.
4. For each rule provide a `checkSql` (a SELECT returning the FAILING rows). The platform will
   execute it to compute the percentage of impacted records automatically, so make the SQL a
   clean subquery-safe SELECT (no trailing semicolon).
5. When an existing-rules catalogue is included, cite those ruleIds. Do NOT propose a new rule
   that duplicates an ACTIVE, APPROVED, or DRAFT rule on the same table and dimension.
   Do not revive a RETIRED rule unless the analyst explicitly asks.

DQ dimensions you MUST use: ACCURACY, COMPLETENESS, CONSISTENCY, TIMELINESS, VALIDITY, UNIQUENESS.

When recommending a rule, always provide:
- business definition (plain language for stewardship)
- technical definition (how to measure)
- impacted tables and columns (list the primary table FIRST — its row count is the % base)
- SQL check (SELECT that returns failing rows)

You have tools (use them to inspect real data before finalising):
1) list_tables()
2) describe_table(table)
3) run_select(sql) — SELECT only

Respond with JSON only in this shape:
{
  "answer": "markdown-friendly explanation for the user",
  "tool_calls": [{"name":"run_select","sql":"SELECT ..."}],
  "recommended_rules": [
    {
      "ruleName": "...",
      "dimension": "COMPLETENESS",
      "businessDefinition": "...",
      "technicalDefinition": "...",
      "impactedTables": ["usage_cdr","rated_usage"],
      "impactedColumns": ["usage_cdr.cdr_id"],
      "checkSql": "SELECT ..."
    }
  ],
  "done": true|false
}

Set done=false while you still need tools. Set done=true when you can answer.
"""


def resolve_model(requested: Optional[str]) -> str:
    if requested and requested in ALLOWED_MODELS:
        return requested
    return CLAUDE_MODEL if CLAUDE_MODEL in ALLOWED_MODELS else next(iter(ALLOWED_MODELS))


def call_claude(messages: list[dict[str, str]], model: str) -> dict:
    if not ANTHROPIC_API_KEY:
        return {
            "answer": "Claude API key is not configured (set ANTHROPIC_API_KEY).",
            "tool_calls": [],
            "recommended_rules": [],
            "done": True,
            "_offline": True,
        }
    try:
        import anthropic
    except ImportError as e:
        raise HTTPException(500, f"anthropic package missing: {e}") from e

    client = anthropic.Anthropic(api_key=ANTHROPIC_API_KEY)
    resp = client.messages.create(
        model=model,
        max_tokens=4096,
        system=SYSTEM_PROMPT,
        messages=[{"role": m["role"], "content": m["content"]} for m in messages],
    )
    text = "".join(block.text for block in resp.content if hasattr(block, "text"))
    return parse_agent_json(text)


def parse_agent_json(text: str) -> dict:
    text = text.strip()
    fence = re.search(r"```(?:json)?\s*([\s\S]*?)```", text)
    if fence:
        text = fence.group(1).strip()
    try:
        return json.loads(text)
    except json.JSONDecodeError:
        start = text.find("{")
        end = text.rfind("}")
        if start >= 0 and end > start:
            return json.loads(text[start : end + 1])
        return {"answer": text, "tool_calls": [], "recommended_rules": [], "done": True}


def catalogue_block(rules: list[ExistingRule]) -> str:
    """Compact catalogue injected so the model cites saved rules instead of cloning them."""
    if not rules:
        return ""
    lines = ["[Existing DQ rules in this project — cite ruleId; do not duplicate ACTIVE, APPROVED, or DRAFT]"]
    for r in rules:
        tables = ", ".join(r.impactedTables) or "—"
        lines.append(
            f"- #{r.ruleId} {r.status} {r.dimension} {r.ruleName} | tables: {tables} | {r.businessDefinition}"
        )
    return "\n".join(lines) + "\n\n"


def claude_analyse(
    source: DataSource, req: ChatRequest, traces: list, model: str, want_rules: bool
) -> ChatResponse:
    # Step 1: assess the complete metadata and hand it to the model up front.
    meta = assess_metadata(source, traces)
    schema_summary = summarize_schema(meta)
    target = _match_table(req.message, meta["tables"])

    if want_rules:
        focus = (
            f" Focus specifically on the `{target}` table and its columns."
            if target else
            " No specific table was named — review the whole database and recommend rules for the tables with the biggest data-quality gaps."
        )
        directive = (
            "Intent: RULE RECOMMENDATIONS. Recommend DQ rules with a subquery-safe checkSql for "
            "each; the platform computes impacted-record percentages from your checkSql."
            + focus
        )
    else:
        directive = (
            "Intent: DATA QUESTION. Answer the analyst's question about the data using the tools. "
            "Do NOT recommend rules; leave recommended_rules empty unless the analyst asks for rules."
        )

    messages: list[dict[str, str]] = list(req.history or [])
    messages.append(
        {
            "role": "user",
            "content": (
                f"{req.message}\n\n"
                f"{catalogue_block(req.existingRules)}"
                f"[Complete metadata already assessed — {len(meta['tables'])} tables]\n"
                f"{schema_summary}\n\n"
                f"{directive}"
            ),
        }
    )

    last_preview: Optional[DataPreview] = None
    parsed: dict = {}
    for _ in range(6):
        parsed = call_claude(messages, model)
        if parsed.get("_offline"):
            return offline_analyse(source, req.message, traces, model)

        tool_calls = parsed.get("tool_calls") or []
        if parsed.get("done") and not tool_calls:
            break

        tool_results = []
        for call in tool_calls:
            name = call.get("name")
            try:
                if name == "list_tables":
                    result = source.list_tables()
                elif name == "describe_table":
                    result = source.describe_table(call["table"])
                elif name == "run_select":
                    result = source.run_select(call["sql"])
                    if isinstance(result, list):
                        last_preview = preview_from_rows(call.get("sql", ""), result)
                else:
                    result = {"error": f"unknown tool {name}"}
            except Exception as ex:  # noqa: BLE001
                result = {"error": str(ex)}
            traces.append({"tool": name, "args": call, "resultPreview": str(result)[:1000]})
            tool_results.append({"tool": name, "result": result})

        messages.append({"role": "assistant", "content": json.dumps(parsed)})
        messages.append(
            {
                "role": "user",
                "content": "Tool results:\n" + json.dumps(tool_results, default=str)[:12000],
            }
        )

    rules = finalize_rules(
        source,
        [RecommendedRule(**r) for r in (parsed.get("recommended_rules") or [])],
        traces,
    )
    # For rule requests, prefer a preview of the top rule's failing records with its
    # impact %. For plain data questions, keep whatever the model previewed.
    if rules:
        preview = preview_for_rule(source, rules[0]) or last_preview
    else:
        preview = last_preview
    return ChatResponse(
        answer=parsed.get("answer") or "Analysis complete.",
        recommendedRules=rules,
        toolTraces=traces,
        dataPreview=preview,
        modelUsed=model,
        dataSource=source.kind,
    )


# ----------------------------------------------------------------------------
# HTTP endpoints
# ----------------------------------------------------------------------------
@app.get("/health")
def health():
    return {
        "status": "ok",
        "service": "reveal-dq-agent",
        "claudeConfigured": bool(ANTHROPIC_API_KEY),
        "mcpEnabled": MCP_ENABLED,
        "models": [{"id": k, "label": v} for k, v in ALLOWED_MODELS.items()],
        "defaultModel": resolve_model(None),
    }


# ----------------------------------------------------------------------------
# ETL / SQL Analyser endpoint
# ----------------------------------------------------------------------------
class EtlMetric(BaseModel):
    label: str
    value: str
    hint: str = ""
    tone: str = "info"


class EtlStep(BaseModel):
    index: int
    name: str
    type: str
    detail: str = ""
    kind: str = "transform"


class EtlAnalyzeRequest(BaseModel):
    fileName: str = ""
    fileType: str = "auto"  # sql | informatica_xml | auto
    content: str = ""
    message: Optional[str] = None
    history: list[dict[str, str]] = Field(default_factory=list)
    model: Optional[str] = None


class EtlAnalyzeResponse(BaseModel):
    mode: str  # analysis | answer
    answer: str
    summaryMarkdown: str = ""
    metrics: list[EtlMetric] = Field(default_factory=list)
    steps: list[EtlStep] = Field(default_factory=list)
    graph: dict = Field(default_factory=dict)
    fileType: str = "sql"
    modelUsed: str = CLAUDE_MODEL


class EtlNodeRef(BaseModel):
    id: str = ""
    label: str = ""
    ntype: str = ""
    kind: str = ""
    upstream: list[str] = Field(default_factory=list)
    downstream: list[str] = Field(default_factory=list)


class EtlExplainRequest(BaseModel):
    fileName: str = ""
    fileType: str = "auto"
    content: str = ""
    node: EtlNodeRef
    model: Optional[str] = None


class EtlExplainResponse(BaseModel):
    explanation: str
    modelUsed: str = CLAUDE_MODEL
    source: str = "heuristic"  # heuristic | llm


ETL_SYSTEM_PROMPT = """You are the Reveal DQ ETL Analyser Agent. You help data engineers understand
SQL scripts and Informatica PowerCenter (XML) ETL jobs. Answer the user's question about the provided
file concisely and precisely in markdown. Ground every statement in the file content and the parsed
facts you are given. If something is not present in the file, say so. Do not invent tables or steps.
"""


def etl_followup(req: EtlAnalyzeRequest, analysis: dict, model: str) -> str:
    if not ANTHROPIC_API_KEY:
        return etl_analysis.heuristic_answer(req.message or "", analysis, req.content)
    try:
        import anthropic
    except ImportError:
        return etl_analysis.heuristic_answer(req.message or "", analysis, req.content)
    try:
        client = anthropic.Anthropic(api_key=ANTHROPIC_API_KEY)
        context = (
            f"File: {req.fileName} ({analysis.get('fileType')})\n"
            f"Parsed facts: {json.dumps(analysis.get('facts', {}), default=str)[:4000]}\n\n"
            f"File content (truncated):\n{req.content[:12000]}"
        )
        messages = list(req.history or [])
        messages.append({"role": "user", "content": f"{context}\n\nQuestion: {req.message}"})
        resp = client.messages.create(
            model=model, max_tokens=1500, system=ETL_SYSTEM_PROMPT,
            messages=[{"role": m["role"], "content": m["content"]} for m in messages],
        )
        return "".join(b.text for b in resp.content if hasattr(b, "text")) or ""
    except Exception:  # noqa: BLE001 - fall back to deterministic answer
        return etl_analysis.heuristic_answer(req.message or "", analysis, req.content)


@app.post("/etl/analyze", response_model=EtlAnalyzeResponse)
def etl_analyze(req: EtlAnalyzeRequest):
    model = resolve_model(req.model)
    if not (req.content or "").strip():
        raise HTTPException(400, "no file content provided")

    analysis = etl_analysis.analyze(req.fileName, req.fileType, req.content)
    metrics = [EtlMetric(**m) for m in analysis.get("metrics", [])]
    steps = [EtlStep(**{k: s[k] for k in ("index", "name", "type", "detail", "kind")}) for s in analysis.get("steps", [])]
    graph = etl_graph.build_graph(analysis)

    is_followup = bool(req.message and req.message.strip())
    if is_followup:
        answer = etl_followup(req, analysis, model)
        mode = "answer"
    else:
        label = "Informatica ETL job" if analysis["fileType"] == "informatica_xml" else "SQL script"
        name = req.fileName or "the uploaded file"
        answer = (
            f"I analysed **{name}** as an {label}. Below is the impact story (metrics), a "
            f"step-by-step flow, and a detailed activity breakdown. Ask me anything about it — "
            f"e.g. its sources/targets, the transformations, or where the complexity comes from."
        )
        mode = "analysis"

    return EtlAnalyzeResponse(
        mode=mode,
        answer=answer,
        summaryMarkdown=analysis.get("summaryMarkdown", ""),
        metrics=metrics,
        steps=steps,
        graph=graph,
        fileType=analysis["fileType"],
        modelUsed=model,
    )


@app.post("/etl/explain-node", response_model=EtlExplainResponse)
def etl_explain_node(req: EtlExplainRequest):
    """LLM Reasoning agent: explain a single graph node in context."""
    model = resolve_model(req.model)
    ftype = etl_analysis.detect_file_type(req.fileName, req.content, req.fileType)
    node = req.node
    note = etl_analysis.type_note(ftype, node.ntype)

    conn = []
    if node.upstream:
        conn.append("receives data from " + ", ".join(node.upstream))
    if node.downstream:
        conn.append("feeds " + ", ".join(node.downstream))
    heuristic = f"**{node.label}** ({node.ntype}) {note}." + (
        " It " + "; ".join(conn) + "." if conn else ""
    )

    if not ANTHROPIC_API_KEY:
        return EtlExplainResponse(explanation=heuristic, modelUsed=model, source="heuristic")
    try:
        import anthropic

        client = anthropic.Anthropic(api_key=ANTHROPIC_API_KEY)
        prompt = (
            f"You are explaining one step of an ETL/SQL pipeline to a data engineer.\n"
            f"File type: {ftype}\n"
            f"Step: {node.label} (type: {node.ntype}, role: {node.kind})\n"
            f"Upstream: {', '.join(node.upstream) or 'none'}\n"
            f"Downstream: {', '.join(node.downstream) or 'none'}\n\n"
            f"Relevant file content (truncated):\n{req.content[:10000]}\n\n"
            f"In 2-4 concise sentences, explain what this step does, why it exists, and any "
            f"data-quality or performance considerations. Ground it in the file. Use markdown."
        )
        resp = client.messages.create(
            model=model, max_tokens=600, system=ETL_SYSTEM_PROMPT,
            messages=[{"role": "user", "content": prompt}],
        )
        text = "".join(b.text for b in resp.content if hasattr(b, "text")).strip()
        return EtlExplainResponse(explanation=text or heuristic, modelUsed=model,
                                  source="llm" if text else "heuristic")
    except Exception:  # noqa: BLE001
        return EtlExplainResponse(explanation=heuristic, modelUsed=model, source="heuristic")


# ----------------------------------------------------------------------------
# ETL Code Conversion Agent endpoints
# ----------------------------------------------------------------------------
class ConvertAnalyzeRequest(BaseModel):
    fileName: str = ""
    content: str = ""


class ConvertAnalyzeResponse(BaseModel):
    source: dict = Field(default_factory=dict)
    summaryMarkdown: str = ""
    planMarkdown: str = ""
    metrics: list[EtlMetric] = Field(default_factory=list)
    steps: list[EtlStep] = Field(default_factory=list)


class ConvertRunRequest(BaseModel):
    fileName: str = ""
    content: str = ""
    sourceType: str = ""
    target: str = "databricks_sql"  # databricks_sql | pyspark
    model: Optional[str] = None


class ConvertRunResponse(BaseModel):
    code: str = ""
    language: str = "sql"
    notes: list[str] = Field(default_factory=list)
    validation: dict = Field(default_factory=dict)
    engine: str = "heuristic"  # llm | heuristic | lakebridge
    modelUsed: str = ""


CONVERT_SYSTEM_PROMPT = """You are an expert data engineer migrating legacy ETL and SQL to Databricks.
Convert the provided source (Oracle/T-SQL/MySQL stored procedures, or Informatica/SSIS ETL) into the
requested target (Databricks SQL or PySpark). Rules:
- Produce a faithful 1:1 conversion; preserve object/column names.
- Create target objects under the `migration_demo` catalog.
- Map dialect-specific functions to Databricks equivalents.
- Return ONLY the converted code inside a single fenced code block, no prose before/after.
"""


def _extract_code_block(text: str) -> str:
    m = re.search(r"```[a-zA-Z]*\n(.*?)```", text, re.DOTALL)
    return m.group(1).strip() if m else text.strip()


@app.post("/convert/analyze", response_model=ConvertAnalyzeResponse)
def convert_analyze(req: ConvertAnalyzeRequest):
    if not (req.content or "").strip():
        raise HTTPException(400, "no file content provided")
    a = etl_convert.analyze(req.fileName, req.content)
    return ConvertAnalyzeResponse(
        source=a["source"],
        summaryMarkdown=a["summaryMarkdown"],
        planMarkdown=a["planMarkdown"],
        metrics=[EtlMetric(**m) for m in a.get("metrics", [])],
        steps=[EtlStep(**{k: s[k] for k in ("index", "name", "type", "detail", "kind")}) for s in a.get("steps", [])],
    )


@app.post("/convert/run", response_model=ConvertRunResponse)
def convert_run(req: ConvertRunRequest):
    if not (req.content or "").strip():
        raise HTTPException(400, "no file content provided")
    source_type = req.sourceType or etl_convert.detect_source(req.fileName, req.content)["sourceType"]
    target = req.target if req.target in ("databricks_sql", "pyspark") else "databricks_sql"

    # Lakebridge is a future, fully-Databricks-integrated path.
    if (req.model or "").lower() == "lakebridge":
        lang = "python" if target == "pyspark" else "sql"
        placeholder = (
            f"-- Lakebridge conversion is the next step and will be fully integrated with Databricks.\n"
            f"-- Target catalog: {etl_convert.TARGET_CATALOG}\n"
            f"-- Source: {source_type}\n"
        )
        return ConvertRunResponse(
            code=placeholder, language=lang,
            notes=["Lakebridge agent selected — integration with Databricks is planned for the next phase."],
            validation={"ok": True, "issues": [], "checkedWith": "n/a"},
            engine="lakebridge", modelUsed="lakebridge",
        )

    engine = "heuristic"
    model = resolve_model(req.model)
    result = None
    if ANTHROPIC_API_KEY:
        try:
            import anthropic

            client = anthropic.Anthropic(api_key=ANTHROPIC_API_KEY)
            tgt_label = "PySpark" if target == "pyspark" else "Databricks SQL"
            prompt = (
                f"Source dialect/tool: {source_type}\n"
                f"Target: {tgt_label}\n"
                f"Target catalog: {etl_convert.TARGET_CATALOG}\n\n"
                f"Source code:\n```\n{req.content[:16000]}\n```"
            )
            resp = client.messages.create(
                model=model, max_tokens=4096, system=CONVERT_SYSTEM_PROMPT,
                messages=[{"role": "user", "content": prompt}],
            )
            text = "".join(b.text for b in resp.content if hasattr(b, "text"))
            code = _extract_code_block(text)
            if code:
                result = {"code": code, "language": "python" if target == "pyspark" else "sql",
                          "notes": ["Converted with " + ALLOWED_MODELS.get(model, model) + "."]}
                engine = "llm"
        except Exception as exc:  # noqa: BLE001 - fall back to heuristic
            result = None

    if result is None:
        result = etl_convert.heuristic_convert(req.content, source_type, target)
        model = ""

    validation = etl_convert.validate_code(result["code"], result["language"])
    return ConvertRunResponse(
        code=result["code"], language=result["language"], notes=result.get("notes", []),
        validation=validation, engine=engine, modelUsed=model,
    )


# ---------------------------------------------------------------------------
# Reveal VoC — agentic analytics (extraction is done by the Go automation)
# ---------------------------------------------------------------------------


class VocAnalyzeRequest(BaseModel):
    company: dict[str, Any] = Field(default_factory=dict)
    items: list[dict[str, Any]] = Field(default_factory=list)
    model: Optional[str] = None


class VocActionsRequest(BaseModel):
    company: dict[str, Any] = Field(default_factory=dict)
    signals: list[dict[str, Any]] = Field(default_factory=list)
    positiveTotal: int = 0
    negativeTotal: int = 0
    model: Optional[str] = None


class VocAskRequest(BaseModel):
    question: str
    stats: dict[str, Any] = Field(default_factory=dict)
    signals: list[dict[str, Any]] = Field(default_factory=list)
    actions: list[dict[str, Any]] = Field(default_factory=list)
    model: Optional[str] = None


@app.post("/voc/analyze")
def voc_analyze(req: VocAnalyzeRequest):
    """Sentiment / category / themes / urgency per LinkedIn signal."""
    if len(req.items) > 100:
        raise HTTPException(400, "send at most 100 items per batch")
    # Classification is high-volume; default to the cheaper model unless asked.
    model = resolve_model(req.model or os.getenv("CLAUDE_MODEL_CHEAP"))
    return voc_analysis.analyse_items(req.company, req.items, model)


@app.post("/voc/actions")
def voc_actions(req: VocActionsRequest):
    """Group analysed signals into de-duplicated, prioritised actions."""
    model = resolve_model(req.model)
    return voc_analysis.propose_actions(req.company, req.signals, req.positiveTotal, req.negativeTotal, model)


@app.post("/voc/ask")
def voc_ask(req: VocAskRequest):
    """Conversational VoC analyst over the project's stored picture."""
    model = resolve_model(req.model)
    return voc_analysis.answer_question(req.question, req.stats, req.signals, req.actions, model)


# ---------------------------------------------------------------------------
# Deterministic tools hosted alongside the agents (no LLM involved)
# ---------------------------------------------------------------------------

class LinkedInScrapeRequest(BaseModel):
    handle: str = ""
    company: str = ""
    hashtags: list[str] = Field(default_factory=list)
    dateFrom: Optional[str] = None  # ISO-8601; default = dateTo - 5 days
    dateTo: Optional[str] = None    # ISO-8601; default = now
    maxPosts: int = 25
    includeCompanyPosts: bool = True
    includeDiscovery: bool = True


_scrape_lock = threading.Lock()


@app.post("/tools/linkedin/scrape")
def linkedin_scrape(req: LinkedInScrapeRequest):
    """
    Playwright scrape of public LinkedIn pages for one company inside a date
    window. Pure automation — the Go sync engine calls this, then hands the
    stored items to the VoC agent for analysis.
    """
    import linkedin_scraper  # lazy: pulls in playwright

    def _dt(s: Optional[str]):
        if not s:
            return None
        d = linkedin_scraper.parse_iso(s)
        if d is None:
            raise HTTPException(400, f"bad date {s!r}; expected ISO-8601")
        return d

    if not req.handle and not req.company:
        raise HTTPException(400, "handle or company is required")
    if not _scrape_lock.acquire(timeout=1):
        raise HTTPException(409, "a LinkedIn scrape is already running; try again shortly")
    try:
        return linkedin_scraper.scrape(
            req.handle, req.company, req.hashtags, _dt(req.dateFrom), _dt(req.dateTo),
            max_posts=req.maxPosts, include_company_posts=req.includeCompanyPosts, include_discovery=req.includeDiscovery,
        )
    except HTTPException:
        raise
    except Exception as e:  # noqa: BLE001
        msg = str(e)
        if "Executable doesn't exist" in msg or "playwright install" in msg:
            raise HTTPException(500, "Chromium for Playwright is not installed — run: agents/.venv/bin/python -m playwright install chromium") from e
        raise HTTPException(502, f"scrape failed: {msg[:300]}") from e
    finally:
        _scrape_lock.release()


@app.post("/chat", response_model=ChatResponse)
def chat(req: ChatRequest):
    model = resolve_model(req.model)
    traces: list[dict[str, Any]] = []
    try:
        source = build_source(req.connection, traces)
    except Exception as e:  # noqa: BLE001
        raise HTTPException(502, f"cannot connect to data source: {e}") from e

    try:
        # "Analyse the whole database / where to focus" → hotspot scan + rules.
        if is_whole_db_request(req.message):
            return whole_db_analyse(source, req.message, traces, model)

        want_rules = wants_rules(req.message)
        if not ANTHROPIC_API_KEY:
            # Separate the two intents: recommend rules only when asked, otherwise
            # answer the analyst's data question conversationally.
            if want_rules:
                return offline_analyse(source, req.message, traces, model)
            return offline_answer(source, req.message, traces, model)
        return claude_analyse(source, req, traces, model, want_rules)
    finally:
        source.close()


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(
        "main:app",
        host="0.0.0.0",
        port=int(os.getenv("AGENT_PORT", "8090")),
        reload=False,
    )
