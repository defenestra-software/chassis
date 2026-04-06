package gtk

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"defenestra.io/chassis/internal/core/domain"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	glib2 "github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func (g *GtkUI) buildActionButtons(node *domain.DeviceNode) gtk.Widgetter {
	buttonBox := gtk.NewBox(gtk.OrientationHorizontal, 12)
	buttonBox.SetHAlign(gtk.AlignCenter)
	buttonBox.SetMarginTop(12)

	if node.Driver != "" {
		disableBtn := gtk.NewButtonWithLabel("Disable")
		disableBtn.AddCSSClass("destructive-action")
		disableBtn.SetTooltipText("Detach the driver and disable this device")
		deviceID := node.ID
		displayName := node.DisplayName()
		driver := node.Driver
		disableBtn.ConnectClicked(func() {
			g.confirmAction("Disable Device",
				fmt.Sprintf("This will disable %q by detaching its driver (%s).\n\nThe device will stop functioning until re-enabled.", displayName, driver),
				func() { g.doAction(deviceID, "disable") })
		})
		buttonBox.Append(disableBtn)
	} else {
		enableBtn := gtk.NewButtonWithLabel("Enable")
		enableBtn.AddCSSClass("suggested-action")
		enableBtn.SetTooltipText("Restore the default driver")
		deviceID := node.ID
		displayName := node.DisplayName()
		enableBtn.ConnectClicked(func() {
			g.confirmAction("Enable Device",
				fmt.Sprintf("This will enable %q by loading the appropriate driver.", displayName),
				func() { g.doAction(deviceID, "enable") })
		})
		buttonBox.Append(enableBtn)
	}

	changeBtn := gtk.NewButtonWithLabel("Change Driver")
	changeBtn.SetTooltipText("Bind a specific driver to this device")
	nodeCopy := node
	changeBtn.ConnectClicked(func() { g.showChangeDriverDialog(nodeCopy) })
	buttonBox.Append(changeBtn)

	return buttonBox
}

func (g *GtkUI) confirmAction(title, message string, action func()) {
	dialog := adw.NewMessageDialog(&g.window.Window, title, message)
	dialog.AddResponse("cancel", "Cancel")
	dialog.AddResponse("confirm", "Confirm")
	dialog.SetResponseAppearance("confirm", adw.ResponseDestructive)
	dialog.SetDefaultResponse("cancel")
	dialog.SetCloseResponse("cancel")

	dialog.ConnectResponse(func(response string) {
		if response == "confirm" {
			action()
		}
	})
	dialog.Show()
}

func (g *GtkUI) doAction(deviceID, action string) {
	if g.onAction == nil {
		return
	}

	g.ShowProgress(fmt.Sprintf("Performing %s…", action))
	go func() {
		if err := g.onAction(deviceID, action); err != nil {
			g.ShowError(err)
		} else if g.onRefresh != nil {
			g.onRefresh()
		}
		g.HideProgress()
	}()
}

func (g *GtkUI) showChangeDriverDialog(node *domain.DeviceNode) {
	dialog := adw.NewMessageDialog(&g.window.Window, "Change Driver",
		fmt.Sprintf("Enter the driver name to bind to %q.\n\nThe current driver will be unbound first.", node.DisplayName()))

	dialog.AddResponse("cancel", "Cancel")
	dialog.AddResponse("apply", "Apply")
	dialog.SetResponseAppearance("apply", adw.ResponseSuggested)
	dialog.SetDefaultResponse("cancel")
	dialog.SetCloseResponse("cancel")

	entry := gtk.NewEntry()
	entry.SetPlaceholderText("e.g. r8169, nvidia, nouveau")
	if node.Driver != "" {
		entry.SetText(node.Driver)
	}
	entry.SetMarginStart(24)
	entry.SetMarginEnd(24)
	entry.SetMarginTop(12)
	dialog.SetExtraChild(entry)

	deviceID := node.ID
	currentDriver := node.Driver
	dialog.ConnectResponse(func(response string) {
		if response != "apply" {
			return
		}
		driverName := strings.TrimSpace(entry.Text())
		if driverName == "" {
			return
		}

		g.ShowProgress(fmt.Sprintf("Changing driver to %s…", driverName))
		go func() {
			if currentDriver != "" {
				if err := g.onAction(deviceID, "disable"); err != nil {
					g.ShowError(err)
					g.HideProgress()
					return
				}
			}
			if err := g.onAction(deviceID, "bind:"+driverName); err != nil {
				g.ShowError(err)
			}
			g.HideProgress()
			if g.onRefresh != nil {
				g.onRefresh()
			}
		}()
	})
	dialog.Show()
}

