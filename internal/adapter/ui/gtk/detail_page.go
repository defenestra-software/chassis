package gtk

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"defenestra.io/chassis/internal/core/domain"

	kit "gitlab.com/defenestra-software/go-adw-kit"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/godbus/dbus/v5"
)

func (g *GtkUI) showDeviceDetail(node *domain.DeviceNode) {
	g.showDeviceDetailScrollTo(node, "")
}

func (g *GtkUI) showDeviceDetailScrollTo(node *domain.DeviceNode, scrollTo string) {
	content, scrollTargets := g.buildDetailContent(node)

	scroll := gtk.NewScrolledWindow()
	scroll.SetChild(content)
	scroll.SetVExpand(true)
	scroll.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)

	headerBar := adw.NewHeaderBar()
	kit.PatchHeaderBar(headerBar, g.window)

	toolbarView := adw.NewToolbarView()
	toolbarView.AddTopBar(headerBar)
	toolbarView.SetContent(scroll)

	page := adw.NewNavigationPage(toolbarView, node.DisplayName())
	// Tag with the device ID so a post-action refresh can tell whether this
	// exact device is still on screen before re-rendering it.
	page.SetTag(node.ID)
	g.navView.Push(page)
	g.onDetailPage = true

	// Auto-scroll to the target section after the page is realized.
	// We delay slightly to let GTK finish layout before scrolling.
	if scrollTo != "" {
		if target, ok := scrollTargets[scrollTo]; ok {
			glib.TimeoutSecondsAdd(0, func() bool {
				if w, ok := target.(*adw.PreferencesGroup); ok {
					alloc := w.Allocation()
					adj := scroll.VAdjustment()
					if adj != nil {
						adj.SetValue(float64(alloc.Y()))
					}
				} else if w, ok := target.(*gtk.Box); ok {
					alloc := w.Allocation()
					adj := scroll.VAdjustment()
					if adj != nil {
						adj.SetValue(float64(alloc.Y()))
					}
				}
				return false // Run once
			})
		}
	}
}

func (g *GtkUI) buildDetailContent(node *domain.DeviceNode) (gtk.Widgetter, map[string]gtk.Widgetter) {
	box := gtk.NewBox(gtk.OrientationVertical, 18)
	box.SetMarginTop(24)
	box.SetMarginBottom(24)
	box.SetMarginStart(24)
	box.SetMarginEnd(24)

	scrollTargets := make(map[string]gtk.Widgetter)

	titleBox := gtk.NewBox(gtk.OrientationHorizontal, 16)
	titleBox.SetMarginBottom(12)

	titleIcon := gtk.NewImageFromIconName(g.getIconName(node))
	titleIcon.SetIconSize(gtk.IconSizeLarge)
	titleBox.Append(titleIcon)

	titleLabel := gtk.NewLabel(node.DisplayName())
	titleLabel.AddCSSClass("title-1")
	titleLabel.SetHAlign(gtk.AlignStart)
	titleLabel.SetHExpand(true)
	titleLabel.SetWrap(true)
	titleLabel.SetWrapMode(2) // WORD_CHAR
	titleBox.Append(titleLabel)
	box.Append(titleBox)

	if len(node.Interfaces) > 0 {
		seen := map[string]bool{}
		for _, iface := range node.Interfaces {
			// Skip GPU display interfaces on the device page - they're for the
			// sidebar category only. The GPU's Graphics section already shows
			// Display Outputs with cross-links.
			if iface.Type == domain.InterfaceDisplay && node.GPUInfo != nil {
				continue
			}

			if seen[iface.ScrollTag] {
				continue
			}
			seen[iface.ScrollTag] = true

			section := g.buildInterfaceSection(node, iface)
			if section != nil {
				box.Append(section)
				scrollTargets[iface.ScrollTag] = section
			}
		}
	} else {
		box.Append(g.buildGeneralInfoGroup(node))
	}

	if node.IOMMUGroup >= 0 && node.Type == domain.DeviceTypePCI {
		box.Append(g.buildIOMMUGroupSection(node))
	}

	shownPaths := map[string]bool{}
	for _, iface := range node.Interfaces {
		for _, p := range iface.DevicePaths {
			shownPaths[p] = true
		}
	}
	var extraPaths []string
	for _, p := range node.DevicePaths {
		if !shownPaths[p] {
			extraPaths = append(extraPaths, p)
		}
	}
	if len(extraPaths) > 0 {
		pathsGroup := adw.NewPreferencesGroup()
		if len(extraPaths) == 1 {
			addDetailRow(pathsGroup, "Device", extraPaths[0])
		} else {
			expander := adw.NewExpanderRow()
			expander.SetTitle("Paths")
			expander.SetSubtitle(fmt.Sprintf("%d device paths", len(extraPaths)))
			for _, p := range extraPaths {
				row := adw.NewActionRow()
				row.SetTitle(p)
				row.SetSubtitleSelectable(true)
				expander.AddRow(row)
			}
			pathsGroup.Add(expander)
		}
		box.Append(pathsGroup)
	}

	if node.BatteryInfo != nil {
		box.Append(g.buildBatteryGroup(node))
	}

	if node.Type == domain.DeviceTypePCI || node.Type == domain.DeviceTypeUSB {
		box.Append(g.buildTechnicalDetailsGroup(node))
	}

	if node.Resources != nil && node.Type == domain.DeviceTypePCI {
		if resGroup := g.buildResourcesGroup(node.Resources); resGroup != nil {
			box.Append(resGroup)
		}
	}

	if g.driverMgmtEnabled && (node.Type == domain.DeviceTypePCI || node.Type == domain.DeviceTypeUSB) &&
		!strings.HasPrefix(node.ID, "usb:usb") {
		box.Append(g.buildActionButtons(node))
	}

	clamp := adw.NewClamp()
	clamp.SetMaximumSize(700)
	clamp.SetChild(box)

	return clamp, scrollTargets
}

