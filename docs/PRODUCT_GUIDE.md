# Reality Reveal — Product Guide

**Authored by Harish Kumar.** Built with assistance from AI models in Cursor.

**Reveal the reality behind your enterprise — one product at a time.**

This guide walks through each screen of the Reality Reveal platform and its product modules: what each is for, what you can do on it, and how the pieces fit together.

---

## Platform at a glance

Reality Reveal is a **multi-product** agentic platform. Sign in once, then create **customer-scoped projects** for the product module you need:

| Module | Status | Purpose |
|--------|--------|---------|
| **Reveal DQ** | Live | Enterprise data quality. Connect a **MySQL** warehouse, chat with the **DQ Analyser Agent** (Claude + MCP), turn findings into **governed DQ rules**, run them as **stored-procedure pipelines**, and watch outcomes on an **operations dashboard**. |
| **Reveal VoC** | Preview | Voice of Customer. Project scaffold, data sources and access control are live today; signal ingestion, the VoC Analyser Agent, theme/sentiment engine and impact dashboard arrive with the VoC specification. |

The project shell — left explorer, banner and default landing screen — adapts to the module the project belongs to. Sections 4–7 below describe Reveal DQ screens; section 11 covers Reveal VoC.

| Layer | Technology |
|-------|------------|
| UI | Angular |
| API | Go (Chi) |
| Agent | Python FastAPI + Claude |
| Warehouse (sample) | MySQL `RevealDQ` |
| Platform metadata | MySQL `RevealDQ_ctrl` |
| Jobs | MySQL stored procedures |
| IDE DB tools | MySQL MCP (`mcp/`) |

---

## 1. Landing & sign-in

![Reveal DQ landing page (light mode)](images/landing-light.png)

![Reveal DQ landing page (dark mode)](images/landing-dark.png)

**Purpose:** Introduce the product and authenticate users.

**What you see**
- Product name, logo, and short definition of Reveal DQ
- Highlights for the six DQ dimensions, MCP connectivity, and SQL-backed rules
- Capability cards: conversational analysis, ruled lifecycle, executable pipelines
- Sign-in / register form on the right
- Theme toggle (light / dark)

**What you can do**
- Sign in with your account (default admin after first API start: `admin@revealdq.local`)
- Register a new account
- Switch light/dark theme
- If already signed in, continue into workspaces or **Sign out** back to this page

---

## 2. Projects (Home)

![Project workspaces](images/workspaces.png)

**Purpose:** Create and open customer-scoped projects for a chosen product module. Every artefact (connections, rules, pipelines, runs) stays inside its project.

**What you see**
- Collapsible dark **explorer** ribbon listing your projects, grouped by product module
- Top banner with home, theme toggle, user avatar, and a visible **Sign out** button
- **New project** form with a **product-module picker** (Reveal DQ · Reveal VoC), then name, customer, description
- Filter chips (All / Reveal DQ / Reveal VoC) with counts
- Project cards with a product badge and role badge (e.g. ADMIN)

**What you can do**
- Pick a module and create a project for it (modules marked *Coming soon* cannot be selected)
- Open a project into its module-specific shell
- Sign out to return to the landing page

---

## 3. Project shell (module-driven)

Once you open a project, the left ribbon renders the navigation for that project's module.

**Reveal DQ project** (eyebrow: *Reveal DQ · Control Plane · Data Plane*)

| Section | Screens |
|---------|---------|
| **Management** | DQ Analyser Agent, DQ Rules Engine, ETL Analyser, ETL Code Conversion Agent |
| **Operations** | Dashboard, DQ Pipelines |
| **Config** | Data sources |
| **Settings** | Security & access control |

**Reveal VoC project** (eyebrow: *Reveal VoC · Customer Signal Plane*, violet accent)

| Section | Screens |
|---------|---------|
| **Voice of Customer** | Overview |
| **Config** | Data sources |
| **Settings** | Security & access control |

Opening a bare project URL lands on the module's default screen. A route from another module (for example a DQ screen inside a VoC project) is redirected back to the module home.

The banner always shows the project name, home link, theme toggle, avatar, **Sign out**, and the Reality Reveal logo; the ribbon shows the product lockup (Reveal DQ / Reveal VoC).

---

## 4. DQ Analyser Agent

