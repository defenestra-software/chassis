#!/bin/bash
# Build script for Chassis Flatpak bundle
# Creates a distributable .flatpak file for GitHub releases or direct download

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
APP_ID="io.defenestra.Chassis"
MANIFEST="${SCRIPT_DIR}/${APP_ID}.yml"

# Default values
VERSION=""
BUILD_DIR="${PROJECT_ROOT}/dist/flatpak-build"
REPO_DIR="${PROJECT_ROOT}/dist/flatpak-repo"
OUTPUT_DIR="${PROJECT_ROOT}/dist"
INSTALL_DEPS=false
INSTALL_LOCAL=false
GPG_KEY=""

usage() {
    cat << EOF
Usage: $(basename "$0") [OPTIONS]

Build a distributable Flatpak bundle for Chassis.

Options:
    -v, --version VERSION   Version string for bundle filename (e.g., 1.0.0)
                            If not specified, uses 'latest'
    -o, --output DIR        Output directory for the bundle (default: dist/)
    -d, --install-deps      Install Flatpak SDK and runtime if missing
    -i, --install           Install the Flatpak locally after building
    -g, --gpg-key KEY       GPG key ID for signing the bundle
    -c, --clean             Clean build directories before building
    -h, --help              Show this help message

Examples:
    $(basename "$0") -v 1.0.0                    # Build version 1.0.0
    $(basename "$0") -v 1.0.0 -i                 # Build and install locally
    $(basename "$0") -v 1.0.0 -g ABCD1234        # Build with GPG signing
    $(basename "$0") -c -v 1.0.0                 # Clean build

Output:
    Creates chassis-VERSION.flatpak in the output directory
EOF
    exit 0
}

log() {
    echo "[$(date '+%H:%M:%S')] $*"
}

error() {
    echo "[ERROR] $*" >&2
    exit 1
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -v|--version)
            VERSION="$2"
            shift 2
            ;;
        -o|--output)
            OUTPUT_DIR="$2"
            shift 2
            ;;
        -d|--install-deps)
            INSTALL_DEPS=true
            shift
            ;;
        -i|--install)
            INSTALL_LOCAL=true
            shift
            ;;
        -g|--gpg-key)
            GPG_KEY="$2"
            shift 2
            ;;
        -c|--clean)
            log "Cleaning build directories..."
            rm -rf "${BUILD_DIR}" "${REPO_DIR}"
            shift
            ;;
        -h|--help)
            usage
            ;;
        *)
            error "Unknown option: $1"
            ;;
    esac
done

# Set version for filename
if [ -z "$VERSION" ]; then
    VERSION="latest"
fi

BUNDLE_NAME="chassis-${VERSION}.flatpak"
BUNDLE_PATH="${OUTPUT_DIR}/${BUNDLE_NAME}"

# Write version file so the Flatpak build can embed it via ldflags.
echo "$VERSION" > "${PROJECT_ROOT}/.version"

# Check for flatpak-builder
if ! command -v flatpak-builder &> /dev/null; then
    error "flatpak-builder not found. Install it with: sudo dnf install flatpak-builder"
fi

# Check for manifest
if [ ! -f "$MANIFEST" ]; then
    error "Manifest not found: $MANIFEST"
fi

# Install dependencies if requested
if [ "$INSTALL_DEPS" = true ]; then
    log "Installing Flatpak SDK and runtime..."
    flatpak install -y flathub org.gnome.Platform//49 org.gnome.Sdk//49 org.freedesktop.Sdk.Extension.golang//24.08 || true
fi

# Check for required runtime
if ! flatpak info org.gnome.Platform//49 &> /dev/null; then
    error "GNOME Platform 49 not installed. Run with -d to install dependencies, or:
    flatpak install flathub org.gnome.Platform//49 org.gnome.Sdk//49"
fi

# Ensure output directory exists
mkdir -p "$OUTPUT_DIR"

log "Building Flatpak application..."
log "  App ID:  $APP_ID"
log "  Version: $VERSION"
log "  Output:  $BUNDLE_PATH"

# Update metainfo XML with the current version and date
METAINFO="${SCRIPT_DIR}/${APP_ID}.metainfo.xml"
if [ "$VERSION" != "latest" ] && [ -f "$METAINFO" ]; then
    TODAY=$(date '+%Y-%m-%d')
    sed -i "s/<release version=\"[^\"]*\" date=\"[^\"]*\">/<release version=\"${VERSION}\" date=\"${TODAY}\">/" "$METAINFO"
    log "Updated metainfo version to ${VERSION} (${TODAY})"
fi

# Build the application (cache stays in deploy/flatpak for faster rebuilds)
cd "$SCRIPT_DIR"
flatpak-builder --disable-rofiles-fuse --state-dir="${SCRIPT_DIR}/.flatpak-builder" --force-clean "$BUILD_DIR" "$MANIFEST"

log "Exporting to local repository..."
flatpak-builder --disable-rofiles-fuse --state-dir="${SCRIPT_DIR}/.flatpak-builder" --repo="$REPO_DIR" --force-clean "$BUILD_DIR" "$MANIFEST"

log "Creating bundle..."
BUNDLE_ARGS=()
if [ -n "$GPG_KEY" ]; then
    log "Signing with GPG key: $GPG_KEY"
    BUNDLE_ARGS+=(--gpg-sign="$GPG_KEY")
fi

flatpak build-bundle "$REPO_DIR" "$BUNDLE_PATH" "$APP_ID" "${BUNDLE_ARGS[@]}"

log "Bundle created: $BUNDLE_PATH"
ls -lh "$BUNDLE_PATH"

# Install locally if requested
if [ "$INSTALL_LOCAL" = true ]; then
    log "Installing Flatpak locally (user installation)..."
    flatpak install --user -y --reinstall "$BUNDLE_PATH"
    log "Installed. Run with: flatpak run $APP_ID"
    echo ""
    echo "Note: For system-wide installation (recommended for daemon access), use:"
    echo "  sudo flatpak install $BUNDLE_PATH"
fi

log "Done!"
echo ""
echo "To install on another system:"
echo "  sudo flatpak install ${BUNDLE_NAME}    # System-wide (recommended)"
echo "  flatpak install --user ${BUNDLE_NAME}  # User only"
echo ""
echo "To run:"
echo "  flatpak run ${APP_ID}"