func (g *GtkUI) showModesDialog(monitor *domain.Monitor) {
	dialog := adw.NewDialog()
	dialog.SetTitle(fmt.Sprintf("%s – Display Modes", monitor.FormattedName()))
	dialog.SetContentWidth(400)
	dialog.SetContentHeight(500)

	headerBar := adw.NewHeaderBar()
	headerBar.SetShowStartTitleButtons(false)
	headerBar.SetShowEndTitleButtons(false)

	closeBtn := gtk.NewButton()
	closeBtn.SetIconName("window-close-symbolic")
	closeBtn.AddCSSClass("flat")
	closeBtn.ConnectClicked(func() { dialog.Close() })
	headerBar.PackEnd(closeBtn)

	listBox := gtk.NewListBox()
	listBox.AddCSSClass("boxed-list")
	listBox.SetSelectionMode(gtk.SelectionNone)

	// Group modes by resolution, sorted largest first
	type resGroup struct {
		key    string
		width  uint16
		height uint16
		modes  []domain.DisplayMode
	}
	byRes := make(map[string]*resGroup)
	for _, m := range monitor.Modes {
		key := fmt.Sprintf("%dx%d", m.Width, m.Height)
		if _, ok := byRes[key]; !ok {
			byRes[key] = &resGroup{key: key, width: m.Width, height: m.Height}
		}
		byRes[key].modes = append(byRes[key].modes, m)
	}
	var resolutions []*resGroup
	for _, rg := range byRes {
		resolutions = append(resolutions, rg)
	}
	sort.Slice(resolutions, func(i, j int) bool {
		return int(resolutions[i].width)*int(resolutions[i].height) >
			int(resolutions[j].width)*int(resolutions[j].height)
	})

	for _, res := range resolutions {
		row := adw.NewActionRow()
		row.SetTitle(res.key)
		var rates []string
		for _, m := range res.modes {
			r := fmt.Sprintf("%d Hz", m.RefreshRate)
			if m.IsCurrent {
				r += " (current)"
			}
			rates = append(rates, r)
		}
		row.SetSubtitle(strings.Join(rates, ", "))
		for _, m := range res.modes {
			if m.IsCurrent {
				icon := gtk.NewImageFromIconName("emblem-ok-symbolic")
				icon.AddCSSClass("success")
				row.AddPrefix(icon)
				break
			}
		}
		listBox.Append(row)
	}

	scroll := gtk.NewScrolledWindow()
	scroll.SetChild(listBox)
	scroll.SetVExpand(true)
	scroll.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)

	infoLabel := gtk.NewLabel(fmt.Sprintf("%d display modes available", len(monitor.Modes)))
	infoLabel.AddCSSClass("dim-label")
	infoLabel.SetMarginTop(12)
	infoLabel.SetMarginBottom(6)

	contentBox := gtk.NewBox(gtk.OrientationVertical, 0)
	contentBox.Append(infoLabel)
	if monitor.WidthMM > 0 && monitor.HeightMM > 0 {
		physLabel := gtk.NewLabel(fmt.Sprintf("Physical size: %d × %d mm", monitor.WidthMM, monitor.HeightMM))
		physLabel.AddCSSClass("dim-label")
		physLabel.SetMarginBottom(12)
		contentBox.Append(physLabel)
	}
	contentBox.Append(scroll)

	toolbarView := adw.NewToolbarView()
	toolbarView.AddTopBar(headerBar)
	toolbarView.SetContent(contentBox)

	dialog.SetChild(toolbarView)
	dialog.Present(&g.window.Window)
}

