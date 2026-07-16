#!/bin/zsh
set -euo pipefail

PROJECT_DIR="/Users/vedantsubramanian/Documents/Harish/AgenticAI_Projects/AgenticDataGov"
GITHUB_USER="harish-aiproject-lib"
REPO_NAME="RevealDQ"
export PATH="$HOME/.local/bin:$PATH"

cd "$PROJECT_DIR"

if ! command -v git >/dev/null 2>&1; then
  echo "Git is not available. Install Xcode Command Line Tools first:"
  echo "  xcode-select --install"
  exit 1
fi

if ! command -v gh >/dev/null 2>&1; then
  echo "GitHub CLI (gh) not found on PATH (expected ~/.local/bin/gh)."
  exit 1
fi

if ! gh auth status >/dev/null 2>&1; then
  echo "GitHub CLI is not authenticated. Run:"
  echo "  gh auth login"
  exit 1
fi

if [ ! -d .git ]; then
  git init
  git branch -M main
fi

if ! git remote get-url origin >/dev/null 2>&1; then
  git remote add origin "https://github.com/${GITHUB_USER}/${REPO_NAME}.git"
fi

# Ensure remote is public
gh repo edit "${GITHUB_USER}/${REPO_NAME}" --visibility public 2>/dev/null || true

git push -u origin main
echo "Done. Repository: https://github.com/${GITHUB_USER}/${REPO_NAME}"