func (g *GtkUI) buildInterfaceSection(node *domain.DeviceNode, iface *domain.DeviceInterface) gtk.Widgetter {
	switch {
	case iface.ProcessorInfo != nil:
		box := gtk.NewBox(gtk.OrientationVertical, 18)
		box.Append(g.buildProcessorInfoGroup(iface.ProcessorInfo))
		if cacheGroup := g.buildProcessorCacheGroup(iface.ProcessorInfo); cacheGroup != nil {
			box.Append(cacheGroup)
		}
		return box

	case iface.GPUInfo != nil:
		gpuBox := gtk.NewBox(gtk.OrientationVertical, 18)
		gpuBox.Append(g.buildGPUInfoGroup(node))
		if len(node.GPUInfo.Monitors) > 0 {
			gpuBox.Append(g.buildMonitorsGroup(node.GPUInfo))
		}
		return gpuBox

	case iface.StorageInfo != nil:
		box := gtk.NewBox(gtk.OrientationVertical, 18)
		box.Append(g.buildStorageInfoGroup(iface.StorageInfo))
		if iface.StorageInfo.SMART != nil {
			box.Append(g.buildSMARTGroup(iface.StorageInfo.SMART))
		}
		return box

	case iface.NetworkInfo != nil:
		return g.buildNetworkInfoGroup(node)

	case iface.DisplayInfo != nil:
		box := gtk.NewBox(gtk.OrientationVertical, 18)
		box.Append(g.buildDisplayInfoGroup(iface.DisplayInfo))
		if modesGroup := g.buildDisplayModesGroup(iface.DisplayInfo); modesGroup != nil {
			box.Append(modesGroup)
		}
		return box

	case iface.MotherboardInfo != nil:
		box := gtk.NewBox(gtk.OrientationVertical, 18)
		box.Append(g.buildMotherboardInfoGroup(iface.MotherboardInfo))
		if biosGroup := g.buildBIOSInfoGroup(iface.MotherboardInfo); biosGroup != nil {
			box.Append(biosGroup)
		}
		return box

	case iface.MemoryModule != nil:
		return g.buildMemoryModuleGroup(iface.MemoryModule)

	case len(iface.SensorData) > 0:
		sensorNode := &domain.DeviceNode{
			SensorReadings: iface.SensorData,
			Path:           node.Path,
		}
		return g.buildSensorReadingsGroupTitle(sensorNode, iface.SectionTitle)

	case iface.Type == domain.InterfaceDebug:
		group := adw.NewPreferencesGroup()
		group.SetTitle(iface.SectionTitle)
		addDetailRow(group, "Type", "Vendor-specific debug interface")
		if node.Driver != "" {
			addDetailRow(group, "Driver", node.Driver)
		}
		return group

	case len(iface.DevicePaths) > 0 && iface.Type == domain.InterfaceSerial:
		group := adw.NewPreferencesGroup()
		group.SetTitle(iface.SectionTitle)
		for _, p := range iface.DevicePaths {
			label := "Port"
			if strings.Contains(p, "/serial/by-id/") {
				label = "By ID"
			} else if strings.Contains(p, "/serial/by-path/") {
				label = "By Path"
			}
			addDetailRow(group, label, p)
		}
		return group

	default:
		if iface.Type == domain.InterfaceUSB && strings.HasPrefix(node.ID, "usb:usb") {
			group := adw.NewPreferencesGroup()
			group.SetTitle("USB Host Controller")
			addDetailRow(group, "Type", node.Name)
			addDetailRow(group, "Driver", node.Driver)
			return group
		}
		if strings.HasPrefix(node.ID, "bt-hid:") {
			return g.buildBluetoothHIDGroup(node)
		}
		return g.buildGeneralInfoGroup(node)
	}
}

func (g *GtkUI) buildMonitorsSection(node *domain.DeviceNode) gtk.Widgetter {
	if node.GPUInfo == nil || len(node.GPUInfo.Monitors) == 0 {
		return nil
	}
	return g.buildMonitorsGroup(node.GPUInfo)
}

func (g *GtkUI) buildProcessorInfoGroup(info *domain.ProcessorInfo) *adw.PreferencesGroup {
	group := adw.NewPreferencesGroup()
	group.SetTitle("Processor")

	addDetailRow(group, "Model", info.ModelName)

	vendor := info.Vendor
	switch vendor {
	case "AuthenticAMD":
		vendor = "AMD"
	case "GenuineIntel":
		vendor = "Intel"
	}
	addDetailRow(group, "Vendor", vendor)

	addDetailRow(group, "Architecture", info.Architecture)

	if info.Cores > 0 {
		addDetailRow(group, "Cores / Threads", info.CoreString())
	}

	if info.MaxFreqMHz > 0 {
		freqStr := info.FreqString()
		if info.MinFreqMHz > 0 {
			freqStr = fmt.Sprintf("%.0f MHz – %s", info.MinFreqMHz, freqStr)
		}
		addDetailRow(group, "Frequency", freqStr)
	}

	if info.BogoMIPS > 0 {
		addDetailRow(group, "BogoMIPS", fmt.Sprintf("%.2f", info.BogoMIPS))
	}

	if info.Family != "" {
		addDetailRow(group, "Family / Model / Stepping",
			fmt.Sprintf("%s / %s / %s", info.Family, info.Model, info.Stepping))
	}

	if info.Microcode != "" {
		addDetailRow(group, "Microcode", info.Microcode)
	}

	return group
}

func (g *GtkUI) buildProcessorCacheGroup(info *domain.ProcessorInfo) *adw.PreferencesGroup {
	summary := info.CacheSummary()
	if summary == "" {
		return nil
	}

	group := adw.NewPreferencesGroup()
	group.SetTitle("Cache")

	if info.CacheL1d > 0 {
		addDetailRow(group, "L1 Data", formatCacheSize(info.CacheL1d))
	}
	if info.CacheL1i > 0 {
		addDetailRow(group, "L1 Instruction", formatCacheSize(info.CacheL1i))
	}
	if info.CacheL2 > 0 {
		addDetailRow(group, "L2", formatCacheSize(info.CacheL2))
	}
	if info.CacheL3 > 0 {
		addDetailRow(group, "L3", formatCacheSize(info.CacheL3))
	}

	return group
}

