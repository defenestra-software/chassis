#!/bin/bash
# Prepare the changelog for VERSION, commit it if needed, and create vX.Y.Z.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "${SCRIPT_DIR}/../.."

fail() {
    echo "release-tag: $*" >&2
    exit 1
}

[ $# -eq 1 ] || fail "usage: release-tag.sh VERSION"
version="${1#v}"
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || fail "'$1' is not a version of the form x.y.z"
tag="v${version}"

git rev-parse -q --verify "refs/tags/${tag}" >/dev/null && fail "tag ${tag} already exists"

latest=$(git describe --tags --abbrev=0 2>/dev/null || true)
if [ -n "$latest" ]; then
    newest=$(printf '%s\n%s\n' "${latest#v}" "$version" | sort -V | tail -n1)
    [ "$newest" = "$version" ] && [ "${latest#v}" != "$version" ] ||
        fail "${version} is not newer than the latest tag ${latest}"
fi

other=$(git status --porcelain --untracked-files=no | grep -v -E '^.. CHANGELOG\.md$' || true)
if [ -n "$other" ]; then
    echo "$other" >&2
    fail "commit or stash the changes above first"
fi

"${SCRIPT_DIR}/changelog.sh" roll "$version" "$(date +%F)"
"${SCRIPT_DIR}/changelog.sh" check "$version"

git add CHANGELOG.md
if git diff --cached --quiet; then
    echo "release-tag: CHANGELOG.md already committed for ${version}"
else
    git commit -q -m "Release ${version}" -- CHANGELOG.md
    echo "release-tag: committed CHANGELOG.md as 'Release ${version}'"
fi
git tag "$tag"
echo "release-tag: created ${tag} at $(git rev-parse --short HEAD)"
echo
echo "Next:"
echo "  git push && git push origin ${tag}"
echo "  task release:notes   # release text in dist/release-notes.md"
