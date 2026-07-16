"""
Reveal DQ Analyser Agent — FastAPI + Claude + MySQL tool layer.
Uses direct MySQL queries (MCP-compatible tool surface) so the agent can
inspect schema and data without requiring a separate MCP process at runtime.
Cursor MCP config in ../mcp still documents the MySQL MCP server for IDE use.
"""

from __future__ import annotations

import json
import os
import re
from typing import Any, Optional

import pymysql
from dotenv import load_dotenv
from fastapi import FastAPI, HTTPException
from fastapi.middleware.cors import CORSMiddleware
from pydantic import BaseModel, Field

load_dotenv(os.path.join(os.path.dirname(__file__), "..", ".env"))
load_dotenv()

ANTHROPIC_API_KEY = os.getenv("ANTHROPIC_API_KEY", "")
CLAUDE_MODEL = os.getenv("CLAUDE_MODEL", "claude-sonnet-4-20250514")
ALLOWED_MODELS = {
    "claude-sonnet-4-20250514": "Claude Sonnet 4 (default)",
    "claude-3-5-haiku-20241022": "Claude Haiku 3.5 (cheaper)",
}
DEFAULT_MYSQL = {
    "host": os.getenv("MYSQL_HOST", "127.0.0.1"),
    "port": int(os.getenv("MYSQL_PORT", "3306")),
    "user": os.getenv("MYSQL_USER", "root"),
    "password": os.getenv("MYSQL_PASSWORD", ""),
    "database": os.getenv("MYSQL_DATABASE", "RevealDQ"),
}

app = FastAPI(title="Reveal DQ Analyser Agent", version="0.1.0")
app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_methods=["*"],
    allow_headers=["*"],
)


class ConnectionInfo(BaseModel):
    host: str = "127.0.0.1"
    port: int = 3306
    database: str = "RevealDQ"
    username: str = "root"
    password: str = ""


class ChatRequest(BaseModel):
    message: str
    workspaceId: Optional[int] = None
    history: list[dict[str, str]] = Field(default_factory=list)
    connection: Optional[ConnectionInfo] = None
    model: Optional[str] = None


class RecommendedRule(BaseModel):
    ruleName: str
    dimension: str
    businessDefinition: str
    technicalDefinition: str
    impactedTables: list[str]
    impactedColumns: list[str]
    checkSql: str


class DataPreview(BaseModel):
    title: str = "Query preview"
    sql: str = ""
    columns: list[str] = Field(default_factory=list)
    rows: list[dict[str, Any]] = Field(default_factory=list)


class ChatResponse(BaseModel):
    answer: str
    recommendedRules: list[RecommendedRule] = Field(default_factory=list)
    toolTraces: list[dict[str, Any]] = Field(default_factory=list)
    dataPreview: Optional[DataPreview] = None
    modelUsed: str = CLAUDE_MODEL


def connect(conn: Optional[ConnectionInfo]):
    cfg = DEFAULT_MYSQL.copy()
    if conn:
        cfg.update(
            {
                "host": conn.host,
                "port": conn.port,
                "user": conn.username,
                "password": conn.password,
                "database": conn.database,
            }
        )
    return pymysql.connect(
        host=cfg["host"],
        port=int(cfg["port"]),
        user=cfg["user"],
        password=cfg["password"],
        database=cfg["database"],
        cursorclass=pymysql.cursors.DictCursor,
        autocommit=True,
    )


def tool_list_tables(db) -> list[str]:
    with db.cursor() as cur:
        cur.execute("SHOW TABLES")
        rows = cur.fetchall()
        if not rows:
            return []
        key = list(rows[0].keys())[0]
        return [r[key] for r in rows]


def tool_describe_table(db, table: str) -> list[dict]:
    if not re.match(r"^[A-Za-z0-9_]+$", table):
        raise ValueError("invalid table name")
    with db.cursor() as cur:
        cur.execute(f"DESCRIBE `{table}`")
        return list(cur.fetchall())


def tool_run_select(db, sql: str, limit: int = 50) -> list[dict]:
    cleaned = sql.strip().rstrip(";")
    if not cleaned.lower().startswith("select"):
        raise ValueError("only SELECT statements are allowed")
    if " limit " not in cleaned.lower():
        cleaned = f"{cleaned} LIMIT {limit}"
    with db.cursor() as cur:
        cur.execute(cleaned)
        return list(cur.fetchall())


SYSTEM_PROMPT = """You are the Reveal DQ Analyser Agent for an enterprise data quality platform.
You analyse MySQL warehouses (RevealDQ telco CRM/billing sample) and recommend DQ rules.

DQ dimensions you MUST use: ACCURACY, COMPLETENESS, CONSISTENCY, TIMELINESS, VALIDITY, UNIQUENESS.

When recommending a rule, always provide:
- business definition (plain language for stewardship)
- technical definition (how to measure)
- impacted tables and columns
- SQL check (SELECT that returns failing rows)

You have tools:
1) list_tables()
2) describe_table(table)
3) run_select(sql) — SELECT only

Respond with JSON only in this shape:
{
  "answer": "markdown-friendly explanation for the user",
  "tool_calls": [{"name":"list_tables"}] OR [{"name":"describe_table","table":"customer"}] OR [{"name":"run_select","sql":"SELECT ..."}],
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
If the question is simple and schema is known, you may finish in one turn.
"""