func (g *GtkUI) saveVBIOS(pciAddress string) {
	if g.onAction == nil {
		return
	}

	// Use GTK FileDialog to let the user choose where to save
	dialog := gtk.NewFileDialog()
	dialog.SetTitle("Save VBIOS ROM")
	filename := fmt.Sprintf("vbios_%s.rom", strings.ReplaceAll(pciAddress, ":", "-"))
	dialog.SetInitialName(filename)

	dialog.Save(context.Background(), &g.window.Window, func(result gio.AsyncResulter) {
		file, err := dialog.SaveFinish(result)
		if err != nil || file == nil {
			return // user cancelled or error
		}

		destPath := file.Path()
		if destPath == "" {
			return
		}

		toast := adw.NewToast("Reading VBIOS ROM...")
		toast.SetTimeout(2)
		g.toast.AddToast(toast)

		go func() {
			err := g.onAction(fmt.Sprintf("pci:%s", pciAddress), "dump-vbios:"+destPath)
			if err != nil {
				g.ShowError(err)
			} else {
				glib.IdleAdd(func() {
					done := adw.NewToast(fmt.Sprintf("VBIOS saved to %s", destPath))
					done.SetTimeout(3)
					g.toast.AddToast(done)
				})
			}
		}()
	})
}

func (g *GtkUI) buildAppMenu() gio.MenuModeller {
	menu := gio.NewMenu()

	// Setup section
	setupSection := gio.NewMenu()
	setupSection.Append("Setup Service…", "win.setup")
	menu.AppendSection("", setupSection)

	// Help section
	helpSection := gio.NewMenu()
	helpSection.Append("Keyboard Shortcuts", "win.shortcuts")
	helpSection.Append("Wiki", "win.wiki")
	helpSection.Append("Forum", "win.forum")
	helpSection.Append("About", "win.about")
	menu.AppendSection("", helpSection)

	// Register actions
	g.registerAction("shortcuts", func() {
		showShortcutsDialog(&g.window.Window)
	})
	g.registerAction("setup", func() {
		go func() {
			exe, err := os.Executable()
			if err != nil {
				return
			}
			cmd := exec.Command(exe, "--setup")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err == nil {
				glib.IdleAdd(func() {
					toast := adw.NewToast("Service installed. Restart Chassis for full functionality.")
					toast.SetTimeout(0)
					g.toast.AddToast(toast)
				})
			}
		}()
	})
	g.registerAction("about", func() {
		showAboutDialog(&g.window.Window)
	})
	g.registerAction("wiki", func() {
		gtk.ShowURI(nil, "https://wiki.defenestra.io/", 0)
	})
	g.registerAction("forum", func() {
		gtk.ShowURI(nil, "https://forum.defenestra.io/", 0)
	})
	g.registerAction("quit", func() {
		g.window.Close()
	})
	g.registerAction("search", func() {
		if g.searchBtn != nil {
			g.searchBtn.SetActive(!g.searchBtn.Active())
		}
	})

	// Register keyboard accelerators
	g.app.SetAccelsForAction("win.shortcuts", []string{"<Control>question"})
	g.app.SetAccelsForAction("win.quit", []string{"<Control>q"})
	g.app.SetAccelsForAction("win.search", []string{"<Control>f"})

	return menu
}

func (g *GtkUI) registerAction(name string, cb func()) {
	action := gio.NewSimpleAction(name, nil)
	action.ConnectActivate(func(_ *glib2.Variant) {
		cb()
	})
	g.window.AddAction(action)
}

func (g *GtkUI) navigateToDisplayByConnector(cardIndex int, connectorName string) {
	g.mu.RLock()

	cardPrefix := fmt.Sprintf("display:card%d-", cardIndex)

	// Extract the connector index number from the DRM name (e.g., "HDMI-1" → "1", "DP-3" → "3")
	parts := strings.Split(connectorName, "-")
	connIdx := ""
	if len(parts) > 0 {
		connIdx = parts[len(parts)-1]
	}

	// Extract connector type prefix (e.g., "HDMI" from "HDMI-1", "DP" from "DP-3")
	connTypePrefix := ""
	if len(parts) > 0 {
		connTypePrefix = strings.ToUpper(parts[0])
	}

	for id, node := range g.deviceMap {
		if !strings.HasPrefix(id, cardPrefix) {
			continue
		}
		if node.DisplayInfo == nil {
			continue
		}
		// Match by connector index and type prefix
		// sysfs: "HDMI-A-1" → ConnectorType="HDMI-A", ConnectorIndex="1"
		// DRM:   "HDMI-1"   → connTypePrefix="HDMI", connIdx="1"
		if node.DisplayInfo.ConnectorIndex == connIdx &&
			strings.HasPrefix(strings.ToUpper(node.DisplayInfo.ConnectorType), connTypePrefix) {
			g.mu.RUnlock()
			g.showDeviceDetail(node)
			return
		}
	}
	g.mu.RUnlock()
}