![DQ Analyser Agent](images/analyser.png)

**Purpose:** Conversational data-quality analysis against the connected MySQL warehouse via MCP tools and Claude.

**Layout**
- **Left:** Stateful chat (conversation persists when you switch tabs)
- **Top right:** Rule recommendations from the agent
- **Bottom right:** Data preview (query results tied to the question)

**Toolbar**
- **MCP status** (green when connected) — hover for MySQL host / database details
- **LLM picker** — Claude Sonnet (default) or a cheaper Haiku option

**What you can do**
- Ask questions in plain language (completeness, validity, rating gaps, invoice leakage, etc.)
- Review recommended rules with:
  - Business & technical definitions
  - DQ dimension
  - Impacted tables / columns
  - Supporting SQL (monospace code block)
- **Save to Rules Engine** to persist a recommendation as a draft rule

---

## 5. DQ Rules Engine

**Purpose:** Govern the lifecycle of DQ rules produced by the analyser (or seeded later).

**What you can do**
- Browse rules with dimension, status, impact, and SQL check
- Move rules through **DRAFT → APPROVED → ACTIVE** (or retire active ones)
- Active rules are available for pipeline / job execution

---

## 6. Operations — Dashboard

**Purpose:** Summarise DQ execution health for the current workspace.

**What you see**
- Metrics: total runs, passed, failed, errors, draft vs active rules
- Breakdown by DQ dimension (runs / failed)

Use this after running pipelines to see whether checks are cleaning up or still failing.

---

## 7. Operations — DQ Pipelines

**Purpose:** Execute warehouse DQ jobs implemented as MySQL stored procedures.

**What you can do**
- List available pipelines (procedure name + description)
- **Run** a job on demand
- Inspect recent runs (status, rows checked / failed, message)

Sample procedures in this MVP cover completeness, consistency, and validity checks on the telco demo data.

---

## 8. Config — Data sources

**Purpose:** Connect the workspace to MySQL so the analyser and jobs have a target.

**What you can do**
- Add a MySQL connection (host, port, database, credentials)
- List configured connections
- **Test** a connection (table count / connectivity check)

For the MVP, MySQL is the supported source (e.g. local `RevealDQ`).

---

## 9. Settings — Security & access

**Purpose:** Control who can view or edit which screens (role × screen ACL matrix).

**What you can do**
- Toggle **View** / **Edit** per role and screen
- Persist ACL changes for the workspace

---

## 10. Reveal DQ — ETL Analyser & ETL Code Conversion Agent

**ETL Analyser** parses SQL scripts and Informatica mappings into metrics, a step list and an interactive flow graph; follow-up questions and node explanations use Claude when a key is configured.

**ETL Code Conversion Agent** analyses legacy SQL / Informatica / SSIS, converts to Databricks SQL or PySpark, validates the output, and can check the result into git and deploy / run it on Databricks.

---

## 11. Reveal VoC — Voice of Customer from LinkedIn

**Purpose:** Hear what customers say about the companies you care about on LinkedIn, classify it, and turn it into actions — on a schedule.

All VoC data lives in its own MySQL database, **`VOC_DB`** (created automatically on API start). Extraction is deterministic automation — a **credential-less Playwright scrape of LinkedIn's public pages** inside a date window (default: the last 5 days); only the analysis is agentic.

**What the scraper can and cannot see.** Without a LinkedIn session it reads the company page (roughly the ten latest posts and reposts, dated exactly from the activity id), each post's public page (body, author, engagement and the visible comments, via LinkedIn's schema.org JSON-LD), and third-party posts naming the company found through a web search index (best-effort — indexes lag by days to weeks). It cannot see LinkedIn search, hashtag feeds, the `/posts` tab or deep comment threads. Runs are throttled and stop at LinkedIn's sign-in wall; the run message says how far they got. Automated access is restricted by LinkedIn's User Agreement — keep volumes small.

### 11.1 Overview (`voc/overview`)
- KPIs: signals stored (posts / comments), **happy customer voice %** (positive ÷ customer voice, broadcasts excluded), negative signals with high-urgency count, open actions
- Sentiment split bar (Happy · Negative · Neutral · Broadcast), 21-day stacked timeline, "what they talk about" categories and top themes
- Per-company table (signals, happy, negative, happiness ratio), open actions, latest signals
- **Automation** card: connected sources, last run, schedule, analysis coverage; "Refresh now"