func formatCacheSize(bytes uint64) string {
	if bytes >= 1024*1024 {
		return fmt.Sprintf("%d MB", bytes/(1024*1024))
	}
	return fmt.Sprintf("%d KB", bytes/1024)
}

func (g *GtkUI) buildDisplayInfoGroup(info *domain.DisplayInfo) *adw.PreferencesGroup {
	group := adw.NewPreferencesGroup()
	group.SetTitle("Display")

	addDetailRow(group, "Manufacturer", info.Vendor)
	addDetailRow(group, "Model", info.MonitorName)

	if sz := info.SizeString(); sz != "" {
		addDetailRow(group, "Screen Size", sz)
	}

	addDetailRow(group, "Resolution", info.CurrentMode)

	if r := info.RefreshString(); r != "" {
		addDetailRow(group, "Refresh Rate", r)
	}

	connStr := info.ConnectorType
	if info.ConnectorIndex != "" {
		connStr = fmt.Sprintf("%s-%s", connStr, info.ConnectorIndex)
	}
	addDetailRow(group, "Connection", connStr)
	gpuRow := adw.NewActionRow()
	gpuRow.SetTitle("GPU")
	gpuName := info.GPUCard
	cardIndex := strings.TrimPrefix(info.GPUCard, "card")
	g.mu.RLock()
	for _, dev := range g.deviceMap {
		if dev.GPUInfo != nil && fmt.Sprintf("%d", dev.GPUInfo.CardIndex) == cardIndex {
			gpuName = dev.DisplayName()
			break
		}
	}
	g.mu.RUnlock()
	gpuRow.SetSubtitle(gpuName)
	gpuRow.SetSubtitleSelectable(true)
	gpuRow.SetActivatable(true)
	gpuRow.ConnectActivated(func() {
		g.navigateToGPUByCard(cardIndex)
	})
	chevron := gtk.NewImageFromIconName("go-next-symbolic")
	chevron.AddCSSClass("dim-label")
	gpuRow.AddSuffix(chevron)
	group.Add(gpuRow)

	if info.Serial != "" && info.Serial != "0000000000001" {
		addDetailRow(group, "Serial", info.Serial)
	}

	if m := info.ManufactureString(); m != "" {
		addDetailRow(group, "Manufactured", m)
	}

	addDetailRow(group, "EDID Version", info.EDIDVersion)

	return group
}

func (g *GtkUI) buildDisplayModesGroup(info *domain.DisplayInfo) *adw.PreferencesGroup {
	if len(info.Modes) == 0 {
		return nil
	}

	group := adw.NewPreferencesGroup()

	expander := adw.NewExpanderRow()
	expander.SetTitle("Supported Resolutions")
	expander.SetSubtitle(fmt.Sprintf("%d resolutions", info.ModeCount))

	seen := map[string]bool{}
	for _, mode := range info.Modes {
		if seen[mode] {
			continue
		}
		seen[mode] = true
		row := adw.NewActionRow()
		row.SetTitle(mode)
		expander.AddRow(row)
	}

	group.Add(expander)
	return group
}

func (g *GtkUI) buildMotherboardInfoGroup(info *domain.MotherboardInfo) *adw.PreferencesGroup {
	group := adw.NewPreferencesGroup()
	group.SetTitle("Motherboard")

	addDetailRow(group, "Vendor", info.Vendor)
	addDetailRow(group, "Model", info.Product)
	if info.Version != "" && info.Version != "Not Applicable" {
		addDetailRow(group, "Version", info.Version)
	}
	addDetailRow(group, "Serial Number", info.SerialNumber)

	if info.TotalMemoryBytes > 0 {
		memStr := info.TotalMemoryString()
		if info.MemoryModules > 0 {
			memStr = fmt.Sprintf("%s (%d modules)", memStr, info.MemoryModules)
		}
		addDetailRow(group, "Total Installed Memory", memStr)
	}

	return group
}

func (g *GtkUI) buildBIOSInfoGroup(info *domain.MotherboardInfo) *adw.PreferencesGroup {
	if info.BIOSVendor == "" && info.BIOSVersion == "" {
		return nil
	}

	group := adw.NewPreferencesGroup()
	group.SetTitle("Firmware / BIOS")

	addDetailRow(group, "Vendor", info.BIOSVendor)
	addDetailRow(group, "Version", info.BIOSVersion)
	addDetailRow(group, "Date", info.BIOSDate)

	return group
}

func (g *GtkUI) buildMemoryModuleGroup(mod *domain.MemoryModuleInfo) *adw.PreferencesGroup {
	group := adw.NewPreferencesGroup()
	group.SetTitle("Memory Module")

	if mod.Size != "" {
		addDetailRow(group, "Capacity", mod.Size)
	}
	addDetailRow(group, "Slot", mod.Label)
	if mod.Type != "" && mod.Type != "Unknown" {
		addDetailRow(group, "Type", mod.Type)
	}
	if mod.Speed != "" && mod.Speed != "Unknown" {
		addDetailRow(group, "Speed", mod.Speed)
	}
	addDetailRow(group, "Vendor", mod.Vendor)
	if mod.PartNumber != "" {
		addDetailRow(group, "Part Number", mod.PartNumber)
	}
	addDetailRow(group, "Serial Number", mod.SerialNumber)
	if mod.Rank != "" && mod.Rank != "Unknown" {
		addDetailRow(group, "Rank", mod.Rank)
	}

	return group
}