func (g *GtkUI) navigateToDevice(deviceID string) {
	g.mu.RLock()
	device, ok := g.deviceMap[deviceID]
	g.mu.RUnlock()

	if ok {
		g.showDeviceDetail(device)
	}
}

func (g *GtkUI) navigateToGPUByCard(cardIndex string) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	for _, node := range g.deviceMap {
		if node.GPUInfo != nil && fmt.Sprintf("%d", node.GPUInfo.CardIndex) == cardIndex {
			g.mu.RUnlock()
			g.showDeviceDetail(node)
			g.mu.RLock()
			return
		}
	}
}

func (g *GtkUI) getIconName(node *domain.DeviceNode) string {
	switch node.Class {
	case domain.DeviceClassGPU:
		return "applications-graphics-symbolic"
	case domain.DeviceClassNetwork:
		return "network-wired-symbolic"
	case domain.DeviceClassStorage:
		return "drive-harddisk-symbolic"
	case domain.DeviceClassAudio:
		return "audio-card-symbolic"
	case domain.DeviceClassUSBController:
		return "drive-removable-media-symbolic"
	case domain.DeviceClassHub:
		return "network-server-symbolic"
	case domain.DeviceClassInput:
		return g.getInputIcon(node)
	case domain.DeviceClassWireless:
		return "network-wireless-symbolic"
	case domain.DeviceClassCamera:
		return "camera-photo-symbolic"
	case domain.DeviceClassBridge:
		return "emblem-system-symbolic"
	case domain.DeviceClassSecurity:
		return "channel-secure-symbolic"
	case domain.DeviceClassDisplay:
		return "video-single-display-symbolic"
	case domain.DeviceClassProcessor:
		return "computer-symbolic"
	case domain.DeviceClassSensor:
		return "applications-engineering-symbolic"
	case domain.DeviceClassMemory:
		return "drive-harddisk-solidstate-symbolic"
	case domain.DeviceClassPortable:
		return "phone-symbolic"
	case domain.DeviceClassPower:
		return "battery-symbolic"
	case domain.DeviceClassLighting:
		return "preferences-color-symbolic"
	}
	switch node.Type {
	case domain.DeviceTypePCI:
		return "computer-symbolic"
	case domain.DeviceTypeUSB:
		return "drive-removable-media-symbolic"
	}
	return "dialog-question-symbolic"
}

func (g *GtkUI) getInputIcon(node *domain.DeviceNode) string {
	nameLower := strings.ToLower(node.Name)
	driverLower := strings.ToLower(node.Driver)

	// Gamepad / controller
	if strings.Contains(nameLower, "controller") ||
		strings.Contains(nameLower, "gamepad") ||
		strings.Contains(nameLower, "joystick") ||
		strings.Contains(nameLower, "stadia") ||
		strings.Contains(nameLower, "dualshock") ||
		strings.Contains(nameLower, "dualsense") ||
		strings.Contains(nameLower, "xbox") ||
		driverLower == "stadia" ||
		driverLower == "xpad" ||
		driverLower == "sony" ||
		driverLower == "nintendo" {
		return "input-gaming-symbolic"
	}

	// Mouse / trackball / touchpad
	if strings.Contains(nameLower, "mouse") ||
		strings.Contains(nameLower, "trackball") ||
		strings.Contains(nameLower, "touchpad") ||
		strings.Contains(nameLower, "triathlon") ||
		strings.Contains(nameLower, "m720") ||
		strings.Contains(nameLower, "m590") ||
		strings.Contains(nameLower, "mx master") ||
		strings.Contains(nameLower, "mx anywhere") {
		return "input-mouse-symbolic"
	}

	// Default: keyboard
	return "input-keyboard-symbolic"
}
