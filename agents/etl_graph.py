"""
Knowledge Graph + Visualization agents for the ETL Intelligence UI.

`build_graph()` turns the parsed metadata (from etl_analysis) into a directed
Source -> Transform -> Target knowledge graph laid out for React Flow
(@xyflow/react): every node carries a position, a kind (source/transform/target),
a type, and a heuristic AI explanation. `node_context()` reconstructs the
upstream/downstream neighbours of a node so the LLM reasoning agent can explain
it on demand.
"""

from __future__ import annotations

import re
from typing import Any

import etl_analysis

X_GAP = 280
Y_GAP = 120


def _slug(text: str) -> str:
    return re.sub(r"[^A-Za-z0-9_]+", "_", str(text)).strip("_") or "n"


def _layered_positions(node_ids: list[str], edges: list[tuple[str, str]]) -> dict[str, dict]:
    """Longest-path layering so edges always point left -> right."""
    if not node_ids:
        return {}
    idset = set(node_ids)
    edges = [(f, t) for f, t in edges if f in idset and t in idset and f != t]
    indeg = {n: 0 for n in node_ids}
    adj: dict[str, list[str]] = {n: [] for n in node_ids}
    for f, t in edges:
        adj[f].append(t)
        indeg[t] += 1

    # Kahn topological order (fallback keeps input order on cycles).
    queue = [n for n in node_ids if indeg[n] == 0] or [node_ids[0]]
    order: list[str] = []
    local_indeg = dict(indeg)
    seen: set[str] = set()
    while queue:
        n = queue.pop(0)
        if n in seen:
            continue
        seen.add(n)
        order.append(n)
        for m in adj[n]:
            local_indeg[m] -= 1
            if local_indeg[m] <= 0 and m not in seen:
                queue.append(m)
    for n in node_ids:
        if n not in seen:
            order.append(n)

    depth = {n: 0 for n in node_ids}
    for n in order:
        for m in adj[n]:
            depth[m] = max(depth[m], depth[n] + 1)

    by_depth: dict[int, list[str]] = {}
    for n in node_ids:
        by_depth.setdefault(depth[n], []).append(n)

    pos: dict[str, dict] = {}
    for d, group in sorted(by_depth.items()):
        for i, n in enumerate(group):
            pos[n] = {"x": d * X_GAP, "y": i * Y_GAP + 40}
    return pos


def _mk_node(nid: str, label: str, kind: str, ntype: str, detail: str, explanation: str) -> dict:
    return {
        "id": nid,
        "type": "etl",
        "position": {"x": 0, "y": 0},
        "data": {
            "label": label,
            "kind": kind,
            "ntype": ntype,
            "detail": detail,
            "explanation": explanation,
        },
    }


def _finalize(nodes: list[dict], edges: list[dict]) -> dict:
    positions = _layered_positions([n["id"] for n in nodes], [(e["source"], e["target"]) for e in edges])
    for n in nodes:
        if n["id"] in positions:
            n["position"] = positions[n["id"]]
    return {"nodes": nodes, "edges": edges}


# ---------------------------------------------------------------------------
# Informatica graph
# ---------------------------------------------------------------------------
def _informatica_graph(facts: dict) -> dict:
    sources = facts.get("sources", [])
    targets = facts.get("targets", [])
    instances = facts.get("instances", []) or facts.get("transformations", [])
    pairs = facts.get("connectorPairs", [])

    nodes: list[dict] = []
    id_by_name: dict[str, str] = {}

    def ensure(name: str, kind: str, ntype: str) -> str:
        if name in id_by_name:
            return id_by_name[name]
        nid = _slug(name)
        while any(n["id"] == nid for n in nodes):
            nid += "_"
        id_by_name[name] = nid
        note = etl_analysis.type_note("informatica_xml", ntype)
        nodes.append(_mk_node(nid, name, kind, ntype, note, f"**{name}** ({ntype}) — {note}."))
        return nid

    for s in sources:
        ensure(s, "source", "Source Definition")
    for inst in instances:
        name = inst.get("name", "TRANSFORM")
        ntype = inst.get("type", "Transformation")
        ensure(name, etl_analysis._kind_for_type(ntype), ntype)
    for t in targets:
        ensure(t, "target", "Target Definition")

    edges: list[dict] = []
    for f, t in pairs:
        if f in id_by_name and t in id_by_name:
            edges.append({
                "id": f"e_{id_by_name[f]}_{id_by_name[t]}",
                "source": id_by_name[f],
                "target": id_by_name[t],
                "animated": True,
            })

    # Fall back to a linear chain when the export carries no connectors.
    if not edges and nodes:
        chain = [n["id"] for n in nodes]
        for a, b in zip(chain, chain[1:]):
            edges.append({"id": f"e_{a}_{b}", "source": a, "target": b, "animated": True})

    _annotate_connections(nodes, edges)
    return _finalize(nodes, edges)