func (g *GtkUI) buildNetworkInfoGroup(node *domain.DeviceNode) *adw.PreferencesGroup {
	net := node.NetworkInfo
	group := adw.NewPreferencesGroup()

	if net.IsBluetooth {
		group.SetTitle("Bluetooth")
	} else if net.IsWireless {
		group.SetTitle("Wireless")
	} else {
		group.SetTitle("Network")
	}

	addDetailRow(group, "Vendor", node.Vendor)
	addDetailRow(group, "Product", node.Product)

	addDetailRow(group, "Interface", net.InterfaceName)
	addDetailRow(group, "MAC Address", net.MACAddress)

	if net.State != "" && net.State != "unknown" {
		stateStr := net.State
		switch stateStr {
		case "up":
			stateStr = "Connected"
		case "down":
			stateStr = "Disconnected"
		case "dormant":
			stateStr = "Dormant"
		}
		addDetailRow(group, "Status", stateStr)
	}

	if net.IsWireless {
		ssidRow := addLiveRow(group, "Network (SSID)", net.SSID)
		signalRow := addLiveRow(group, "Signal Strength", net.SignalString())
		freqRow := addLiveRow(group, "Frequency", net.FrequencyString())
		bitrateRow := addLiveRow(group, "Bit Rate", net.BitRateString())
		securityRow := addLiveRow(group, "Security", net.SecurityType)

		var sourceID glib.SourceHandle
		group.ConnectMap(func() {
			sourceID = glib.TimeoutSecondsAdd(3, func() bool {
				w := pollWirelessNM(net.InterfaceName)
				if w == nil {
					return true
				}
				setLiveRow(ssidRow, w.SSID)
				setLiveRow(signalRow, w.SignalString())
				setLiveRow(freqRow, w.FrequencyString())
				setLiveRow(bitrateRow, w.BitRateString())
				setLiveRow(securityRow, w.SecurityType)
				return true
			})
		})
		group.ConnectUnmap(func() {
			if sourceID > 0 {
				glib.SourceRemove(sourceID)
				sourceID = 0
			}
		})
	}

	if net.Speed != "" && net.Speed != "-1" && net.Speed != "unknown" {
		addDetailRow(group, "Link Speed", net.Speed)
	}

	if net.Duplex != "" && net.Duplex != "unknown" && net.Duplex != "Unknown!" {
		addDetailRow(group, "Duplex", net.Duplex)
	}

	if net.MTU != "" && net.MTU != "0" {
		addDetailRow(group, "MTU", net.MTU)
	}

	if node.VendorID != "" && node.ProductID != "" {
		addDetailRow(group, "VID:PID", fmt.Sprintf("%s:%s", node.VendorID, node.ProductID))
	}

	if node.Driver != "" {
		addDetailRow(group, "Driver", node.Driver)
	}

	return group
}

func (g *GtkUI) buildSensorReadingsGroup(node *domain.DeviceNode) *adw.PreferencesGroup {
	return g.buildSensorReadingsGroupTitle(node, "Sensors")
}

func (g *GtkUI) buildSensorReadingsGroupTitle(node *domain.DeviceNode, title string) *adw.PreferencesGroup {
	group := adw.NewPreferencesGroup()
	group.SetTitle(title)

	type sensorRow struct {
		sensor *domain.SensorInfo
		row    *adw.ActionRow
	}
	var liveRows []sensorRow

	addSensorRow := func(s *domain.SensorInfo) {
		row := adw.NewActionRow()
		row.SetTitle(s.Label)
		row.SetSubtitleSelectable(true)

		value := s.FormattedValue()
		if s.Type == "temperature" {
			if t := s.ThresholdString(); t != "" {
				value = fmt.Sprintf("%s (%s)", value, t)
			}
		}
		row.SetSubtitle(value)
		group.Add(row)

		if s.HWMonPath != "" {
			liveRows = append(liveRows, sensorRow{sensor: s, row: row})
		}
	}

	for _, s := range node.SensorReadings {
		if s.Type == "temperature" {
			addSensorRow(s)
		}
	}
	for _, s := range node.SensorReadings {
		if s.Type == "fan" {
			addSensorRow(s)
		}
	}
	for _, s := range node.SensorReadings {
		if s.Type == "voltage" {
			addSensorRow(s)
		}
	}
	for _, s := range node.SensorReadings {
		if s.Type == "power" {
			addSensorRow(s)
		}
	}

	if node.Driver != "" {
		addDetailRow(group, "Device", node.Driver)
	}

	// Live update: refresh sensor values every 3 seconds while page is visible
	if len(liveRows) > 0 {
		var sourceID glib.SourceHandle
		firstRow := liveRows[0].row
		firstRow.ConnectMap(func() {
			sourceID = glib.TimeoutSecondsAdd(3, func() bool {
				for _, sr := range liveRows {
					raw := readSysfsForUI(sr.sensor.HWMonPath)
					if raw == "" {
						continue
					}
					rawVal, err := strconv.ParseFloat(raw, 64)
					if err != nil {
						continue
					}
					// hwmon values: temps in millidegrees, voltages in millivolts,
					// fans in RPM (no conversion), power in microwatts
					switch sr.sensor.Type {
					case "temperature":
						sr.sensor.Value = rawVal / 1000
					case "voltage":
						sr.sensor.Value = rawVal / 1000
					case "power":
						sr.sensor.Value = rawVal / 1000000
					default:
						sr.sensor.Value = rawVal
					}
					value := sr.sensor.FormattedValue()
					if sr.sensor.Type == "temperature" {
						if t := sr.sensor.ThresholdString(); t != "" {
							value = fmt.Sprintf("%s (%s)", value, t)
						}
					}
					sr.row.SetSubtitle(value)
				}
				return true
			})
		})
		firstRow.ConnectUnmap(func() {
			if sourceID > 0 {
				glib.SourceRemove(sourceID)
				sourceID = 0
			}
		})
	}

	return group
}

func addLiveRow(group *adw.PreferencesGroup, label, value string) *adw.ActionRow {
	row := adw.NewActionRow()
	row.SetTitle(label)
	row.SetSubtitleSelectable(true)
	if value != "" {
		row.SetSubtitle(value)
		row.SetVisible(true)
	} else {
		row.SetVisible(false)
	}
	group.Add(row)
	return row
}

func setLiveRow(row *adw.ActionRow, value string) {
	if row == nil {
		return
	}
	if value != "" {
		row.SetSubtitle(value)
		row.SetVisible(true)
	} else {
		row.SetVisible(false)
	}
}

