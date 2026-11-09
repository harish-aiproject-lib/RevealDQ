#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

if [[ -f "$ROOT/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT/.env"
  set +a
fi

# Playwright browsers (Reveal VoC LinkedIn scraper) live inside the project.
export PLAYWRIGHT_BROWSERS_PATH="${PLAYWRIGHT_BROWSERS_PATH:-$ROOT/.tools/playwright}"
if [[ ! -d "$PLAYWRIGHT_BROWSERS_PATH" ]]; then
  echo "Installing Chromium for the LinkedIn scraper into $PLAYWRIGHT_BROWSERS_PATH ..."
  "$ROOT/agents/.venv/bin/python" -m playwright install chromium
fi

cd "$ROOT/agents"
exec .venv/bin/uvicorn main:app --host 0.0.0.0 --port "${AGENT_PORT:-8090}"
