#!/bin/bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
OUT_DIR="$(pwd)"

SPEC_SRC="${SCRIPT_DIR}/defenestra-chassis.spec"
NAME="defenestra-chassis"

cd "$REPO_ROOT"

git fetch --tags --force --unshallow 2>/dev/null || git fetch --tags --force

# Resolve version from git, trying multiple strategies (Copr-style
# shallow clones may have HEAD detached at a tag without describe info).
VERSION=""
for cmd in \
    "git describe --tags --exact-match HEAD" \
    "git describe --tags --abbrev=0" \
    "git tag --points-at HEAD"; do
    candidate=$(eval "$cmd" 2>/dev/null | head -1 | sed 's/^v//' || true)
    if [ -n "${candidate:-}" ]; then
        VERSION="$candidate"
        break
    fi
done

if [ -z "${VERSION:-}" ]; then
    echo "make_srpm: no git tag found on HEAD; refusing to fall back to spec Version" >&2
    echo "make_srpm: tag the release commit (e.g. 'git tag v0.0.1') and re-trigger the build" >&2
    exit 1
fi
echo "make_srpm: using git tag version: ${VERSION}"

SRCDIR="${NAME}-${VERSION}"
TARBALL="${SRCDIR}.tar.gz"

go mod vendor

WORK=$(mktemp -d)
trap 'rm -rf "$WORK"' EXIT

mkdir -p "${WORK}/${SRCDIR}"
tar --exclude='./.git' \
    --exclude='./dist' \
    --exclude='./.flatpak-builder' \
    --exclude='./deploy/flatpak/.flatpak-builder' \
    -cf - . | tar -xf - -C "${WORK}/${SRCDIR}"

(cd "$WORK" && tar czf "${OUT_DIR}/${TARBALL}" "$SRCDIR")

SPEC_OUT="${OUT_DIR}/$(basename "$SPEC_SRC")"
sed "s/^Version:.*/Version:        ${VERSION}/" "$SPEC_SRC" > "$SPEC_OUT"

cd "$OUT_DIR"
rpmbuild -bs \
    --define "_sourcedir ${OUT_DIR}" \
    --define "_srcrpmdir ${OUT_DIR}" \
    "$(basename "$SPEC_OUT")"
