#!/bin/bash
# Chassis daemon launcher for systemd
# This script runs the daemon using Flatbox to access the Flatpak runtime
# while having full host system access for sysfs operations.

set -e

SCRIPT_DIR="$(dirname "$(readlink -f "$0")")"
FLATBOX_BIN="${SCRIPT_DIR}/flatbox"
APP_ID="io.defenestra.Chassis"

# Determine Flatpak installation path
# Check system installation first (preferred for daemon access)
if [ -d "/var/lib/flatpak/app/${APP_ID}" ]; then
    FLATPAK_INSTALL_PATH="/var/lib/flatpak"
# Fall back to user installation if FLATPAK_INSTALL_USER is set
elif [ -n "$FLATPAK_INSTALL_USER" ]; then
    USER_FLATPAK_DIR="/home/${FLATPAK_INSTALL_USER}/.local/share/flatpak"
    if [ -d "${USER_FLATPAK_DIR}/app/${APP_ID}" ]; then
        FLATPAK_INSTALL_PATH="$USER_FLATPAK_DIR"
    fi
fi

if [ -z "$FLATPAK_INSTALL_PATH" ]; then
    # Flatpak was uninstalled - clean up the service and remove ourselves
    echo "Flatpak ${APP_ID} not found - uninstalling daemon service"
    SERVICE_NAME="chassis.service"
    UNIT_PATH="/etc/systemd/system/${SERVICE_NAME}"

    # Stop and disable the service (ignore errors since we may be the running service)
    systemctl disable "${SERVICE_NAME}" 2>/dev/null || true

    # Remove the unit file and D-Bus policy
    rm -f "${UNIT_PATH}"
    rm -f "/etc/dbus-1/system.d/${APP_ID}.conf"

    # Reload systemd and D-Bus
    systemctl daemon-reload
    systemctl reload dbus 2>/dev/null || true

    echo "Daemon service uninstalled successfully"
    exit 0
fi

# Set environment for the daemon
export FLATBOX_ENV=1

# Run the daemon using Flatbox with access to the host system bus
exec "$FLATBOX_BIN" run \
    --flatpak-install-path "$FLATPAK_INSTALL_PATH" \
    --app "$APP_ID" \
    chassis-daemon
