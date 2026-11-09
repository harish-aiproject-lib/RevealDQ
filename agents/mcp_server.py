"""
Reveal DQ MySQL MCP server (stdio transport).

Speaks the Model Context Protocol over stdio using JSON-RPC 2.0 with
newline-delimited messages (`initialize`, `tools/list`, `tools/call`). It exposes
read-only MySQL tools so the Analyser Agent can inspect data sets through a real
MCP channel rather than a hidden in-process DB call.

Implemented with the standard library only (no external MCP SDK) so it runs on
the project's Python 3.9 runtime. Connection parameters come from environment
variables injected by the agent, scoping the server to the active workspace's
data connection.
"""

from __future__ import annotations

import json
import os
import re
import sys

import pymysql

from sql_guard import assert_readonly_select

PROTOCOL_VERSION = "2024-11-05"
SERVER_INFO = {"name": "reveal-dq-mysql", "version": "0.2.0"}
_IDENT = re.compile(r"^[A-Za-z0-9_]+$")

TOOLS = [
    {
        "name": "list_tables",
        "description": "List every table in the connected MySQL database (JSON array of names).",
        "inputSchema": {"type": "object", "properties": {}},
    },
    {
        "name": "describe_table",
        "description": "Describe the columns of a table (JSON array of column metadata).",
        "inputSchema": {
            "type": "object",
            "properties": {"table": {"type": "string"}},
            "required": ["table"],
        },
    },
    {
        "name": "run_select",
        "description": "Run a read-only SELECT query (auto-limited) and return rows as JSON.",
        "inputSchema": {
            "type": "object",
            "properties": {
                "sql": {"type": "string"},
                "limit": {"type": "integer"},
            },
            "required": ["sql"],
        },
    },
]


# A single MySQL connection is kept warm for the lifetime of the server process
# and reused across every tool call. This removes a full MySQL handshake per
# query — the dominant cost during metadata assessment and whole-database scans.
_DB = None


def _connect():
    return pymysql.connect(
        host=os.getenv("MYSQL_HOST") or "127.0.0.1",
        port=int(os.getenv("MYSQL_PORT") or "3306"),
        user=os.getenv("MYSQL_USER") or "root",
        password=os.getenv("MYSQL_PASSWORD", ""),
        database=os.getenv("MYSQL_DATABASE") or "RevealDQ",
        cursorclass=pymysql.cursors.DictCursor,
        autocommit=True,
    )


def _get_db():
    global _DB
    if _DB is not None:
        try:
            _DB.ping(reconnect=True)
            return _DB
        except Exception:  # noqa: BLE001
            try:
                _DB.close()
            except Exception:  # noqa: BLE001
                pass
            _DB = None
    _DB = _connect()
    return _DB


def tool_list_tables() -> str:
    db = _get_db()
    with db.cursor() as cur:
        cur.execute("SHOW TABLES")
        rows = cur.fetchall()
        if not rows:
            return json.dumps([])
        key = list(rows[0].keys())[0]
        return json.dumps([r[key] for r in rows])


def tool_describe_table(table: str) -> str:
    if not _IDENT.match(table or ""):
        return json.dumps({"error": "invalid table name"})
    db = _get_db()
    with db.cursor() as cur:
        cur.execute(f"DESCRIBE `{table}`")
        return json.dumps(cur.fetchall(), default=str)


def tool_run_select(sql: str, limit: int = 50) -> str:
    try:
        cleaned = assert_readonly_select(sql)
    except ValueError as exc:
        return json.dumps({"error": str(exc)})
    if " limit " not in cleaned.lower():
        cleaned = f"{cleaned} LIMIT {int(limit)}"
    db = _get_db()
    with db.cursor() as cur:
        cur.execute(cleaned)
        return json.dumps(cur.fetchall(), default=str)


def _dispatch_tool(name: str, args: dict) -> dict:
    try:
        if name == "list_tables":
            text = tool_list_tables()
        elif name == "describe_table":
            text = tool_describe_table(args.get("table", ""))
        elif name == "run_select":
            text = tool_run_select(args.get("sql", ""), int(args.get("limit", 50) or 50))
        else:
            return {
                "content": [{"type": "text", "text": json.dumps({"error": f"unknown tool {name}"})}],
                "isError": True,
            }
    except Exception as exc:  # noqa: BLE001 - report tool failures over MCP, don't crash
        return {
            "content": [{"type": "text", "text": json.dumps({"error": str(exc)})}],
            "isError": True,
        }
    return {"content": [{"type": "text", "text": text}], "isError": False}


def _handle(method: str, params: dict) -> dict:
    if method == "initialize":
        return {
            "protocolVersion": PROTOCOL_VERSION,
            "capabilities": {"tools": {}},
            "serverInfo": SERVER_INFO,
        }
    if method == "tools/list":
        return {"tools": TOOLS}
    if method == "tools/call":
        return _dispatch_tool(params.get("name", ""), params.get("arguments") or {})
    raise KeyError(method)


def _write(msg: dict) -> None:
    sys.stdout.write(json.dumps(msg) + "\n")
    sys.stdout.flush()


def main() -> None:
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            request = json.loads(line)
        except json.JSONDecodeError:
            continue
        method = request.get("method")
        req_id = request.get("id")
        # Notifications have no id and expect no response.
        if req_id is None:
            continue
        try:
            result = _handle(method, request.get("params") or {})
            _write({"jsonrpc": "2.0", "id": req_id, "result": result})
        except KeyError:
            _write(
                {
                    "jsonrpc": "2.0",
                    "id": req_id,
                    "error": {"code": -32601, "message": f"method not found: {method}"},
                }
            )
        except Exception as exc:  # noqa: BLE001
            _write(
                {
                    "jsonrpc": "2.0",
                    "id": req_id,
                    "error": {"code": -32603, "message": str(exc)},
                }
            )


if __name__ == "__main__":
    main()
