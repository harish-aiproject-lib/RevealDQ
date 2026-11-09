"""
ETL Code Conversion engine for the ETL Code Conversion Agent.

Multi-agent pipeline helpers:
  - detect_source(): identify the source dialect / ETL tool from a file.
  - build_plan(): produce a downloadable conversion-plan markdown.
  - heuristic_convert(): best-effort, deterministic transpile to Databricks SQL
    or PySpark (used when no LLM key is configured; the LLM path in main.py
    produces higher-quality output when ANTHROPIC_API_KEY is set).
  - validate_code(): syntax-check the converted artifact.

Everything here is dialect-aware but intentionally conservative: conversions are
always flagged for human review (requirement: 1:1, reviewed, validated, tested).
"""

from __future__ import annotations

import re
from typing import Any

import etl_analysis

TARGET_CATALOG = "migration_demo"


# ---------------------------------------------------------------------------
# Source detection
# ---------------------------------------------------------------------------
def detect_source(file_name: str, content: str) -> dict:
    name = (file_name or "").lower()
    head = (content or "")[:4000]
    low = head.lower()

    if name.endswith(".dtsx") or "<executable" in low or "dts:executable" in low:
        return {"sourceType": "ssis_dtsx", "sourceLabel": "SSIS package (.dtsx)", "kind": "etl"}
    if name.endswith(".xml") or "<powermart" in low or "<mapping" in low:
        return {"sourceType": "informatica_xml", "sourceLabel": "Informatica mapping (.xml)", "kind": "etl"}

    # SQL family — sniff the dialect.
    if re.search(r"\bgetdate\s*\(|\bisnull\s*\(|\[dbo\]|\bnvarchar\b|\btop\s+\d+|@@|\bbegin\s+tran", low):
        return {"sourceType": "tsql", "sourceLabel": "SQL Server / T-SQL", "kind": "sql"}
    if re.search(r"\bsysdate\b|\bnvl\s*\(|\bfrom\s+dual\b|\bvarchar2\b|\bdbms_|\bpackage\b|\bnumber\s*\(", low):
        return {"sourceType": "oracle", "sourceLabel": "Oracle PL/SQL", "kind": "sql"}
    if re.search(r"\bdelimiter\b|\bauto_increment\b|engine\s*=|`", head):
        return {"sourceType": "mysql", "sourceLabel": "MySQL / stored procedure", "kind": "sql"}
    return {"sourceType": "sql", "sourceLabel": "Generic SQL", "kind": "sql"}


# ---------------------------------------------------------------------------
# Analysis + plan
# ---------------------------------------------------------------------------
def analyze(file_name: str, content: str) -> dict:
    src = detect_source(file_name, content)
    stype = src["sourceType"]

    if stype in ("informatica_xml", "ssis_dtsx"):
        base = etl_analysis.analyze_informatica(content) if stype == "informatica_xml" else _analyze_dtsx(content)
    else:
        base = etl_analysis.analyze_sql(content)

    plan = build_plan(file_name, src, base)
    return {
        "source": src,
        "summaryMarkdown": base.get("summaryMarkdown", ""),
        "planMarkdown": plan,
        "metrics": base.get("metrics", []),
        "steps": base.get("steps", []),
        "facts": base.get("facts", {}),
    }


def _analyze_dtsx(content: str) -> dict:
    """Lightweight SSIS .dtsx introspection (Data Flow / Execute SQL tasks)."""
    tasks = len(re.findall(r"dts:executable\b", content, re.I)) or len(re.findall(r"<executable", content, re.I))
    dataflows = len(re.findall(r"pipeline", content, re.I))
    sqls = len(re.findall(r"executesqltask", content, re.I))
    conns = len(re.findall(r"connectionmanager", content, re.I))
    total_lines = content.count("\n") + 1
    band, tone = ("Medium", "warn") if tasks > 5 else ("Low", "good")
    metrics = [
        {"label": "Executables / tasks", "value": str(tasks), "hint": f"{sqls} Execute-SQL tasks", "tone": "info"},
        {"label": "Data flows", "value": str(dataflows), "hint": "pipeline components", "tone": "info"},
        {"label": "Connection managers", "value": str(conns), "hint": "sources & targets", "tone": "info"},
        {"label": "Size", "value": f"{total_lines} lines", "hint": "raw XML", "tone": "info"},
        {"label": "Complexity", "value": band, "hint": "heuristic", "tone": tone},
    ]
    md = (
        "## SSIS package analysis\n\n"
        f"This package defines **{tasks} executable task(s)**, **{dataflows} data-flow component(s)** "
        f"and **{conns} connection manager(s)**.\n\n"
        "### Migration notes\n"
        "- Data Flow tasks map to Spark reads/writes; Execute-SQL tasks map to `spark.sql` statements.\n"
        "- Connection managers become Databricks connections / Unity Catalog tables.\n"
    )
    return {"summaryMarkdown": md, "metrics": metrics, "steps": [],
            "facts": {"tasks": tasks, "dataflows": dataflows, "executeSql": sqls, "connections": conns}}


