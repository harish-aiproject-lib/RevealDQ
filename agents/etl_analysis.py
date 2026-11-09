"""
ETL / SQL static analysis for the Reveal DQ ETL Analyser Agent.

Deterministic (no-LLM-required) parsing of:
  - SQL files (.sql)
  - Informatica PowerCenter export files (.xml)

Produces a structured analysis: a detailed markdown write-up of the activities,
impact-story metrics (lines of code, complexity, object counts, ...), and an
ordered list of steps for a visual flow diagram. An optional LLM layer in main.py
enriches the narrative and answers multi-step follow-up questions.
"""

from __future__ import annotations

import re
import xml.etree.ElementTree as ET
from typing import Any, Optional


# ---------------------------------------------------------------------------
# File-type detection
# ---------------------------------------------------------------------------
def detect_file_type(file_name: str, content: str, hint: Optional[str]) -> str:
    if hint in {"sql", "informatica_xml"}:
        return hint
    name = (file_name or "").lower()
    if name.endswith(".xml"):
        return "informatica_xml"
    if name.endswith(".sql"):
        return "sql"
    head = (content or "").lstrip()[:400].lower()
    if head.startswith("<?xml") or "<powermart" in head or "<mapping" in head or "<transformation" in head:
        return "informatica_xml"
    return "sql"


INFA_TYPE_NOTES = {
    "Source Qualifier": "reads from the source and applies source-side filtering",
    "Source Definition": "defines a source table/file the mapping reads from",
    "Target Definition": "defines the target table/file the mapping writes to",
    "Expression": "computes derived / cleansed columns row-by-row",
    "Filter": "drops rows that fail a boolean condition",
    "Joiner": "joins two heterogeneous pipelines on a join condition",
    "Aggregator": "groups rows and computes aggregates (adds shuffle cost)",
    "Lookup Procedure": "enriches rows via a lookup source",
    "Router": "splits the pipeline into multiple conditional groups",
    "Update Strategy": "flags each row for insert / update / delete",
    "Sequence Generator": "generates surrogate / sequence keys",
    "Sorter": "orders rows (often a Joiner / Aggregator prerequisite)",
    "Normalizer": "pivots / normalises multi-occurring data",
    "Rank": "ranks rows within groups",
    "Union": "merges multiple pipelines with the same structure",
    "Stored Procedure": "invokes a database stored procedure",
    "Transaction Control": "controls commit / rollback boundaries",
}

SQL_TYPE_NOTES = {
    "SELECT": "reads / projects data",
    "INSERT": "loads rows into a target table",
    "UPDATE": "mutates existing rows in place",
    "DELETE": "removes rows from a table",
    "MERGE": "upserts (insert + update) into a target",
    "CREATE": "defines a new database object",
    "WITH": "builds common table expressions used downstream",
    "TRUNCATE": "empties a table",
    "ALTER": "changes an object's structure",
    "DROP": "drops a database object",
    "CALL": "invokes a stored routine",
}


def type_note(ftype: str, node_type: str) -> str:
    table = INFA_TYPE_NOTES if ftype == "informatica_xml" else SQL_TYPE_NOTES
    return table.get(node_type, "processes data in the pipeline")


def _complexity_band(score: float) -> tuple[str, str]:
    if score < 12:
        return "Low", "good"
    if score < 30:
        return "Medium", "warn"
    return "High", "bad"


def _step(index: int, name: str, stype: str, detail: str, kind: str) -> dict:
    return {"index": index, "name": name, "type": stype, "detail": detail, "kind": kind}


# ---------------------------------------------------------------------------
# SQL analysis
# ---------------------------------------------------------------------------
_SQL_STMT_TYPES = [
    "SELECT", "INSERT", "UPDATE", "DELETE", "MERGE", "CREATE", "ALTER",
    "DROP", "TRUNCATE", "WITH", "GRANT", "CALL",
]


def _strip_sql_comments(sql: str) -> str:
    sql = re.sub(r"/\*.*?\*/", " ", sql, flags=re.DOTALL)
    sql = re.sub(r"--[^\n]*", " ", sql)
    return sql