func pollWirelessNM(ifaceName string) *domain.NetworkInfo {
	conn, err := dbus.SystemBus()
	if err != nil {
		return nil
	}

	nm := conn.Object("org.freedesktop.NetworkManager", "/org/freedesktop/NetworkManager")
	var devPath dbus.ObjectPath
	if err := nm.Call("org.freedesktop.NetworkManager.GetDeviceByIpIface", 0, ifaceName).Store(&devPath); err != nil {
		return nil
	}

	dev := conn.Object("org.freedesktop.NetworkManager", devPath)
	apVar, err := dev.GetProperty("org.freedesktop.NetworkManager.Device.Wireless.ActiveAccessPoint")
	if err != nil {
		return nil
	}
	apPath, ok := apVar.Value().(dbus.ObjectPath)
	if !ok || !apPath.IsValid() || apPath == "/" {
		return &domain.NetworkInfo{}
	}

	ap := conn.Object("org.freedesktop.NetworkManager", apPath)
	info := &domain.NetworkInfo{}

	if v, err := ap.GetProperty("org.freedesktop.NetworkManager.AccessPoint.Ssid"); err == nil {
		if ssidBytes, ok := v.Value().([]byte); ok {
			info.SSID = string(ssidBytes)
		}
	}
	if v, err := ap.GetProperty("org.freedesktop.NetworkManager.AccessPoint.Strength"); err == nil {
		if strength, ok := v.Value().(byte); ok {
			info.Signal = int(strength)
		}
	}
	if v, err := ap.GetProperty("org.freedesktop.NetworkManager.AccessPoint.Frequency"); err == nil {
		if freq, ok := v.Value().(uint32); ok {
			info.Frequency = freq
			info.Channel = domain.FrequencyToChannel(freq)
		}
	}
	if v, err := ap.GetProperty("org.freedesktop.NetworkManager.AccessPoint.MaxBitrate"); err == nil {
		if rate, ok := v.Value().(uint32); ok {
			info.BitRate = int(rate)
		}
	}

	var rsnFlags, wpaFlags uint32
	if v, err := ap.GetProperty("org.freedesktop.NetworkManager.AccessPoint.RsnFlags"); err == nil {
		if f, ok := v.Value().(uint32); ok {
			rsnFlags = f
		}
	}
	if v, err := ap.GetProperty("org.freedesktop.NetworkManager.AccessPoint.WpaFlags"); err == nil {
		if f, ok := v.Value().(uint32); ok {
			wpaFlags = f
		}
	}
	info.SecurityType = nmSecurityString(rsnFlags, wpaFlags)

	return info
}

func nmSecurityString(rsnFlags, wpaFlags uint32) string {
	const (
		keyMgmtPSK          = 0x100
		keyMgmt8021X        = 0x200
		keyMgmtSAE          = 0x400
		keyMgmtOWE          = 0x800
		keyMgmtEAPSuiteB192 = 0x2000
		pairWEP40           = 0x1
		pairWEP104          = 0x2
	)
	switch {
	case rsnFlags&keyMgmtSAE != 0:
		return "WPA3"
	case rsnFlags&keyMgmtEAPSuiteB192 != 0:
		return "WPA3 Enterprise"
	case rsnFlags&keyMgmt8021X != 0:
		return "WPA2 Enterprise"
	case rsnFlags&keyMgmtPSK != 0:
		return "WPA2"
	case wpaFlags&keyMgmt8021X != 0:
		return "WPA Enterprise"
	case wpaFlags&keyMgmtPSK != 0:
		return "WPA"
	case rsnFlags&(pairWEP40|pairWEP104) != 0,
		wpaFlags&(pairWEP40|pairWEP104) != 0:
		return "WEP"
	case rsnFlags&keyMgmtOWE != 0:
		return "OWE"
	default:
		if rsnFlags == 0 && wpaFlags == 0 {
			return "Open"
		}
		return ""
	}
}

