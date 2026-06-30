package gtk

import (
	"fmt"
	"sync"

	"defenestra.io/chassis/internal/core/domain"
	"defenestra.io/chassis/internal/core/port"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

type interfaceEntry struct {
	Device    *domain.DeviceNode
	Interface *domain.DeviceInterface
}

type GtkUI struct {
	app    *adw.Application
	window *adw.ApplicationWindow
	toast  *adw.ToastOverlay

	// Navigation
	splitView *adw.NavigationSplitView
	navView   *adw.NavigationView
	listPage  *adw.NavigationPage

	// Sidebar
	categoryList *gtk.ListBox
	iommuList    *gtk.ListBox
	iommuSection *gtk.Box
	hasIOMMU     bool

	// Device list
	deviceListBox *gtk.ListBox
	contentStack  *gtk.Stack
	iommuViewBox  *gtk.Box
	listTitle     string

	// Search
	searchBar   *gtk.SearchBar
	searchEntry *gtk.SearchEntry
	searchBtn   *gtk.ToggleButton

	// State
	deviceRoot           *domain.DeviceNode
	deviceMap            map[string]*domain.DeviceNode
	interfacesByCategory map[domain.DeviceClass][]interfaceEntry
	iommuGroups          map[int][]*domain.DeviceNode
	filterText           string
	selectedView         string // "class:gpu" or "iommu:all"
	uiReady              bool
	restoringSelection   bool
	pendingTree          *domain.DeviceNode
	onDetailPage         bool
	progressToast        *adw.Toast
	mu                   sync.RWMutex

	// Callbacks
	onAction          port.ActionHandler
	onRefresh         func() error
	driverMgmtEnabled bool
}

func NewGtkUI() *GtkUI {
	return &GtkUI{
		deviceMap:            make(map[string]*domain.DeviceNode),
		interfacesByCategory: make(map[domain.DeviceClass][]interfaceEntry),
		iommuGroups:          make(map[int][]*domain.DeviceNode),
	}
}

var _ port.UIAdapter = (*GtkUI)(nil)

func (g *GtkUI) SetActionHandler(handler port.ActionHandler) { g.onAction = handler }
func (g *GtkUI) SetRefreshHandler(handler func() error)      { g.onRefresh = handler }
func (g *GtkUI) SetDriverManagementEnabled(enabled bool)     { g.driverMgmtEnabled = enabled }

func (g *GtkUI) Run() error {
	g.app = adw.NewApplication("io.defenestra.Chassis", gio.ApplicationFlagsNone)

	g.app.ConnectActivate(func() {
		// Show app icon in the header bar decoration
		if s := gtk.SettingsGetDefault(); s != nil {
			s.SetObjectProperty("gtk-decoration-layout", "icon:minimize,maximize,close")
		}

		g.setupUI()
		g.window.Show()

		g.mu.Lock()
		g.uiReady = true
		pending := g.pendingTree
		g.pendingTree = nil
		g.mu.Unlock()

		if pending != nil {
			g.ShowDeviceTree(pending)
		}
	})

	if code := g.app.Run(nil); code != 0 {
		return fmt.Errorf("GTK application exited with code %d", code)
	}
	return nil
}

func (g *GtkUI) setupUI() {
	g.window = adw.NewApplicationWindow(&g.app.Application)
	g.window.SetTitle("Chassis")
	g.window.SetDefaultSize(1100, 700)
	g.window.SetSizeRequest(360, 500) // Minimum size for phone-like layouts

	// Toast overlay wraps everything
	g.toast = adw.NewToastOverlay()

	// Navigation split view (sidebar + content)
	g.splitView = adw.NewNavigationSplitView()
	g.splitView.SetMinSidebarWidth(200)
	g.splitView.SetMaxSidebarWidth(280)

	// Build sidebar
	sidebarPage := g.buildSidebarPage()
	g.splitView.SetSidebar(sidebarPage)

	// Build content (NavigationView for list → detail navigation)
	g.navView = adw.NewNavigationView()
	g.listPage = g.buildDeviceListPage()
	g.navView.Push(g.listPage)

	contentPage := adw.NewNavigationPage(g.navView, "")
	g.splitView.SetContent(contentPage)

	g.toast.SetChild(g.splitView)
	g.window.SetContent(g.toast)

	// Adaptive layout: collapse to single-pane when window is narrow.
	// At 600sp or less, the split view collapses and shows sidebar/content
	// one at a time with back-button navigation between them.
	breakpoint := adw.NewBreakpoint(
		adw.NewBreakpointConditionLength(adw.BreakpointConditionMaxWidth, 600, adw.LengthUnitSp))
	breakpoint.AddSetterDirect(
		glib.BaseObject(g.splitView), "collapsed", glib.NewValue(true))
	g.window.AddBreakpoint(breakpoint)
}

func (g *GtkUI) ShowDeviceTree(root *domain.DeviceNode) {
	g.mu.Lock()
	if !g.uiReady {
		g.pendingTree = root
		g.mu.Unlock()
		return
	}

	g.deviceRoot = root
	g.deviceMap = make(map[string]*domain.DeviceNode)
	g.interfacesByCategory = make(map[domain.DeviceClass][]interfaceEntry)
	g.iommuGroups = make(map[int][]*domain.DeviceNode)
	g.indexDevices(root)
	g.mu.Unlock()

	// Schedule GTK updates on the main thread
	glib.IdleAdd(func() {
		g.rebuildSidebar()
		if g.selectedView != "" {
			g.rebuildDeviceList()
		}
	})
}

func (g *GtkUI) indexDevices(node *domain.DeviceNode) {
	if node == nil {
		return
	}
	g.deviceMap[node.ID] = node

	// Index by interfaces - each interface puts the device into a sidebar category.
	// A GPU with Graphics + Sensor + Display interfaces appears in 3 categories.
	isLeaf := len(node.Children) == 0
	isReceiver := node.Type == domain.DeviceTypeUSB && node.Class == domain.DeviceClassUSBController && len(node.Children) > 0

	if (isLeaf || isReceiver) && len(node.Interfaces) > 0 {
		for _, iface := range node.Interfaces {
			// Skip GPU display interfaces from the sidebar - the standalone
			// display nodes (from EDID) already cover the Displays category.
			// GPU monitors are accessible via the GPU's Display Outputs section.
			if iface.Type == domain.InterfaceDisplay && node.GPUInfo != nil {
				continue
			}

			cat := iface.Category
			// Merge related categories
			switch cat {
			case domain.DeviceClassHub:
				cat = domain.DeviceClassUSBController
			case domain.DeviceClassWireless:
				cat = domain.DeviceClassNetwork
			}
			g.interfacesByCategory[cat] = append(
				g.interfacesByCategory[cat],
				interfaceEntry{Device: node, Interface: iface},
			)
		}

		// IOMMU groups still index by device (PCI only)
		if node.IOMMUGroup >= 0 && node.Type == domain.DeviceTypePCI {
			g.iommuGroups[node.IOMMUGroup] = append(g.iommuGroups[node.IOMMUGroup], node)
		}
	}

	for _, child := range node.Children {
		g.indexDevices(child)
	}
}

func (g *GtkUI) ShowError(err error) {
	glib.IdleAdd(func() {
		dialog := adw.NewMessageDialog(&g.window.Window, "Error", err.Error())
		dialog.AddResponse("ok", "OK")
		dialog.SetDefaultResponse("ok")
		dialog.Show()
	})
}

func (g *GtkUI) ShowProgress(message string) {
	glib.IdleAdd(func() {
		// Timeout 0 = stays until dismissed. HideProgress dismisses it, so a
		// new progress message must clear the previous one first.
		if g.progressToast != nil {
			g.progressToast.Dismiss()
		}
		toast := adw.NewToast(message)
		toast.SetTimeout(0)
		g.toast.AddToast(toast)
		g.progressToast = toast
	})
}

func (g *GtkUI) HideProgress() {
	glib.IdleAdd(func() {
		if g.progressToast != nil {
			g.progressToast.Dismiss()
			g.progressToast = nil
		}
	})
}

func (g *GtkUI) RefreshDevice(device *domain.DeviceNode) {
	g.mu.Lock()
	g.deviceMap[device.ID] = device

	// Remove old interface entries for this device and re-add
	for cat, entries := range g.interfacesByCategory {
		var filtered []interfaceEntry
		for _, e := range entries {
			if e.Device.ID != device.ID {
				filtered = append(filtered, e)
			}
		}
		g.interfacesByCategory[cat] = filtered
	}
	// Re-index this device's interfaces
	for _, iface := range device.Interfaces {
		g.interfacesByCategory[iface.Category] = append(
			g.interfacesByCategory[iface.Category],
			interfaceEntry{Device: device, Interface: iface},
		)
	}
	g.mu.Unlock()

	glib.IdleAdd(func() {
		g.rebuildSidebar()
		if g.selectedView != "" {
			g.rebuildDeviceList()
		}
	})
}