### 11.2 Companies (`voc/companies`)
- **Scrape window**: presets (last 5 / 7 / 14 / 30 days) or any two dates up to 90 days; applies to *Scrape all now*, per-company *Scrape* and the first scrape of a new company. Scheduled refreshes ignore it and run incrementally from the last sync.
- Add a company: name (used to find third-party posts), official LinkedIn handle **or page URL** (normalised to the vanity name), hashtags customers use; optionally scrape the selected window immediately
- Per company: signal count, last scrape, **Scrape**, Pause / Resume (excluded from the scheduler), Remove (deletes its signals, analysis, actions and runs)
- After a run, the scraper's notes are shown (posts in range, pages opened, comments found, whether discovery was rate-limited)

### 11.3 Signals (`voc/signals`)
- Every raw post / comment with author, headline, engagement, hashtags and a link to LinkedIn
- Agent classification per signal: sentiment tag with score, category, urgency, themes; expand for the one-line summary, suggested response, engine and external id
- Filters (company, sentiment, category, type, text search) are kept in the URL; **Re-analyse** drops and recomputes the classification

### 11.4 Actions (`voc/actions`)
- Board: Open → In progress → Done, plus Dismissed
- Agent actions are grouped by root cause (`dedupe_key`), carry evidence (signal ids) and a signal count; re-syncs merge new evidence instead of duplicating
- Manual actions, owner and priority edits

### 11.5 VoC Analyser Agent (`voc/analyser`)
- Ask in plain language ("what are customers most unhappy about this month?"). The API packages stats, a sample of classified signals and the action list; the agent answers with numbers, quotes and next steps. Scope by company; choose the Claude model.

### 11.6 Sources & sync (`voc/sources`) — the automation panel
- **Scrape window** (top): same presets / dates as Companies, applied to *Scrape all companies now*.
- **LinkedIn — public pages (Playwright scraper)**: the default. Created automatically on the first scrape; no credentials, no API keys. Headless Chromium reads the company feed, third-party posts naming the company or its hashtags, and the comments under them, inside the window.
- **LinkedIn — data provider endpoint**: any HTTP endpoint returning items in the Reveal VoC format (`{handle} {hashtags} {company} {since} {until}` placeholders, optional auth header) — for a commercial listening service or your own crawler.
- *LinkedIn official API (OAuth)* remains supported in the backend (`OFFICIAL_API` source) but is no longer offered in the picker.
- **Manual import**: paste or upload a JSON array of signals for a company.
- **Sample generator**: realistic synthetic LinkedIn signals, deterministic per company, with a fresh batch every 6 hours so scheduled refreshes visibly bring new items.
- **Periodic refresh**: enable + cadence (hourly → weekly). The API scheduler scrapes every active company incrementally (from the last sync minus 48 h), analyses new signals and refreshes actions.
- **Run history**: every manual / scheduled run with provider, status, fetched / new / analysed / actions and message.

**Sentiment classes** — `POSITIVE` (happy), `NEGATIVE`, `NEUTRAL` (questions, comparisons), `BROADCAST` (the company or a partner broadcasting information — not a customer voice).

---

## Sign-out behaviour

![Landing page after sign out](images/landing-after-logout.png)

**Sign out** is available on every authenticated banner. It clears the session and returns you to the landing page so you can share the product introduction without staying inside a workspace.

---

## Typical first run

1. Apply SQL and start API, agent, and UI (see root [README](../README.md)).
2. Sign in on the Reality Reveal landing page.
3. **New project** → pick **Reveal DQ** → name it → create.
4. **Config** → add / test MySQL connection.
5. **DQ Analyser Agent** → ask questions → save recommended rules.
6. **DQ Rules Engine** → approve / activate rules.
7. **DQ Pipelines** → run jobs.
8. **Dashboard** → review pass / fail statistics.
9. Back on **Projects**, create a **Reveal VoC** project to see the module-specific shell.

---

## Related docs

- [README.md](../README.md) — setup, stack, and how to run locally
- `sql/` — control-plane schema, warehouse seed, and DQ procedures
- `mcp/` — MySQL MCP configuration for IDE / agent tooling
