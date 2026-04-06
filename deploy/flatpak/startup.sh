#!/bin/bash
# Chassis Flatpak startup script
# Checks for the system daemon and offers to install it if missing.
#
# Usage:
#   startup.sh - Normal startup (install daemon if needed, launch GUI)
#   startup.sh --uninstall-service - Uninstall the daemon service and exit

set -e

APP_ID="io.defenestra.Chassis"
SERVICE_NAME="chassis.service"
UNIT_PATH="/etc/systemd/system/${SERVICE_NAME}"
DBUS_NAME="io.defenestra.Chassis"

# Determine the privilege escalation command
get_root_cmd() {
    if flatpak-spawn --host command -v pkexec > /dev/null 2>&1; then
        echo "pkexec"
    elif flatpak-spawn --host command -v run0 > /dev/null 2>&1; then
        echo "run0 --pipe"
    else
        echo ""
    fi
}

# Uninstall the daemon service
uninstall_service() {
    local root_cmd
    root_cmd=$(get_root_cmd)

    if [ -z "$root_cmd" ]; then
        echo "Error: No privilege escalation method found (pkexec or run0)"
        return 1
    fi

    if ! flatpak-spawn --host test -f "${UNIT_PATH}"; then
        echo "Service ${SERVICE_NAME} is not installed"
        return 0
    fi

    echo "Uninstalling ${SERVICE_NAME}..."
    flatpak-spawn --host $root_cmd sh -c "
        systemctl stop ${SERVICE_NAME} 2>/dev/null || true
        systemctl disable ${SERVICE_NAME} 2>/dev/null || true
        rm -f ${UNIT_PATH}
        rm -f /etc/dbus-1/system.d/${DBUS_NAME}.conf
        systemctl daemon-reload
        systemctl reload dbus 2>/dev/null || true
    "

    echo "Service uninstalled successfully"
    return 0
}

# Main logic
main() {
    # Handle --uninstall-service flag
    if [ "$1" = "--uninstall-service" ]; then
        uninstall_service
        exit $?
    fi

    # Check if daemon is already running
    if /app/bin/chassis --check; then
        exec /app/bin/chassis "$@"
    fi

    # Daemon not running - check for unattended setup
    if [ "$CHASSIS_UNATTENDED_SETUP" = "true" ]; then
        # Let the Go setup code handle installation
        /app/bin/chassis --setup
        if [ $? -ne 0 ]; then
            echo "Error: Failed to set up daemon in unattended mode"
            exit 1
        fi
        exec /app/bin/chassis "$@"
    fi

    # Interactive setup - use native GTK4 setup screen
    /app/bin/chassis --setup
    if [ $? -ne 0 ]; then
        echo "User cancelled setup or setup failed"
        exit 0
    fi

    # Setup succeeded - launch the GUI
    exec /app/bin/chassis "$@"
}

main "$@"