def analyze_sql(content: str) -> dict:
    lines = content.splitlines()
    total_lines = len(lines)
    blank = sum(1 for ln in lines if not ln.strip())
    comment = sum(1 for ln in lines if ln.strip().startswith("--"))
    block_comment_lines = 0
    for m in re.finditer(r"/\*.*?\*/", content, flags=re.DOTALL):
        block_comment_lines += m.group(0).count("\n") + 1
    comment_total = comment + block_comment_lines
    code_lines = max(total_lines - blank - comment, 0)

    clean = _strip_sql_comments(content)

    raw_statements = [s.strip() for s in clean.split(";") if s.strip()]
    joins = len(re.findall(r"\bjoin\b", clean, re.I))
    subqueries = len(re.findall(r"\(\s*select\b", clean, re.I))
    unions = len(re.findall(r"\bunion\b", clean, re.I))
    cases = len(re.findall(r"\bcase\b", clean, re.I))
    windows = len(re.findall(r"\bover\s*\(", clean, re.I))
    ctes = len(re.findall(r"(?:with|,)\s+[a-z_]\w*\s+as\s*\(", clean, re.I))
    aggregates = len(re.findall(r"\b(sum|count|avg|min|max|group_concat)\s*\(", clean, re.I))

    # Statement type counts
    type_counts: dict[str, int] = {}
    for stmt in raw_statements:
        head = stmt.lstrip().split(None, 1)[0].upper() if stmt.strip() else ""
        if head in _SQL_STMT_TYPES:
            type_counts[head] = type_counts.get(head, 0) + 1

    # Referenced tables
    tables: list[str] = []
    for pat in (r"\bfrom\s+([`\"\[]?[\w.]+)", r"\bjoin\s+([`\"\[]?[\w.]+)",
                r"\binto\s+([`\"\[]?[\w.]+)", r"\bupdate\s+([`\"\[]?[\w.]+)"):
        for t in re.findall(pat, clean, re.I):
            name = t.strip("`\"[]").lower()
            if name and name not in tables and not name.isdigit():
                tables.append(name)

    score = (
        joins * 2 + subqueries * 3 + ctes * 2 + unions * 2 + cases * 1
        + windows * 3 + len(raw_statements) * 1 + aggregates * 0.5
    )
    band, tone = _complexity_band(score)

    # Steps: one per meaningful statement, showing target and sources.
    steps: list[dict] = []
    lineage: list[dict] = []
    for i, stmt in enumerate(raw_statements, 1):
        head = stmt.lstrip().split(None, 1)[0].upper() if stmt.strip() else "STMT"
        target = ""
        mt = re.search(r"\b(?:insert\s+into|update|merge\s+into|create\s+table(?:\s+if\s+not\s+exists)?)\s+([`\"\[]?[\w.]+)", stmt, re.I)
        if mt:
            target = mt.group(1).strip("`\"[]")
        srcs = re.findall(r"\bfrom\s+([`\"\[]?[\w.]+)|\bjoin\s+([`\"\[]?[\w.]+)", stmt, re.I)
        src_names = sorted({(a or b).strip("`\"[]") for a, b in srcs if (a or b)})
        detail = (f"→ {target}" if target else "") + (f"  ⇐ {', '.join(src_names)}" if src_names else "")
        kind = "target" if head in {"INSERT", "UPDATE", "MERGE", "CREATE"} else "transform"
        if head == "SELECT" and not src_names:
            kind = "source"
        step = _step(i, f"{head} statement {i}", head.title(), detail.strip() or "—", kind)
        step["sources"] = src_names
        step["target"] = target
        steps.append(step)
        lineage.append({"index": i, "type": head, "sources": src_names, "target": target})

    metrics = [
        {"label": "Lines of code", "value": str(code_lines),
         "hint": f"{total_lines} total · {comment_total} comments · {blank} blank", "tone": "info"},
        {"label": "SQL statements", "value": str(len(raw_statements)),
         "hint": ", ".join(f"{k} {v}" for k, v in type_counts.items()) or "—", "tone": "info"},
        {"label": "Tables referenced", "value": str(len(tables)),
         "hint": ", ".join(tables[:6]) + (" …" if len(tables) > 6 else ""), "tone": "info"},
        {"label": "Joins", "value": str(joins), "hint": f"{subqueries} subqueries · {ctes} CTEs", "tone": "info"},
        {"label": "Complexity", "value": band, "hint": f"score {round(score)} · {unions} unions · {windows} window fns", "tone": tone},
    ]

    summary = _sql_markdown(
        code_lines, total_lines, comment_total, blank, len(raw_statements),
        type_counts, tables, joins, subqueries, ctes, unions, cases, windows, band, score,
    )

    return {
        "fileType": "sql",
        "summaryMarkdown": summary,
        "metrics": metrics,
        "steps": steps,
        "facts": {
            "codeLines": code_lines, "totalLines": total_lines, "statements": len(raw_statements),
            "typeCounts": type_counts, "tables": tables, "joins": joins, "subqueries": subqueries,
            "ctes": ctes, "unions": unions, "cases": cases, "windows": windows,
            "complexity": band, "score": round(score, 1), "lineage": lineage,
        },
    }