func readSysfsForUI(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func (g *GtkUI) buildBluetoothHIDGroup(node *domain.DeviceNode) *adw.PreferencesGroup {
	group := adw.NewPreferencesGroup()
	group.SetTitle("Bluetooth Device")

	addDetailRow(group, "Vendor", node.Vendor)
	addDetailRow(group, "Connection", "Bluetooth")

	if node.VendorID != "" && node.ProductID != "" {
		addDetailRow(group, "VID:PID", fmt.Sprintf("%s:%s", node.VendorID, node.ProductID))
	}

	addDetailRow(group, "Driver", node.Driver)

	return group
}

func (g *GtkUI) buildGeneralInfoGroup(node *domain.DeviceNode) *adw.PreferencesGroup {
	group := adw.NewPreferencesGroup()
	group.SetTitle("General")

	if node.Vendor != "" {
		addDetailRow(group, "Vendor", node.Vendor)
	}

	if node.Product != "" {
		addDetailRow(group, "Product", node.Product)
	}

	if node.VendorID != "" && node.ProductID != "" {
		addDetailRow(group, "VID:PID", fmt.Sprintf("%s:%s", node.VendorID, node.ProductID))
	}

	driverText := "None"
	if node.Driver != "" {
		driverText = node.Driver
	}
	addDetailRow(group, "Driver", driverText)

	return group
}

func (g *GtkUI) buildIOMMUGroupSection(node *domain.DeviceNode) *adw.PreferencesGroup {
	group := adw.NewPreferencesGroup()
	group.SetTitle(fmt.Sprintf("IOMMU Group %d", node.IOMMUGroup))

	g.mu.RLock()
	members := g.iommuGroups[node.IOMMUGroup]
	g.mu.RUnlock()

	for _, member := range members {
		if member.ID == node.ID {
			continue
		}
		row := adw.NewActionRow()
		row.SetTitle(member.DisplayName())
		if member.Driver != "" {
			row.SetSubtitle(member.Driver)
		}
		icon := gtk.NewImageFromIconName(g.getIconName(member))
		row.AddPrefix(icon)
		group.Add(row)
	}

	if len(members) <= 1 {
		row := adw.NewActionRow()
		row.SetTitle("No other devices in this group")
		row.AddCSSClass("dim-label")
		group.Add(row)
	}

	return group
}

func (g *GtkUI) buildGPUInfoGroup(node *domain.DeviceNode) *adw.PreferencesGroup {
	gpuInfo := node.GPUInfo
	group := adw.NewPreferencesGroup()
	group.SetTitle("Graphics")

	if node.Vendor != "" {
		addDetailRow(group, "Vendor", node.Vendor)
	}

	if gpuInfo.SubsystemVendor != "" {
		mfr := lookupSubsystemVendor(gpuInfo.SubsystemVendor)
		if mfr != "" && mfr != node.Vendor {
			addDetailRow(group, "Manufacturer", mfr)
		}
	}

	if gpuInfo.DriverName != "" {
		driverStr := gpuInfo.DriverName
		if gpuInfo.DriverVersion != "" {
			driverStr = fmt.Sprintf("%s %s", gpuInfo.DriverName, gpuInfo.DriverVersion)
		}
		addDetailRow(group, "Graphics Driver", driverStr)
	}

	if gpuInfo.DriverDesc != "" {
		addDetailRow(group, "Description", gpuInfo.DriverDesc)
	}

	if vram := gpuInfo.VRAMTotalString(); vram != "" {
		vramStr := vram
		if gpuInfo.VRAMUsed > 0 {
			usedMB := gpuInfo.VRAMUsed / (1024 * 1024)
			if usedMB >= 1024 {
				vramStr = fmt.Sprintf("%s (%.1f GB used)", vram, gpuInfo.VRAMUsedGB())
			} else {
				vramStr = fmt.Sprintf("%s (%d MB used)", vram, usedMB)
			}
		}
		addDetailRow(group, "Video Memory", vramStr)
	}

	if gpuInfo.GPUClockMax > 0 {
		clockStr := fmt.Sprintf("%d MHz", gpuInfo.GPUClockMax)
		if gpuInfo.GPUClockMin > 0 {
			clockStr = fmt.Sprintf("%d – %d MHz", gpuInfo.GPUClockMin, gpuInfo.GPUClockMax)
		}
		addDetailRow(group, "GPU Clock", clockStr)
	}

	if gpuInfo.MemClockMax > 0 {
		memStr := fmt.Sprintf("%d MHz", gpuInfo.MemClockMax)
		if gpuInfo.MemClockMin > 0 {
			memStr = fmt.Sprintf("%d – %d MHz", gpuInfo.MemClockMin, gpuInfo.MemClockMax)
		}
		addDetailRow(group, "Memory Clock", memStr)
	}

	if gpuInfo.VRAMVendor != "" {
		addDetailRow(group, "VRAM Vendor", gpuInfo.VRAMVendor)
	}

	if gpuInfo.UniqueID != "" {
		addDetailRow(group, "GPU Serial", gpuInfo.UniqueID)
	}

	if gpuInfo.PCIeLinkSpeed != "" {
		linkStr := gpuInfo.PCIeLinkSpeed
		if gpuInfo.PCIeLinkWidth != "" {
			linkStr = fmt.Sprintf("%s × %s", linkStr, gpuInfo.PCIeLinkWidth)
		}
		if gpuInfo.MaxLinkSpeed != "" && gpuInfo.MaxLinkWidth != "" {
			maxStr := fmt.Sprintf("%s × %s", gpuInfo.MaxLinkSpeed, gpuInfo.MaxLinkWidth)
			if maxStr != linkStr {
				linkStr = fmt.Sprintf("%s (max: %s)", linkStr, maxStr)
			}
		}
		addDetailRow(group, "PCIe Link", linkStr)
	}

	if gpuInfo.VBIOSVersion != "" {
		vbiosRow := adw.NewActionRow()
		vbiosRow.SetTitle("VBIOS")
		vbiosRow.SetSubtitle(gpuInfo.VBIOSVersion)
		vbiosRow.SetSubtitleSelectable(true)

		if g.driverMgmtEnabled && node.Type == domain.DeviceTypePCI {
			pciAddr := strings.TrimPrefix(node.ID, "pci:")
			romPath := fmt.Sprintf("/sys/bus/pci/devices/%s/rom", pciAddr)
			if _, err := os.Stat(romPath); err == nil {
				saveBtn := gtk.NewButton()
				saveBtn.SetLabel("Save")
				saveBtn.AddCSSClass("flat")
				saveBtn.SetVAlign(gtk.AlignCenter)
				saveBtn.SetTooltipText("Save VBIOS ROM to a file")
				saveBtn.ConnectClicked(func() { g.saveVBIOS(pciAddr) })
				vbiosRow.AddSuffix(saveBtn)
			}
		}

		group.Add(vbiosRow)
	}

	return group
}

func (g *GtkUI) buildMonitorsGroup(gpuInfo *domain.GPUInfo) *adw.PreferencesGroup {
	group := adw.NewPreferencesGroup()
	group.SetTitle("Display Outputs")

	for _, monitor := range gpuInfo.Monitors {
		row := adw.NewActionRow()
		row.SetTitle(monitor.FormattedName())

		var subtitle string
		if monitor.Status == domain.ConnectionConnected {
			if monitor.CurrentMode != nil {
				subtitle = fmt.Sprintf("Connected · %dx%d", monitor.CurrentMode.Width, monitor.CurrentMode.Height)
				if monitor.CurrentMode.RefreshRate > 0 {
					subtitle = fmt.Sprintf("%s @ %d Hz", subtitle, monitor.CurrentMode.RefreshRate)
				}
			} else {
				subtitle = "Connected"
			}
			if monitor.WidthMM > 0 && monitor.HeightMM > 0 {
				subtitle = fmt.Sprintf("%s · %d × %d mm", subtitle, monitor.WidthMM, monitor.HeightMM)
			}
		} else {
			subtitle = "Disconnected"
		}
		row.SetSubtitle(subtitle)

		icon := gtk.NewImageFromIconName("video-display-symbolic")
		row.AddPrefix(icon)

		if monitor.Status == domain.ConnectionConnected {
			monName := monitor.FormattedName()
			cardIdx := gpuInfo.CardIndex
			row.SetActivatable(true)
			row.ConnectActivated(func() {
				g.navigateToDisplayByConnector(cardIdx, monName)
			})
			chevron := gtk.NewImageFromIconName("go-next-symbolic")
			chevron.AddCSSClass("dim-label")
			row.AddSuffix(chevron)
		}

		group.Add(row)
	}

	return group
}

func (g *GtkUI) buildStorageInfoGroup(info *domain.StorageInfo) *adw.PreferencesGroup {
	group := adw.NewPreferencesGroup()
	group.SetTitle("Storage")

	if info.Model != "" {
		addDetailRow(group, "Model", info.Model)
	}
	if info.Serial != "" {
		addDetailRow(group, "Serial", info.Serial)
	}
	if info.Firmware != "" {
		addDetailRow(group, "Firmware", info.Firmware)
	}
	if info.SizeBytes > 0 {
		addDetailRow(group, "Capacity", info.SizeString())
	}
	if info.Transport != "" {
		addDetailRow(group, "Interface", strings.ToUpper(info.Transport))
	}
	if info.State != "" {
		addDetailRow(group, "State", info.State)
	}
	if info.BlockDevice != "" {
		addDetailRow(group, "Block Device", "/dev/"+info.BlockDevice)
	}

	return group
}

func (g *GtkUI) buildSMARTGroup(smart *domain.SMARTData) *adw.PreferencesGroup {
	group := adw.NewPreferencesGroup()
	group.SetTitle("Drive Health (SMART)")

	if smart.Status != "" {
		row := adw.NewActionRow()
		row.SetTitle("Health Status")
		row.SetSubtitle(smart.Status)
		if smart.Healthy {
			icon := gtk.NewImageFromIconName("emblem-ok-symbolic")
			icon.AddCSSClass("success")
			row.AddSuffix(icon)
		} else {
			icon := gtk.NewImageFromIconName("dialog-warning-symbolic")
			icon.AddCSSClass("warning")
			row.AddSuffix(icon)
		}
		group.Add(row)
	}

	if smart.PercentageUsed > 0 {
		addDetailRow(group, "Life Used", fmt.Sprintf("%d%%", smart.PercentageUsed))
	}
	if smart.AvailableSpare > 0 {
		addDetailRow(group, "Available Spare", fmt.Sprintf("%d%%", smart.AvailableSpare))
	}
	if smart.PowerOnHours > 0 {
		addDetailRow(group, "Power-On Hours", fmt.Sprintf("%d", smart.PowerOnHours))
	}
	if smart.PowerCycles > 0 {
		addDetailRow(group, "Power Cycles", fmt.Sprintf("%d", smart.PowerCycles))
	}
	if smart.UnsafeShutdowns > 0 {
		addDetailRow(group, "Unsafe Shutdowns", fmt.Sprintf("%d", smart.UnsafeShutdowns))
	}
	if smart.MediaErrors > 0 {
		addDetailRow(group, "Media Errors", fmt.Sprintf("%d", smart.MediaErrors))
	}
	if smart.DataReadBytes > 0 {
		addDetailRow(group, "Total Read", smart.DataReadString())
	}
	if smart.DataWrittenBytes > 0 {
		addDetailRow(group, "Total Written", smart.DataWrittenString())
	}

	return group
}

func batteryIconName(capacity int, status string) string {
	switch {
	case status == "Charging":
		return "battery-level-80-charging-symbolic"
	case capacity >= 80:
		return "battery-level-80-symbolic"
	case capacity >= 50:
		return "battery-level-50-symbolic"
	case capacity >= 20:
		return "battery-level-20-symbolic"
	default:
		return "battery-level-0-symbolic"
	}
}

func (g *GtkUI) buildBatteryGroup(node *domain.DeviceNode) *adw.PreferencesGroup {
	battery := node.BatteryInfo
	group := adw.NewPreferencesGroup()
	group.SetTitle("Battery")

	var chargeRow *adw.ActionRow
	var chargeIcon *gtk.Image
	var statusRow *adw.ActionRow

	if battery.Capacity >= 0 {
		chargeRow = adw.NewActionRow()
		chargeRow.SetTitle("Charge")
		chargeRow.SetSubtitle(fmt.Sprintf("%d%%", battery.Capacity))

		chargeIcon = gtk.NewImageFromIconName(batteryIconName(battery.Capacity, battery.Status))
		chargeRow.AddPrefix(chargeIcon)
		group.Add(chargeRow)
	} else if battery.CapacityLevel != "" {
		addDetailRow(group, "Charge Level", battery.CapacityLevel)
	}

	if battery.Status != "" && battery.Status != "Connected" {
		statusRow = adw.NewActionRow()
		statusRow.SetTitle("Status")
		statusRow.SetSubtitle(battery.Status)
		group.Add(statusRow)
	}

	// Live UPower polling for UPS devices
	if node.Class == domain.DeviceClassPower && node.Type == domain.DeviceTypeUSB {
		usbAddr := strings.TrimPrefix(node.ID, "usb:")
		upowerPath := findUPowerDevice(usbAddr)
		if upowerPath != "" {
			var sourceID glib.SourceHandle
			group.ConnectMap(func() {
				sourceID = glib.TimeoutSecondsAdd(5, func() bool {
					pct, status := pollUPowerBattery(upowerPath)
					if chargeRow != nil && pct >= 0 {
						chargeRow.SetSubtitle(fmt.Sprintf("%d%%", pct))
						chargeIcon.SetFromIconName(batteryIconName(pct, status))
					}
					if statusRow != nil && status != "" {
						statusRow.SetSubtitle(status)
					}
					return true
				})
			})
			group.ConnectUnmap(func() {
				if sourceID > 0 {
					glib.SourceRemove(sourceID)
					sourceID = 0
				}
			})
		}
	}

	return group
}

// findUPowerDevice matches a USB address (e.g. "5-2.4") in UPower native paths.
func findUPowerDevice(usbAddr string) dbus.ObjectPath {
	conn, err := dbus.SystemBus()
	if err != nil {
		return ""
	}

	obj := conn.Object("org.freedesktop.UPower", "/org/freedesktop/UPower")
	var devicePaths []dbus.ObjectPath
	if err := obj.Call("org.freedesktop.UPower.EnumerateDevices", 0).Store(&devicePaths); err != nil {
		return ""
	}

	for _, path := range devicePaths {
		devObj := conn.Object("org.freedesktop.UPower", path)
		native, err := devObj.GetProperty("org.freedesktop.UPower.Device.NativePath")
		if err != nil {
			continue
		}
		if n, ok := native.Value().(string); ok && strings.Contains(n, "/"+usbAddr+"/") {
			return path
		}
	}
	return ""
}

var upowerStateNames = map[uint32]string{
	1: "Charging",
	2: "Discharging",
	3: "Empty",
	4: "Fully charged",
	5: "Pending charge",
	6: "Pending discharge",
}

func pollUPowerBattery(path dbus.ObjectPath) (int, string) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return -1, ""
	}

	devObj := conn.Object("org.freedesktop.UPower", path)

	pct := -1
	if v, err := devObj.GetProperty("org.freedesktop.UPower.Device.Percentage"); err == nil {
		if p, ok := v.Value().(float64); ok {
			pct = int(p)
		}
	}

	status := ""
	if v, err := devObj.GetProperty("org.freedesktop.UPower.Device.State"); err == nil {
		if st, ok := v.Value().(uint32); ok {
			status = upowerStateNames[st]
		}
	}

	return pct, status
}

