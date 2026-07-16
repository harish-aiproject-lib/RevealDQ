#!/usr/bin/env bash
# Smoke-check API + agent health endpoints (services must already be running).
set -euo pipefail
echo "API health:"
curl -sf http://127.0.0.1:8080/health | tee /dev/stderr; echo
echo "Agent health:"
curl -sf http://127.0.0.1:8090/health | tee /dev/stderr; echo
echo "OK"