def _sql_markdown(code, total, comments, blank, stmts, type_counts, tables,
                  joins, subqueries, ctes, unions, cases, windows, band, score) -> str:
    parts = ["## SQL analysis", ""]
    parts.append(f"This script contains **{stmts} statement(s)** across **{total} line(s)** "
                 f"({code} code, {comments} comment, {blank} blank).")
    parts.append("")
    parts.append("### Activities")
    if type_counts:
        for k, v in type_counts.items():
            verb = {
                "SELECT": "reads / projects data", "INSERT": "loads rows into a target",
                "UPDATE": "mutates existing rows", "DELETE": "removes rows",
                "MERGE": "upserts (insert/update)", "CREATE": "defines objects",
                "WITH": "builds common table expressions", "TRUNCATE": "empties a table",
                "ALTER": "changes object structure", "DROP": "drops objects",
            }.get(k, "runs a statement")
            parts.append(f"- **{k}** × {v} — {verb}.")
    else:
        parts.append("- No standard DML/DDL statements detected.")
    parts.append("")
    parts.append("### Data touched")
    parts.append(f"- Tables/views referenced: {', '.join('`'+t+'`' for t in tables) if tables else '—'}")
    parts.append("")
    parts.append("### Complexity drivers")
    parts.append(f"- Joins: **{joins}**, subqueries: **{subqueries}**, CTEs: **{ctes}**, "
                 f"unions: **{unions}**, CASE expressions: **{cases}**, window functions: **{windows}**.")
    parts.append(f"- Overall complexity: **{band}** (weighted score {round(score)}).")
    return "\n".join(parts)


# ---------------------------------------------------------------------------
# Informatica PowerCenter XML analysis
# ---------------------------------------------------------------------------
_INFA_WEIGHTS = {
    "Joiner": 3, "Lookup Procedure": 3, "Aggregator": 3, "Normalizer": 3,
    "Router": 2, "Update Strategy": 2, "Sorter": 2, "Union": 2, "Stored Procedure": 2,
    "Rank": 2, "Transaction Control": 2, "Filter": 1, "Expression": 1,
    "Sequence Generator": 1, "Source Qualifier": 1,
}


def _local(tag: str) -> str:
    return tag.split("}")[-1] if "}" in tag else tag


def _kind_for_type(t: str) -> str:
    tl = (t or "").lower()
    if "source" in tl:
        return "source"
    if "target" in tl:
        return "target"
    return "transform"