func (g *GtkUI) buildTechnicalDetailsGroup(node *domain.DeviceNode) *adw.PreferencesGroup {
	group := adw.NewPreferencesGroup()

	expander := adw.NewExpanderRow()
	expander.SetTitle("Technical Details")
	expander.SetSubtitle("Internal identifiers and capabilities")

	addExpanderRow(expander, "Internal ID", node.ID)
	addExpanderRow(expander, "Sysfs Path", node.Path)

	typeName := typeDisplayNames[node.Type]
	if typeName == "" {
		typeName = string(node.Type)
	}
	addExpanderRow(expander, "Bus Type", typeName)

	className := classDisplayNames[node.Class]
	if className == "" {
		className = string(node.Class)
	}
	addExpanderRow(expander, "Device Class", className)

	if node.ClassCode != "" {
		addExpanderRow(expander, "Class Code", "0x"+node.ClassCode)
	}

	if len(node.Capabilities) > 0 {
		var humanCaps []string
		for _, cap := range node.Capabilities {
			if label, ok := capabilityLabels[cap]; ok {
				humanCaps = append(humanCaps, label)
			} else {
				humanCaps = append(humanCaps, cap)
			}
		}
		addExpanderRow(expander, "Capabilities", strings.Join(humanCaps, ", "))
	}

	if node.GPUInfo != nil && node.GPUInfo.CardPath != "" {
		addExpanderRow(expander, "DRM Device", node.GPUInfo.CardPath)
	}

	if node.GPUInfo != nil && node.GPUInfo.SubsystemVendor != "" {
		addExpanderRow(expander, "Subsystem",
			fmt.Sprintf("%s:%s", node.GPUInfo.SubsystemVendor, node.GPUInfo.SubsystemDevice))
	}

	group.Add(expander)
	return group
}

