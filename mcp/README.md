# MySQL MCP server for Reveal DQ (Cursor / Claude Code)

Copy connection secrets from the project `.env` file.

## Cursor registration

1. Ensure Node is on PATH (project ships a local runtime under `.tools/node` if needed).
2. Merge [`mysql.mcp.json`](mysql.mcp.json) into your Cursor MCP settings, replacing `${MYSQL_PASSWORD}` with your local password.
3. Restart Cursor MCP servers.

Example URI:

```text
mysql://root:YOUR_PASSWORD@127.0.0.1:3306/RevealDQ
```

## Runtime note

The Python DQ Analyser Agent exposes an equivalent MySQL tool surface (`list_tables`, `describe_table`, `run_select`) so chat works even when MCP is only configured in the IDE.