def analyze_informatica(content: str) -> dict:
    try:
        root = ET.fromstring(content)
    except ET.ParseError as exc:
        return {
            "fileType": "informatica_xml",
            "summaryMarkdown": f"## Informatica analysis\n\n**Could not parse XML:** {exc}",
            "metrics": [{"label": "Parse", "value": "failed", "hint": str(exc), "tone": "bad"}],
            "steps": [],
            "facts": {"error": str(exc)},
        }

    def findall(tag: str):
        return [el for el in root.iter() if _local(el.tag) == tag]

    sources = [s.get("NAME", "SOURCE") for s in findall("SOURCE")]
    targets = [t.get("NAME", "TARGET") for t in findall("TARGET")]
    transformations = [
        (t.get("NAME", "TRANSFORM"), t.get("TYPE", "Transformation")) for t in findall("TRANSFORMATION")
    ]
    instances = [
        (
            i.get("NAME", i.get("TRANSFORMATION_NAME", "INSTANCE")),
            i.get("TRANSFORMATION_TYPE", i.get("TYPE", "Transformation")),
        )
        for i in findall("INSTANCE")
    ]
    connectors = [
        (c.get("FROMINSTANCE", ""), c.get("TOINSTANCE", "")) for c in findall("CONNECTOR")
    ]
    mappings = [m.get("NAME", "MAPPING") for m in findall("MAPPING")]
    workflows = [w.get("NAME", "WORKFLOW") for w in findall("WORKFLOW")]
    sessions = [s.get("NAME", "SESSION") for s in findall("SESSION")]
    ports = len(findall("TRANSFORMFIELD"))
    src_fields = len(findall("SOURCEFIELD"))
    tgt_fields = len(findall("TARGETFIELD"))

    # Order the instances into a flow using connectors (Kahn-ish); fall back to doc order.
    inst_types = {name: typ for name, typ in instances}
    ordered = _order_instances(instances, connectors)

    steps: list[dict] = []
    for i, name in enumerate(ordered, 1):
        typ = inst_types.get(name, "Transformation")
        downstream = [to for frm, to in connectors if frm == name]
        detail = ("→ " + ", ".join(sorted(set(downstream)))) if downstream else "terminal"
        steps.append(_step(i, name, typ, detail, _kind_for_type(typ)))

    # Complexity from transformation type weights.
    score = 0.0
    type_counts: dict[str, int] = {}
    for _, typ in (transformations or instances):
        type_counts[typ] = type_counts.get(typ, 0) + 1
        score += _INFA_WEIGHTS.get(typ, 1)
    score += len(connectors) * 0.2 + len(sources) + len(targets)
    band, tone = _complexity_band(score)

    metrics = [
        {"label": "Transformations", "value": str(len(transformations) or len(instances)),
         "hint": ", ".join(f"{k} {v}" for k, v in list(type_counts.items())[:5]) or "—", "tone": "info"},
        {"label": "Sources → Targets", "value": f"{len(sources)} → {len(targets)}",
         "hint": ", ".join(sources[:3] + targets[:3]) or "—", "tone": "info"},
        {"label": "Data flows", "value": str(len(connectors)),
         "hint": f"{ports} ports · {src_fields}+{tgt_fields} fields", "tone": "info"},
        {"label": "Mappings / Sessions", "value": f"{len(mappings)} / {len(sessions)}",
         "hint": ", ".join(mappings[:3]) or "—", "tone": "info"},
        {"label": "Complexity", "value": band, "hint": f"score {round(score)}", "tone": tone},
    ]

    summary = _infa_markdown(
        mappings, workflows, sessions, sources, targets, transformations or instances,
        type_counts, connectors, ports, band, score,
    )

    return {
        "fileType": "informatica_xml",
        "summaryMarkdown": summary,
        "metrics": metrics,
        "steps": steps,
        "facts": {
            "sources": sources, "targets": targets, "mappings": mappings,
            "workflows": workflows, "sessions": sessions,
            "transformations": [{"name": n, "type": t} for n, t in transformations],
            "instances": [{"name": n, "type": t} for n, t in instances],
            "connectorPairs": [[f, t] for f, t in connectors if f and t],
            "typeCounts": type_counts, "connectors": len(connectors), "ports": ports,
            "complexity": band, "score": round(score, 1),
        },
    }


def _order_instances(instances: list[tuple[str, str]], connectors: list[tuple[str, str]]) -> list[str]:
    names = [n for n, _ in instances]
    if not names:
        return []
    edges = [(f, t) for f, t in connectors if f in names and t in names and f != t]
    if not edges:
        return names
    indeg = {n: 0 for n in names}
    adj: dict[str, list[str]] = {n: [] for n in names}
    for f, t in edges:
        adj[f].append(t)
        indeg[t] += 1
    queue = [n for n in names if indeg[n] == 0] or names[:1]
    seen: set[str] = set()
    order: list[str] = []
    while queue:
        n = queue.pop(0)
        if n in seen:
            continue
        seen.add(n)
        order.append(n)
        for m in adj.get(n, []):
            indeg[m] -= 1
            if indeg[m] <= 0 and m not in seen:
                queue.append(m)
    for n in names:  # append any nodes left out by cycles
        if n not in seen:
            order.append(n)
    return order


