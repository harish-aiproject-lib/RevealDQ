#!/usr/bin/env bash
# Apply SQL schemas/seeds and DQ procedures to local MySQL.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
MYSQL_BIN="${MYSQL_BIN:-/usr/local/mysql/bin/mysql}"

if [[ -f "$ROOT/.env" ]]; then
  # shellcheck disable=SC1091
  set -a
  source "$ROOT/.env"
  set +a
fi

HOST="${MYSQL_HOST:-127.0.0.1}"
PORT="${MYSQL_PORT:-3306}"
USER="${MYSQL_USER:-root}"
PASS="${MYSQL_PASSWORD:-}"

ARGS=(-h "$HOST" -P "$PORT" -u "$USER")
if [[ -n "$PASS" ]]; then
  ARGS+=(-p"$PASS")
fi

echo "Applying Reality Reveal control-plane schema (RevealDQ_ctrl)..."
"$MYSQL_BIN" "${ARGS[@]}" < "$ROOT/sql/ctrl/001_schema.sql"
"$MYSQL_BIN" "${ARGS[@]}" < "$ROOT/sql/ctrl/002_seed.sql"
"$MYSQL_BIN" "${ARGS[@]}" < "$ROOT/sql/ctrl/004_rule_audit.sql"
"$MYSQL_BIN" "${ARGS[@]}" < "$ROOT/sql/ctrl/005_product_modules.sql"

echo "Seeding RevealDQ subject data..."
"$MYSQL_BIN" "${ARGS[@]}" < "$ROOT/sql/revealDQ/seed_data.sql"

echo "Installing DQ stored procedures..."
"$MYSQL_BIN" "${ARGS[@]}" < "$ROOT/sql/revealDQ/procs/sp_dq_cdr_unrated.sql"
"$MYSQL_BIN" "${ARGS[@]}" < "$ROOT/sql/revealDQ/procs/sp_dq_invoice_balance.sql"
"$MYSQL_BIN" "${ARGS[@]}" < "$ROOT/sql/revealDQ/procs/sp_dq_msisdn_validity.sql"

echo "Done. After API starts (creates admin), optionally run:"
echo "  $MYSQL_BIN ${ARGS[*]} < $ROOT/sql/ctrl/003_demo_workspace.sql"
