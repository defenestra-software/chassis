# Chassis

A device manager for Linux. See your hardware, manage your drivers.

For more information, visit [defenestra.io](https://defenestra.io/products/chassis).

## Building

### Prerequisites

- Go 1.25+
- GTK4 and libadwaita development libraries
- [Task](https://taskfile.dev/)

Fedora:

```sh
sudo dnf install gtk4-devel libadwaita-devel task
```

Ubuntu/Debian:

```sh
sudo apt install libgtk-4-dev libadwaita-1-dev
```

### Build and Run

```sh
task build        # Build GUI and daemon
task run          # Build and run the GUI
task run:daemon   # Build and run the daemon (requires root)
```

### Flatpak

```sh
task flatpak:install   # Build and install locally
task flatpak:run       # Run the installed Flatpak
```

## License

GPL-3.0-only. See [LICENSE](LICENSE).