def build_plan(file_name: str, src: dict, base: dict) -> str:
    facts = base.get("facts", {})
    parts = [
        "# Conversion plan",
        "",
        f"**Source file:** `{file_name or 'uploaded file'}`  ",
        f"**Detected source:** {src['sourceLabel']}  ",
        f"**Target:** Databricks (catalog `{TARGET_CATALOG}`)  ",
        "",
        "## Strategy",
        "- Perform a **1:1 conversion** of source logic to Databricks SQL / PySpark.",
        "- Preserve object names; create target objects under the "
        f"`{TARGET_CATALOG}` catalog.",
        "- Every converted artifact is **reviewed, syntax-validated, and tested** before deploy.",
        "",
        "## Steps",
        "1. **Analyse** — parse the source and inventory objects (this document).",
        "2. **Convert** — transpile to the chosen target with the selected model.",
        "3. **Validate** — syntax-check the generated code (and dry-run on the warehouse where possible).",
        "4. **Check-in** — commit the reviewed artifact to the Git repository.",
        "5. **Deploy** — import the artifact into the Databricks workspace as a notebook.",
        "6. **Job** — register and (optionally) trigger a Databricks Job for the notebook.",
        "",
        "## Inventory",
    ]
    if facts.get("tables"):
        parts.append(f"- Tables/objects referenced: {', '.join('`'+t+'`' for t in facts['tables'][:12])}")
    if facts.get("sources") or facts.get("targets"):
        parts.append(f"- Sources: {', '.join(facts.get('sources', []) or ['—'])}")
        parts.append(f"- Targets: {', '.join(facts.get('targets', []) or ['—'])}")
    if facts.get("statements"):
        parts.append(f"- SQL statements: {facts.get('statements')}")
    if facts.get("tasks"):
        parts.append(f"- SSIS tasks: {facts.get('tasks')} ({facts.get('executeSql', 0)} Execute-SQL)")
    parts.append("")
    parts.append("## Risks & review points")
    parts.append("- Dialect-specific functions are auto-mapped heuristically — **verify semantics**.")
    parts.append("- Procedural logic (loops, cursors, exceptions) may need manual PySpark redesign.")
    parts.append("- Validate data types and null-handling against Databricks/Delta.")
    return "\n".join(parts)


# ---------------------------------------------------------------------------
# Heuristic conversion (deterministic fallback)
# ---------------------------------------------------------------------------
_SQL_FUNC_MAP = [
    (r"\bNVL\s*\(", "coalesce("),
    (r"\bISNULL\s*\(", "coalesce("),
    (r"\bNVL2\s*\(", "nvl2("),
    (r"\bSYSDATE\b", "current_timestamp()"),
    (r"\bSYSTIMESTAMP\b", "current_timestamp()"),
    (r"\bGETDATE\s*\(\s*\)", "current_timestamp()"),
    (r"\bGETUTCDATE\s*\(\s*\)", "current_timestamp()"),
    (r"\bTRUNC\s*\(", "date_trunc("),
    (r"\bLEN\s*\(", "length("),
    (r"\bCHARINDEX\s*\(", "locate("),
    (r"\bSUBSTR\s*\(", "substring("),
    (r"\bTO_DATE\s*\(", "to_date("),
    (r"\bTO_CHAR\s*\(", "cast("),
]


def heuristic_convert(content: str, source_type: str, target: str) -> dict:
    notes: list[str] = []
    if source_type in ("informatica_xml", "ssis_dtsx"):
        return _convert_etl_scaffold(content, source_type, target, notes)

    sql = content

    # Strip dialect noise.
    sql = re.sub(r"^\s*DELIMITER\s+.*$", "", sql, flags=re.I | re.M)
    sql = re.sub(r"\bFROM\s+DUAL\b", "", sql, flags=re.I)
    sql = re.sub(r"^\s*/\s*$", "", sql, flags=re.M)  # Oracle statement terminator
    sql = sql.replace("[", "`").replace("]", "`")     # T-SQL identifiers

    for pat, repl in _SQL_FUNC_MAP:
        if re.search(pat, sql, re.I):
            sql = re.sub(pat, repl, sql, flags=re.I)
            notes.append(f"Mapped `{pat}` → `{repl}`")

    # SELECT TOP n  ->  LIMIT n
    m = re.search(r"\bSELECT\s+TOP\s+(\d+)\s+", sql, re.I)
    if m:
        n = m.group(1)
        sql = re.sub(r"\bSELECT\s+TOP\s+\d+\s+", "SELECT ", sql, flags=re.I)
        sql = sql.rstrip().rstrip(";") + f"\nLIMIT {n};"
        notes.append(f"Converted `TOP {n}` → `LIMIT {n}`")

    # ROWNUM <= n -> LIMIT n
    m = re.search(r"\bROWNUM\s*<=\s*(\d+)", sql, re.I)
    if m:
        notes.append(f"Found `ROWNUM <= {m.group(1)}` — review; use `LIMIT` in Databricks.")

    header = (
        f"-- Converted to Databricks SQL (target catalog: {TARGET_CATALOG})\n"
        f"-- Source dialect: {source_type} · heuristic transpile — REVIEW REQUIRED\n"
        f"USE CATALOG {TARGET_CATALOG};\n\n"
    )

    if target == "pyspark":
        code = _wrap_pyspark(sql.strip())
        language = "python"
    else:
        code = header + sql.strip() + ("\n" if not sql.strip().endswith("\n") else "")
        language = "sql"

    if not notes:
        notes.append("No dialect-specific functions detected; structure preserved 1:1.")
    return {"code": code, "language": language, "notes": notes}


