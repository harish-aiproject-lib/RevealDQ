# Reveal DQ

**the true health of enterprise data. Reveal the Reality Before you migrate**

Enterprise data-quality platform for analysing warehouse health before migration. This MVP focuses on project workspaces, MySQL connectivity, a Claude-powered DQ Analyser Agent, DQ rules, and stored-procedure pipelines.

**Authored by Harish Kumar.** Built with assistance from AI models in Cursor.

![Landing page](docs/images/landing-light.png)

> **UI walkthrough with screenshots:** see [docs/PRODUCT_GUIDE.md](docs/PRODUCT_GUIDE.md) — what each page does (landing, workspaces, analyser, rules, pipelines, dashboard, config, settings).

## Stack

| Layer | Technology |
|-------|------------|
| UI | Angular 19 |
| API | Go (Chi) |
| Agent | Python FastAPI + Claude |
| Warehouse sample | MySQL `RevealDQ` (telco CRM/billing) |
| Platform metadata | MySQL `RevealDQ_ctrl` |
| Jobs | MySQL stored procedures |
| IDE DB tools | MySQL MCP (`mcp/`) |

## Prerequisites

- Local MySQL with schema `RevealDQ` tables already created
- Fill `.env` from [`.env.example`](.env.example) (`MYSQL_PASSWORD`, optional `ANTHROPIC_API_KEY`)

Optional local toolchains are vendored under `.tools/` (Go, Node) if not installed system-wide.

## First-time setup

```bash
cp .env.example .env
# edit MYSQL_PASSWORD (and ANTHROPIC_API_KEY when ready)

chmod +x scripts/*.sh
./scripts/apply-sql.sh

# start API once so admin user is created, then optionally:
# /usr/local/mysql/bin/mysql -u root -p < sql/ctrl/003_demo_workspace.sql
```

Default login after API start:

- Email: `admin@revealdq.local`
- Password: `Admin@123` (override with `ADMIN_PASSWORD`)

## Run (three terminals)

```bash
./scripts/run-api.sh      # :8080
./scripts/run-agent.sh    # :8090
./scripts/run-ui.sh       # :4200
```

Open http://127.0.0.1:4200 → sign in → create a workspace → **Config** (MySQL) → **DQ Analyser Agent** → save rules → **DQ Pipelines** → **Dashboard**.

## Workspace boundary

Cursor Rule + Skill under `.cursor/` enforce: you can’t delete or add files outside of `/Users/vedantsubramanian/Documents/Harish/AgenticAI_Projects/AgenticDataGov`.

## Project layout

```
frontend/   Angular app
backend/    Go API
agents/     Python DQ Analyser
sql/        RevealDQ_ctrl + RevealDQ seed + DQ procs
mcp/        MySQL MCP config
scripts/    apply-sql / run-* helpers
docs/       Product guide + UI screenshots
```

## Documentation

| Doc | Contents |
|-----|----------|
| [docs/PRODUCT_GUIDE.md](docs/PRODUCT_GUIDE.md) | Screen-by-screen UI guide with screenshots |
| [README.md](README.md) | Setup and run instructions |
