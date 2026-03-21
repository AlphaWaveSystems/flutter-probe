#!/bin/bash
# Initializes the GitHub Wiki from docs/wiki/ markdown files.
# Run this once after enabling the wiki on the repository.
#
# Usage: ./scripts/setup-wiki.sh
#
# Prerequisites:
#   - gh CLI authenticated with write access
#   - Wiki enabled in repo settings
#   - At least one page created manually in the wiki (to initialize the wiki repo)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WIKI_DIR="$REPO_ROOT/docs/wiki"
WIKI_REPO_URL="git@github.com:AlphaWaveSystems/flutter-probe.wiki.git"
TEMP_DIR=$(mktemp -d)

echo "Cloning wiki repository..."
git clone "$WIKI_REPO_URL" "$TEMP_DIR" 2>/dev/null || {
  echo "Error: Could not clone wiki repo."
  echo "Make sure you've created at least one wiki page via the GitHub UI first."
  echo "Go to: https://github.com/AlphaWaveSystems/flutter-probe/wiki"
  exit 1
}

echo "Copying wiki pages..."
cp "$WIKI_DIR"/*.md "$TEMP_DIR/"

cd "$TEMP_DIR"
git add -A
if git diff --cached --quiet; then
  echo "No changes to push."
else
  git commit -m "Update wiki from docs/wiki/"
  git push origin master
  echo "Wiki updated successfully."
fi

rm -rf "$TEMP_DIR"