def resolve_model(requested: Optional[str]) -> str:
    if requested and requested in ALLOWED_MODELS:
        return requested
    return CLAUDE_MODEL if CLAUDE_MODEL in ALLOWED_MODELS else next(iter(ALLOWED_MODELS))


def preview_from_rows(sql: str, rows: list[dict], title: str = "Query preview") -> DataPreview:
    columns = list(rows[0].keys()) if rows else []
    return DataPreview(title=title, sql=sql, columns=columns, rows=rows[:25])


def call_claude(messages: list[dict[str, str]], model: str) -> dict:
    if not ANTHROPIC_API_KEY:
        # Offline/demo mode without API key
        return {
            "answer": (
                "Claude API key is not configured (set ANTHROPIC_API_KEY). "
                "Using heuristic offline analysis against MySQL tools instead."
            ),
            "tool_calls": [{"name": "list_tables"}],
            "recommended_rules": [],
            "done": False,
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


def offline_analyse(db, message: str, traces: list, model: str) -> ChatResponse:
    tables = tool_list_tables(db)
    traces.append({"tool": "list_tables", "result": tables})
    rules: list[RecommendedRule] = []
    previews: list[DataPreview] = []
    answer_parts = [
        f"Connected to MySQL. Found {len(tables)} tables: {', '.join(tables)}.",
        "",
        f"Your question: {message}",
        "",
    ]

    # Completeness: unrated CDRs
    if "usage_cdr" in tables and "rated_usage" in tables:
        sql = (
            "SELECT c.cdr_id, c.usage_type, c.cdr_status "
            "FROM usage_cdr c LEFT JOIN rated_usage r ON r.cdr_id=c.cdr_id "
            "WHERE r.rated_usage_id IS NULL AND c.cdr_status<>'DUPLICATE'"
        )
        rows = tool_run_select(db, sql)
        traces.append({"tool": "run_select", "sql": sql, "rows": len(rows)})
        previews.append(preview_from_rows(sql, rows, "Unrated CDRs"))
        answer_parts.append(f"**Completeness**: {len(rows)} CDR(s) have no rated_usage row.")
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
        rows = tool_run_select(db, sql)
        traces.append({"tool": "run_select", "sql": sql, "rows": len(rows)})
        previews.append(preview_from_rows(sql, rows, "Invoice balance mismatches"))
        answer_parts.append(f"**Consistency**: {len(rows)} invoice(s) with total ≠ sum of lines.")
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
        rows = tool_run_select(db, sql)
        traces.append({"tool": "run_select", "sql": sql, "rows": len(rows)})
        previews.append(preview_from_rows(sql, rows, "Invalid MSISDNs"))
        answer_parts.append(f"**Validity**: {len(rows)} subscription(s) with invalid MSISDN.")
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

    return ChatResponse(
        answer="\n".join(answer_parts),
        recommendedRules=rules,
        toolTraces=traces,
        dataPreview=previews[0] if previews else None,
        modelUsed=model,
    )


@app.get("/health")
def health():
    return {
        "status": "ok",
        "service": "reveal-dq-agent",
        "claudeConfigured": bool(ANTHROPIC_API_KEY),
        "models": [{"id": k, "label": v} for k, v in ALLOWED_MODELS.items()],
        "defaultModel": resolve_model(None),
    }


@app.post("/chat", response_model=ChatResponse)
def chat(req: ChatRequest):
    model = resolve_model(req.model)
    try:
        db = connect(req.connection)
    except Exception as e:
        raise HTTPException(502, f"cannot connect to MySQL: {e}") from e

    traces: list[dict[str, Any]] = []
    last_preview: Optional[DataPreview] = None
    try:
        if not ANTHROPIC_API_KEY:
            return offline_analyse(db, req.message, traces, model)

        messages: list[dict[str, str]] = list(req.history or [])
        messages.append({"role": "user", "content": req.message})
        for _ in range(6):
            parsed = call_claude(messages, model)
            if parsed.get("_offline"):
                return offline_analyse(db, req.message, traces, model)

            tool_calls = parsed.get("tool_calls") or []
            if parsed.get("done") and not tool_calls:
                rules = [RecommendedRule(**r) for r in (parsed.get("recommended_rules") or [])]
                return ChatResponse(
                    answer=parsed.get("answer") or "",
                    recommendedRules=rules,
                    toolTraces=traces,
                    dataPreview=last_preview,
                    modelUsed=model,
                )

            tool_results = []
            for call in tool_calls:
                name = call.get("name")
                try:
                    if name == "list_tables":
                        result = tool_list_tables(db)
                    elif name == "describe_table":
                        result = tool_describe_table(db, call["table"])
                    elif name == "run_select":
                        result = tool_run_select(db, call["sql"])
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

        rules = [RecommendedRule(**r) for r in (parsed.get("recommended_rules") or [])]
        return ChatResponse(
            answer=parsed.get("answer") or "Analysis incomplete after tool loop.",
            recommendedRules=rules,
            toolTraces=traces,
            dataPreview=last_preview,
            modelUsed=model,
        )
    finally:
        db.close()


if __name__ == "__main__":
    import uvicorn

    uvicorn.run(
        "main:app",
        host="0.0.0.0",
        port=int(os.getenv("AGENT_PORT", "8090")),
        reload=False,
    )
