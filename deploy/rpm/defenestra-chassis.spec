Name:           defenestra-chassis
Version:        0.0.1
Release:        1%{?dist}
Summary:        Device manager for Linux

License:        GPL-3.0-only
URL:            https://gitlab.com/defenestra-software/chassis
Source0:        %{name}-%{version}.tar.gz

BuildRequires:  golang >= 1.25
BuildRequires:  gtk4-devel
BuildRequires:  libadwaita-devel
BuildRequires:  pkgconf-pkg-config
BuildRequires:  desktop-file-utils
BuildRequires:  libappstream-glib
BuildRequires:  /usr/bin/appstream-util
BuildRequires:  systemd-rpm-macros

Requires:       gtk4
Requires:       libadwaita
Requires:       hicolor-icon-theme
Requires:       dbus
%{?systemd_requires}

# Skip stripping debug info from a Go binary built with -s -w.
%global debug_package %{nil}

%description
Chassis is a graphical device manager for Linux. Browse and inspect
PCI/USB devices, GPUs, storage drives, sensors, displays, network
adapters, and more. Includes a privileged D-Bus daemon for driver
bind/unbind and other hardware operations.

%prep
%autosetup -p1

%build
export GOFLAGS="-mod=vendor -trimpath"
export CGO_ENABLED=1
go build \
    -ldflags "-s -w -X defenestra.io/chassis/internal/shared.Version=%{version}" \
    -o chassis ./cmd/chassis
go build \
    -ldflags "-s -w -X defenestra.io/chassis/internal/shared.Version=%{version}" \
    -o chassis-daemon ./cmd/chassis-daemon

%install
install -Dm755 chassis %{buildroot}%{_bindir}/chassis
install -Dm755 chassis-daemon %{buildroot}%{_libexecdir}/chassis-daemon

install -Dm644 api/io.defenestra.Chassis.conf \
    %{buildroot}%{_datadir}/dbus-1/system.d/io.defenestra.Chassis.conf
install -Dm644 deploy/share/dbus/io.defenestra.Chassis.service \
    %{buildroot}%{_datadir}/dbus-1/system-services/io.defenestra.Chassis.service
install -Dm644 deploy/share/systemd/chassis-daemon.service \
    %{buildroot}%{_unitdir}/chassis-daemon.service

install -Dm644 deploy/share/applications/io.defenestra.Chassis.desktop \
    %{buildroot}%{_datadir}/applications/io.defenestra.Chassis.desktop
install -Dm644 deploy/share/metainfo/io.defenestra.Chassis.metainfo.xml \
    %{buildroot}%{_datadir}/metainfo/io.defenestra.Chassis.metainfo.xml

install -Dm644 deploy/share/icons/hicolor/scalable/apps/io.defenestra.Chassis.svg \
    %{buildroot}%{_datadir}/icons/hicolor/scalable/apps/io.defenestra.Chassis.svg
for size in 48x48 64x64 128x128 256x256; do
    src="deploy/share/icons/hicolor/${size}/apps/io.defenestra.Chassis.png"
    if [ -f "$src" ]; then
        install -Dm644 "$src" \
            %{buildroot}%{_datadir}/icons/hicolor/${size}/apps/io.defenestra.Chassis.png
    fi
done

%check
desktop-file-validate \
    %{buildroot}%{_datadir}/applications/io.defenestra.Chassis.desktop
appstream-util validate-relax --nonet \
    %{buildroot}%{_datadir}/metainfo/io.defenestra.Chassis.metainfo.xml

%post
gtk-update-icon-cache %{_datadir}/icons/hicolor &>/dev/null || :
%systemd_post chassis-daemon.service

%preun
%systemd_preun chassis-daemon.service

%postun
gtk-update-icon-cache %{_datadir}/icons/hicolor &>/dev/null || :
%systemd_postun_with_restart chassis-daemon.service

%files
%license LICENSE
%{_bindir}/chassis
%{_libexecdir}/chassis-daemon
%{_datadir}/dbus-1/system.d/io.defenestra.Chassis.conf
%{_datadir}/dbus-1/system-services/io.defenestra.Chassis.service
%{_unitdir}/chassis-daemon.service
%{_datadir}/applications/io.defenestra.Chassis.desktop
%{_datadir}/metainfo/io.defenestra.Chassis.metainfo.xml
%{_datadir}/icons/hicolor/scalable/apps/io.defenestra.Chassis.svg
%{_datadir}/icons/hicolor/*/apps/io.defenestra.Chassis.png

%changelog
* Sun May 10 2026 defenestra Team <support@defenestra.io> - 0.0.1-1
- Initial RPM packaging.
