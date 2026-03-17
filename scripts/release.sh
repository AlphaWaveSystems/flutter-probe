#!/usr/bin/env bash
set -euo pipefail

# Release script for FlutterProbe
# Usage: ./scripts/release.sh <version>
# Example: ./scripts/release.sh 0.2.0

VERSION="${1:-}"

if [ -z "$VERSION" ]; then
  echo "Usage: $0 <version>"
  echo "Example: $0 0.2.0"
  exit 1
fi

# Validate semver format (X.Y.Z)
if ! echo "$VERSION" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "Error: version must be in semver format X.Y.Z (e.g., 0.2.0)"
  exit 1
fi

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$REPO_ROOT"

# Check for uncommitted changes
if ! git diff --quiet || ! git diff --cached --quiet; then
  echo "Error: working tree has uncommitted changes. Please commit or stash first."
  exit 1
fi

# Check tag doesn't already exist
if git rev-parse "v$VERSION" >/dev/null 2>&1; then
  echo "Error: tag v$VERSION already exists."
  exit 1
fi

echo "Releasing FlutterProbe v$VERSION..."

# Update VERSION file
echo "$VERSION" > VERSION

# Update probe_agent/pubspec.yaml version field
sed -i '' "s/^version: .*/version: $VERSION/" probe_agent/pubspec.yaml

# Update vscode/package.json version field
sed -i '' "s/\"version\": \".*\"/\"version\": \"$VERSION\"/" vscode/package.json

echo "  Updated VERSION"
echo "  Updated probe_agent/pubspec.yaml"
echo "  Updated vscode/package.json"

# Commit and tag
git add VERSION probe_agent/pubspec.yaml vscode/package.json
git commit -m "Release v$VERSION"
git tag "v$VERSION"

echo ""
echo "Release v$VERSION created successfully."
echo ""
echo "To publish, run:"
echo "  git push origin main --tags"