# ---------------------------------------------------------------------------
# SQL graph
# ---------------------------------------------------------------------------
def _sql_graph(facts: dict) -> dict:
    lineage = facts.get("lineage", [])
    target_set = {li["target"] for li in lineage if li.get("target")}

    nodes: list[dict] = []
    ids: set[str] = set()
    table_id: dict[str, str] = {}

    def ensure_table(name: str) -> str:
        if name in table_id:
            return table_id[name]
        nid = "t_" + _slug(name)
        while nid in ids:
            nid += "_"
        ids.add(nid)
        table_id[name] = nid
        kind = "target" if name in target_set else "source"
        role = "written by the script" if kind == "target" else "read by the script"
        nodes.append(_mk_node(nid, name, kind, "Table", f"table {role}", f"Table `{name}` — {role}."))
        return nid

    edges: list[dict] = []
    for li in lineage:
        sid = "s_" + str(li["index"])
        ids.add(sid)
        stype = li["type"]
        note = etl_analysis.type_note("sql", stype)
        label = f"{stype.title()} #{li['index']}"
        nodes.append(_mk_node(sid, label, "transform", stype, note, f"{label} — {note}."))
        for src in li.get("sources", []):
            fid = ensure_table(src)
            edges.append({"id": f"e_{fid}_{sid}", "source": fid, "target": sid, "animated": True})
        if li.get("target"):
            tid = ensure_table(li["target"])
            edges.append({"id": f"e_{sid}_{tid}", "source": sid, "target": tid, "animated": True})

    _annotate_connections(nodes, edges)
    return _finalize(nodes, edges)


def _annotate_connections(nodes: list[dict], edges: list[dict]) -> None:
    label_by_id = {n["id"]: n["data"]["label"] for n in nodes}
    up: dict[str, list[str]] = {n["id"]: [] for n in nodes}
    down: dict[str, list[str]] = {n["id"]: [] for n in nodes}
    for e in edges:
        down.setdefault(e["source"], []).append(label_by_id.get(e["target"], e["target"]))
        up.setdefault(e["target"], []).append(label_by_id.get(e["source"], e["source"]))
    for n in nodes:
        u = up.get(n["id"], [])
        d = down.get(n["id"], [])
        extra = []
        if u:
            extra.append("receives from " + ", ".join(f"`{x}`" for x in u))
        if d:
            extra.append("feeds " + ", ".join(f"`{x}`" for x in d))
        if extra:
            n["data"]["explanation"] = n["data"]["explanation"] + " It " + "; ".join(extra) + "."
        n["data"]["upstream"] = u
        n["data"]["downstream"] = d


# ---------------------------------------------------------------------------
# Public API
# ---------------------------------------------------------------------------
def build_graph(analysis: dict) -> dict:
    facts = analysis.get("facts", {})
    if analysis.get("fileType") == "informatica_xml":
        return _informatica_graph(facts)
    return _sql_graph(facts)


def node_context(graph: dict, node_id: str) -> dict:
    for n in graph.get("nodes", []):
        if n["id"] == node_id:
            return {
                "label": n["data"].get("label"),
                "kind": n["data"].get("kind"),
                "ntype": n["data"].get("ntype"),
                "upstream": n["data"].get("upstream", []),
                "downstream": n["data"].get("downstream", []),
                "explanation": n["data"].get("explanation"),
            }
    return {}
