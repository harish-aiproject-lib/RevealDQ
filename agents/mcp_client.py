"""
MCP stdio client bridge for the Reveal DQ Analyser Agent.

Spawns the Reveal DQ MySQL MCP server (mcp_server.py) and talks to it over the
Model Context Protocol stdio transport: JSON-RPC 2.0 with newline-delimited
messages. Implemented with the standard library only so it runs on the project's
Python 3.9 runtime, while remaining faithful to the MCP wire protocol
(`initialize` -> `notifications/initialized` -> `tools/list` / `tools/call`).

Each request opens a session for its lifetime and reuses it across the many
queries a whole-database scan performs, then closes it.
"""

from __future__ import annotations

import json
import os
import select
import subprocess
import sys
import threading
from typing import Any, Optional


class McpUnavailable(RuntimeError):
    """Raised when the MCP session cannot be established or used."""


def _parse_tool_result(result: Any) -> Any:
    """Flatten an MCP tools/call result into a Python value (JSON-decoded when possible)."""
    parts: list[str] = []
    for block in (result or {}).get("content", []) or []:
        if isinstance(block, dict) and block.get("type") == "text":
            parts.append(block.get("text", ""))
    text = "\n".join(parts).strip()
    if not text:
        return None
    try:
        return json.loads(text)
    except json.JSONDecodeError:
        return text


class McpBridge:
    """Owns a subprocess MCP server + a synchronous JSON-RPC stdio session."""

    def __init__(self, env: dict[str, str], server_path: Optional[str] = None):
        self._env = env
        self._server_path = server_path or os.path.join(
            os.path.dirname(__file__), "mcp_server.py"
        )
        self._proc: Optional[subprocess.Popen] = None
        self._next_id = 0
        self._lock = threading.Lock()

    def start(self, timeout: float = 25.0) -> None:
        try:
            self._proc = subprocess.Popen(
                [sys.executable, self._server_path],
                stdin=subprocess.PIPE,
                stdout=subprocess.PIPE,
                stderr=subprocess.DEVNULL,
                env={**os.environ, **self._env},
                text=True,
                bufsize=1,
            )
        except Exception as exc:  # noqa: BLE001
            raise McpUnavailable(f"could not launch MCP server: {exc}") from exc

        self._request(
            "initialize",
            {
                "protocolVersion": "2024-11-05",
                "capabilities": {},
                "clientInfo": {"name": "reveal-dq-analyser", "version": "0.2.0"},
            },
            timeout=timeout,
        )
        self._notify("notifications/initialized")

    def _writeline(self, msg: dict) -> None:
        if not self._proc or not self._proc.stdin:
            raise McpUnavailable("MCP server not running")
        try:
            self._proc.stdin.write(json.dumps(msg) + "\n")
            self._proc.stdin.flush()
        except Exception as exc:  # noqa: BLE001
            raise McpUnavailable(f"failed to write to MCP server: {exc}") from exc

    def _readline(self, timeout: float) -> str:
        if not self._proc or not self._proc.stdout:
            raise McpUnavailable("MCP server not running")
        ready, _, _ = select.select([self._proc.stdout], [], [], timeout)
        if not ready:
            raise McpUnavailable("timed out waiting for MCP server response")
        line = self._proc.stdout.readline()
        if line == "":
            raise McpUnavailable("MCP server closed the connection")
        return line

    def _notify(self, method: str, params: Optional[dict] = None) -> None:
        self._writeline({"jsonrpc": "2.0", "method": method, "params": params or {}})

    def _request(self, method: str, params: dict, timeout: float = 45.0) -> dict:
        with self._lock:
            self._next_id += 1
            req_id = self._next_id
            self._writeline({"jsonrpc": "2.0", "id": req_id, "method": method, "params": params})
            # Read until we see the matching id (server emits one response per request).
            while True:
                line = self._readline(timeout)
                try:
                    msg = json.loads(line)
                except json.JSONDecodeError:
                    continue
                if msg.get("id") != req_id:
                    continue
                if "error" in msg:
                    raise McpUnavailable(f"MCP error: {msg['error']}")
                return msg.get("result") or {}

    def is_alive(self) -> bool:
        return self._proc is not None and self._proc.poll() is None

    def list_tool_names(self, timeout: float = 15.0) -> list[str]:
        res = self._request("tools/list", {}, timeout=timeout)
        return [t.get("name") for t in res.get("tools", [])]

    def call(self, tool: str, args: Optional[dict[str, Any]] = None, timeout: float = 45.0) -> Any:
        res = self._request("tools/call", {"name": tool, "arguments": args or {}}, timeout=timeout)
        return _parse_tool_result(res)

    def close(self) -> None:
        if not self._proc:
            return
        try:
            if self._proc.stdin:
                self._proc.stdin.close()
        except Exception:  # noqa: BLE001
            pass
        try:
            self._proc.terminate()
            self._proc.wait(timeout=5)
        except Exception:  # noqa: BLE001
            try:
                self._proc.kill()
            except Exception:  # noqa: BLE001
                pass
        self._proc = None