func (g *GtkUI) buildResourcesGroup(res *domain.DeviceResources) *adw.PreferencesGroup {
	hasContent := res.IRQ >= 0 || len(res.Memory) > 0 || len(res.IOPorts) > 0
	if !hasContent {
		return nil
	}

	group := adw.NewPreferencesGroup()

	expander := adw.NewExpanderRow()
	expander.SetTitle("Hardware Resources")
	expander.SetSubtitle("IRQ, memory, and I/O assignments")

	if res.IRQ >= 0 {
		irqStr := fmt.Sprintf("%d", res.IRQ)
		if len(res.IRQs) > 1 {
			irqStr = fmt.Sprintf("%d (MSI: %d IRQs)", res.IRQ, len(res.IRQs))
		}
		addExpanderRow(expander, "IRQ", irqStr)
	}

	if res.NUMANode >= 0 {
		addExpanderRow(expander, "NUMA Node", fmt.Sprintf("%d", res.NUMANode))
	}

	if res.LocalCPUs != "" {
		addExpanderRow(expander, "CPU Affinity", res.LocalCPUs)
	}

	for i, mem := range res.Memory {
		label := fmt.Sprintf("Memory BAR%d", mem.Index)
		value := fmt.Sprintf("%s [%s]", mem.FormattedRange(), mem.FormattedSize())
		if mem.Prefetchable {
			value += " (prefetchable)"
		}
		addExpanderRow(expander, label, value)
		if i >= 5 {
			addExpanderRow(expander, "…", fmt.Sprintf("%d more regions", len(res.Memory)-i-1))
			break
		}
	}

	for i, io := range res.IOPorts {
		label := fmt.Sprintf("I/O BAR%d", io.Index)
		value := io.FormattedRange()
		if io.Size > 0 {
			value = fmt.Sprintf("%s [%s]", value, io.FormattedSize())
		}
		addExpanderRow(expander, label, value)
		if i >= 3 {
			addExpanderRow(expander, "…", fmt.Sprintf("%d more regions", len(res.IOPorts)-i-1))
			break
		}
	}

	group.Add(expander)
	return group
}

func addDetailRow(group *adw.PreferencesGroup, label, value string) {
	if value == "" {
		return
	}
	row := adw.NewActionRow()
	row.SetTitle(label)
	row.SetSubtitle(value)
	row.SetSubtitleSelectable(true)
	group.Add(row)
}

func addExpanderRow(expander *adw.ExpanderRow, label, value string) {
	row := adw.NewActionRow()
	row.SetTitle(label)
	row.SetSubtitle(value)
	row.SetSubtitleSelectable(true)
	expander.AddRow(row)
}
