"""Read-only SELECT gate. Mirrors backend/internal/sqlguard."""

from __future__ import annotations

import re

_BLOCK = re.compile(r"/\*.*?\*/", re.S)
_LINE = re.compile(r"--[^\n]*")
_STRING = re.compile(r"'(?:''|\\'|[^'])*'")
_FORBIDDEN = re.compile(
    r"\b(insert|update|delete|drop|alter|create|grant|revoke|truncate|call|execute|prepare|deallocate|handler)\b"
    r"|\binto\s+(outfile|dumpfile)\b"
    r"|\bfor\s+update\b"
    r"|\block\s+in\s+share\s+mode\b"
    r"|\bload_file\s*\("
    r"|\binto\s+@",
    re.I,
)


def assert_readonly_select(sql: str) -> str:
    """Return the comment-stripped statement, or raise ValueError."""
    cleaned = _LINE.sub(" ", _BLOCK.sub(" ", sql or ""))
    cleaned = cleaned.strip().rstrip("; \t\r\n")
    if not cleaned:
        raise ValueError("SQL is empty")
    if ";" in cleaned:
        raise ValueError("only a single SELECT statement is allowed")
    head = cleaned.lstrip().lower()
    if not (head.startswith("select") or head.startswith("with")):
        raise ValueError("only SELECT statements are allowed")
    masked = _STRING.sub("''", cleaned)
    if _FORBIDDEN.search(masked):
        raise ValueError("statement is not read-only")
    return cleaned