def _infa_markdown(mappings, workflows, sessions, sources, targets, transforms,
                   type_counts, connectors, ports, band, score) -> str:
    parts = ["## Informatica ETL analysis", ""]
    parts.append(f"This export defines **{len(mappings)} mapping(s)**, **{len(workflows)} workflow(s)** "
                 f"and **{len(sessions)} session(s)**, moving data from **{len(sources)} source(s)** "
                 f"to **{len(targets)} target(s)** through **{len(transforms)} transformation(s)**.")
    parts.append("")
    parts.append("### Sources & targets")
    parts.append(f"- Sources: {', '.join('`'+s+'`' for s in sources) if sources else '—'}")
    parts.append(f"- Targets: {', '.join('`'+t+'`' for t in targets) if targets else '—'}")
    parts.append("")
    parts.append("### Transformations")
    if type_counts:
        for k, v in type_counts.items():
            note = {
                "Source Qualifier": "reads from the source and applies source-side filtering",
                "Expression": "computes derived / cleansed columns row-by-row",
                "Filter": "drops rows that fail a condition",
                "Joiner": "joins two heterogeneous pipelines",
                "Aggregator": "groups and aggregates (adds shuffle cost)",
                "Lookup Procedure": "enriches rows via a lookup source",
                "Router": "splits the pipeline into multiple groups",
                "Update Strategy": "flags rows for insert/update/delete",
                "Sequence Generator": "generates surrogate keys",
                "Sorter": "orders rows (often a Joiner/Aggregator prerequisite)",
                "Normalizer": "pivots/normalises multi-occurring data",
            }.get(k, "processes rows in the pipeline")
            parts.append(f"- **{k}** × {v} — {note}.")
    else:
        parts.append("- No transformations detected.")
    parts.append("")
    parts.append("### Flow & complexity")
    parts.append(f"- Data-flow connectors: **{len(connectors)}**, ports/fields: **{ports}**.")
    parts.append(f"- Overall complexity: **{band}** (weighted score {round(score)}).")
    return "\n".join(parts)


# ---------------------------------------------------------------------------
# Dispatch + heuristic follow-up answering
# ---------------------------------------------------------------------------
def analyze(file_name: str, file_type: str, content: str) -> dict:
    ftype = detect_file_type(file_name, content, file_type)
    if ftype == "informatica_xml":
        return analyze_informatica(content)
    return analyze_sql(content)


def heuristic_answer(question: str, analysis: dict, content: str) -> str:
    q = (question or "").lower()
    facts = analysis.get("facts", {})
    ftype = analysis.get("fileType")

    if any(k in q for k in ("line", "loc", "how big", "size")):
        if ftype == "sql":
            return (f"The script has **{facts.get('totalLines', 0)} total lines** "
                    f"({facts.get('codeLines', 0)} lines of code) and "
                    f"**{facts.get('statements', 0)} SQL statements**.")
        return f"The export has **{facts.get('ports', 0)} ports** across its transformations."

    if any(k in q for k in ("complex", "hard", "difficult", "risk")):
        return (f"Overall complexity is **{facts.get('complexity', 'Unknown')}** "
                f"(weighted score {facts.get('score', 0)}). "
                + ("Drivers: joins/subqueries/CTEs/window functions."
                   if ftype == "sql" else "Drivers: Joiner/Aggregator/Lookup transformations and data flows."))

    if any(k in q for k in ("table", "source", "from", "read")):
        if ftype == "sql":
            tabs = facts.get("tables", [])
            return "Tables/views referenced: " + (", ".join("`" + t + "`" for t in tabs) if tabs else "none detected") + "."
        srcs = facts.get("sources", [])
        return "Sources: " + (", ".join("`" + s + "`" for s in srcs) if srcs else "none detected") + "."

    if any(k in q for k in ("target", "write", "load", "insert")):
        if ftype == "sql":
            tc = facts.get("typeCounts", {})
            return "Write operations: " + (", ".join(f"{k}×{v}" for k, v in tc.items() if k in ("INSERT", "UPDATE", "MERGE", "CREATE")) or "none") + "."
        tgts = facts.get("targets", [])
        return "Targets: " + (", ".join("`" + t + "`" for t in tgts) if tgts else "none detected") + "."

    if any(k in q for k in ("step", "transform", "flow", "stage", "pipeline")):
        steps = analysis.get("steps", [])
        if not steps:
            return "No discrete steps were detected in this file."
        lines = [f"{s['index']}. **{s['name']}** ({s['type']}) — {s['detail']}" for s in steps[:12]]
        return "Pipeline steps:\n" + "\n".join(lines)

    if any(k in q for k in ("join",)) and ftype == "sql":
        return f"The script uses **{facts.get('joins', 0)} join(s)** and {facts.get('subqueries', 0)} subquery(ies)."

    # Default: restate the summary highlights.
    return (analysis.get("summaryMarkdown", "").split("### ")[0].strip()
            + "\n\nAsk me about tables/sources, targets, steps, or complexity for more detail.")