def _wrap_pyspark(sql: str) -> str:
    safe = sql.replace('"""', '\\"\\"\\"')
    return (
        f"# Converted to PySpark (target catalog: {TARGET_CATALOG})\n"
        "# Heuristic conversion — REVIEW REQUIRED\n"
        "from pyspark.sql import SparkSession\n\n"
        "spark = SparkSession.builder.getOrCreate()\n"
        f'spark.sql("USE CATALOG {TARGET_CATALOG}")\n\n'
        f'query = """\n{safe}\n"""\n\n'
        "df = spark.sql(query)\n"
        "df.display()\n"
        "# Example persist:\n"
        f'# df.write.mode("overwrite").saveAsTable("{TARGET_CATALOG}.default.converted_output")\n'
    )


def _convert_etl_scaffold(content: str, source_type: str, target: str, notes: list[str]) -> dict:
    tool = "Informatica mapping" if source_type == "informatica_xml" else "SSIS package"
    notes.append(f"{tool} converted to a PySpark ETL scaffold — implement each stage from the analysis.")
    try:
        base = etl_analysis.analyze_informatica(content) if source_type == "informatica_xml" else _analyze_dtsx(content)
        facts = base.get("facts", {})
    except Exception:  # noqa: BLE001
        facts = {}
    sources = facts.get("sources", []) or ["SOURCE_TABLE"]
    targets = facts.get("targets", []) or ["TARGET_TABLE"]
    reads = "\n".join(
        f'df_{i} = spark.read.table("{TARGET_CATALOG}.default.{s.lower()}")  # from {s}'
        for i, s in enumerate(sources[:5])
    )
    writes = "\n".join(
        f'# result.write.mode("overwrite").saveAsTable("{TARGET_CATALOG}.default.{t.lower()}")'
        for t in targets[:5]
    )
    code = (
        f"# {tool} -> PySpark scaffold (target catalog: {TARGET_CATALOG})\n"
        "# Heuristic scaffold — implement each transformation stage. REVIEW REQUIRED\n"
        "from pyspark.sql import SparkSession, functions as F\n\n"
        "spark = SparkSession.builder.getOrCreate()\n"
        f'spark.sql("USE CATALOG {TARGET_CATALOG}")\n\n'
        "# --- Sources ---\n"
        f"{reads}\n\n"
        "# --- Transformations (map each mapping/transform here) ---\n"
        "# result = df_0.join(...).withColumn(...).filter(...)\n\n"
        "# --- Targets ---\n"
        f"{writes}\n"
    )
    return {"code": code, "language": "python", "notes": notes}


# ---------------------------------------------------------------------------
# Validation
# ---------------------------------------------------------------------------
def validate_code(code: str, language: str) -> dict:
    issues: list[str] = []
    if language == "python":
        try:
            compile(code, "<converted>", "exec")
            return {"ok": True, "issues": [], "checkedWith": "python-compile"}
        except SyntaxError as e:
            return {"ok": False, "issues": [f"Line {e.lineno}: {e.msg}"], "checkedWith": "python-compile"}

    # SQL validation — prefer sqlglot if present.
    try:
        import sqlglot

        checked = "sqlglot(databricks)"
        for stmt in [s for s in code.split(";") if s.strip() and not s.strip().startswith("--")]:
            try:
                sqlglot.parse_one(stmt, read="databricks")
            except Exception as e:  # noqa: BLE001
                issues.append(f"{str(e)[:160]} :: {stmt.strip()[:60]}")
    except ImportError:
        checked = "basic-sql-checks"
        if code.count("(") != code.count(")"):
            issues.append("Unbalanced parentheses.")
        if re.search(r"\b(NVL|ISNULL|SYSDATE|GETDATE)\b", code, re.I):
            issues.append("Residual source-dialect function detected — review.")
        if "`" in code and code.count("`") % 2 != 0:
            issues.append("Unbalanced backtick identifiers.")
    return {"ok": not issues, "issues": issues, "checkedWith": checked}
