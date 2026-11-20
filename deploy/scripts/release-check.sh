#!/bin/bash
# Check the tag at HEAD against CHANGELOG.md and reject changes to tracked files.
# Packaging uses the working tree, so tracked files must match the release commit.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}/../.."

fail() {
    echo "release-check: $*" >&2
    exit 1
}

tag=$(git describe --tags --exact-match HEAD 2>/dev/null) ||
    fail "HEAD is not tagged; run 'task tag VERSION=x.y.z' first"
version="${tag#v}"
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "tag $tag is not of the form vX.Y.Z"

if ! git diff --quiet || ! git diff --cached --quiet; then
    git status --short --untracked-files=no >&2
    fail "working tree has uncommitted changes"
fi

"${SCRIPT_DIR}/changelog.sh" check "$version"
echo "release-check: $tag matches CHANGELOG.md and the tree is clean"
