# Reveal DQ Analyser Agent

FastAPI service on port `8090`.

```bash
# from repo root
./scripts/run-agent.sh
```

- With `ANTHROPIC_API_KEY` set: Claude drives tool calls against MySQL.
- Without a key: offline heuristic analysis still returns completeness / consistency / validity rule recommendations from live SELECT checks.

Tool surface (MCP-compatible): `list_tables`, `describe_table`, `run_select`.
