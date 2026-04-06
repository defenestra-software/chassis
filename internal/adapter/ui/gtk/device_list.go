package gtk

import (
	"fmt"
	"sort"
	"strings"

	"defenestra.io/chassis/internal/core/domain"

	kit "gitlab.com/defenestra-software/go-adw-kit"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

func (g *GtkUI) buildDeviceListPage() *adw.NavigationPage {
	// Header bar with search toggle and patched CSD controls
	headerBar := adw.NewHeaderBar()
	kit.PatchHeaderBar(headerBar, g.window)

	g.searchBtn = gtk.NewToggleButton()
	g.searchBtn.SetIconName("system-search-symbolic")
	g.searchBtn.SetTooltipText("Search devices")
	headerBar.PackEnd(g.searchBtn)

	// Search bar
	g.searchEntry = gtk.NewSearchEntry()
	g.searchEntry.SetPlaceholderText("Search devices…")
	g.searchEntry.SetHExpand(true)

	g.searchBar = gtk.NewSearchBar()
	g.searchBar.SetChild(g.searchEntry)
	g.searchBar.SetSearchMode(false)
	g.searchBar.ConnectEntry(g.searchEntry)

	g.searchBtn.ConnectToggled(func() {
		g.searchBar.SetSearchMode(g.searchBtn.Active())
		if g.searchBtn.Active() {
			g.searchEntry.GrabFocus()
		}
	})

	g.searchEntry.ConnectSearchChanged(func() {
		g.onSearchChanged(g.searchEntry.Text())
	})

	// Device list
	g.deviceListBox = gtk.NewListBox()
	g.deviceListBox.AddCSSClass("boxed-list")
	g.deviceListBox.SetSelectionMode(gtk.SelectionNone)
	g.deviceListBox.ConnectRowActivated(func(row *gtk.ListBoxRow) {
		// Row name format: "deviceID|scrollTag" or just "deviceID"
		name := row.Name()
		deviceID := name
		scrollTag := ""
		if idx := strings.LastIndex(name, "|"); idx > 0 {
			deviceID = name[:idx]
			scrollTag = name[idx+1:]
		}
		g.mu.RLock()
		device, ok := g.deviceMap[deviceID]
		g.mu.RUnlock()
		if ok {
			g.showDeviceDetailScrollTo(device, scrollTag)
		}
	})

	// Device list in a scrolled window
	listBox := gtk.NewBox(gtk.OrientationVertical, 0)
	listBox.SetMarginStart(12)
	listBox.SetMarginEnd(12)
	listBox.SetMarginTop(12)
	listBox.SetMarginBottom(12)
	listBox.Append(g.deviceListBox)

	listClamp := adw.NewClamp()
	listClamp.SetMaximumSize(700)
	listClamp.SetChild(listBox)

	scroll := gtk.NewScrolledWindow()
	scroll.SetChild(listClamp)
	scroll.SetVExpand(true)
	scroll.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)

	// Placeholder for empty state
	placeholder := adw.NewStatusPage()
	placeholder.SetIconName("computer-symbolic")
	placeholder.SetTitle("Select a Category")
	placeholder.SetDescription("Choose a device category from the sidebar to browse hardware")

	// IOMMU grouped view
	g.iommuViewBox = gtk.NewBox(gtk.OrientationVertical, 12)
	g.iommuViewBox.SetMarginStart(12)
	g.iommuViewBox.SetMarginEnd(12)
	g.iommuViewBox.SetMarginTop(12)
	g.iommuViewBox.SetMarginBottom(12)

	iommuClamp := adw.NewClamp()
	iommuClamp.SetMaximumSize(700)
	iommuClamp.SetChild(g.iommuViewBox)

	iommuScroll := gtk.NewScrolledWindow()
	iommuScroll.SetChild(iommuClamp)
	iommuScroll.SetVExpand(true)
	iommuScroll.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)

	// Stack to switch between placeholder, device list, and IOMMU view
	g.contentStack = gtk.NewStack()
	g.contentStack.AddNamed(placeholder, "placeholder")
	g.contentStack.AddNamed(scroll, "list")
	g.contentStack.AddNamed(iommuScroll, "iommu")
	g.contentStack.SetVisibleChildName("placeholder")

	// Container
	contentBox := gtk.NewBox(gtk.OrientationVertical, 0)
	contentBox.Append(g.searchBar)
	contentBox.Append(g.contentStack)

	toolbarView := adw.NewToolbarView()
	toolbarView.AddTopBar(headerBar)
	toolbarView.SetContent(contentBox)

	page := adw.NewNavigationPage(toolbarView, "Devices")
	return page
}

func (g *GtkUI) rebuildDeviceList() {
	g.mu.RLock()
	view := g.selectedView
	filter := g.filterText
	g.mu.RUnlock()

	// When searching with text, search ALL devices globally
	// (except in IOMMU grouped view which filters within itself)
	if filter != "" && view != "iommu:all" {
		g.rebuildGlobalSearchResults(filter)
		return
	}

	// IOMMU grouped view is special - uses preference groups instead of flat list
	if view == "iommu:all" {
		g.rebuildIOMMUGroupedView()
		return
	}

	// Clear existing rows
	for {
		row := g.deviceListBox.RowAtIndex(0)
		if row == nil {
			break
		}
		g.deviceListBox.Remove(row)
	}

	entries := g.getEntriesForView()
	if len(entries) == 0 {
		g.contentStack.SetVisibleChildName("placeholder")
		return
	}

	for _, entry := range entries {
		row := g.createDeviceRow(entry)
		g.deviceListBox.Append(row)
	}

	g.contentStack.SetVisibleChildName("list")
}

func (g *GtkUI) rebuildGlobalSearchResults(filter string) {
	// Clear existing rows
	for {
		row := g.deviceListBox.RowAtIndex(0)
		if row == nil {
			break
		}
		g.deviceListBox.Remove(row)
	}

	g.mu.RLock()
	allDevices := g.deviceMap
	g.mu.RUnlock()

	// Collect matching interface entries from across all categories
	var matches []interfaceEntry
	seen := map[string]bool{}
	for _, node := range allDevices {
		if !g.nodeMatchesFilter(node, filter) {
			continue
		}
		if len(node.Interfaces) == 0 {
			continue
		}
		// Add the primary interface (first one) for search results
		// to avoid listing the same device N times
		if !seen[node.ID] {
			seen[node.ID] = true
			matches = append(matches, interfaceEntry{
				Device:    node,
				Interface: node.Interfaces[0],
			})
		}
	}

	if len(matches) == 0 {
		g.contentStack.SetVisibleChildName("placeholder")
		return
	}

	// Sort by name for consistent ordering
	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Device.Name < matches[j].Device.Name
	})

	for _, entry := range matches {
		row := g.createDeviceRow(entry)
		g.deviceListBox.Append(row)
	}

	g.listPage.SetTitle(fmt.Sprintf("Search: %s", filter))
	g.contentStack.SetVisibleChildName("list")
}

func (g *GtkUI) rebuildIOMMUGroupedView() {
	// Clear existing children
	for child := g.iommuViewBox.FirstChild(); child != nil; child = g.iommuViewBox.FirstChild() {
		g.iommuViewBox.Remove(child)
	}

	g.mu.RLock()
	groups := g.iommuGroups
	filter := g.filterText
	g.mu.RUnlock()

	// Collect and sort group numbers
	var groupNums []int
	for num, devices := range groups {
		if num >= 0 && len(devices) > 0 {
			groupNums = append(groupNums, num)
		}
	}
	sort.Ints(groupNums)

	for _, num := range groupNums {
		devices := groups[num]

		group := adw.NewPreferencesGroup()
		group.SetTitle(fmt.Sprintf("Group %d", num))

		hasVisible := false
		for _, node := range devices {
			if filter != "" && !g.nodeMatchesFilter(node, filter) {
				continue
			}
			row := adw.NewActionRow()
			row.SetTitle(node.DisplayName())
			if node.Driver != "" {
				row.SetSubtitle(node.Driver)
			} else {
				row.SetSubtitle("No driver")
			}
			icon := gtk.NewImageFromIconName(g.getIconName(node))
			row.AddPrefix(icon)
			row.SetActivatable(true)

			deviceID := node.ID
			row.ConnectActivated(func() {
				g.mu.RLock()
				device, ok := g.deviceMap[deviceID]
				g.mu.RUnlock()
				if ok {
					g.showDeviceDetail(device)
				}
			})

			group.Add(row)
			hasVisible = true
		}

		if hasVisible {
			g.iommuViewBox.Append(group)
		}
	}

	g.contentStack.SetVisibleChildName("iommu")
}

func (g *GtkUI) getEntriesForView() []interfaceEntry {
	g.mu.RLock()
	defer g.mu.RUnlock()

	view := g.selectedView
	if view == "" {
		return nil
	}

	// IOMMU groups return devices (not interfaces) - wrap in entries
	if strings.HasPrefix(view, "iommu:") {
		return nil // Handled separately by rebuildIOMMUGroupedView
	}

	class := domain.DeviceClass(strings.TrimPrefix(view, "class:"))
	return g.interfacesByCategory[class]
}

func (g *GtkUI) createDeviceRow(entry interfaceEntry) *gtk.ListBoxRow {
	node := entry.Device
	iface := entry.Interface

	row := gtk.NewListBoxRow()
	// Encode device ID + scroll tag so the click handler knows where to scroll
	rowName := node.ID
	if iface.ScrollTag != "" {
		rowName = node.ID + "|" + iface.ScrollTag
	}
	row.SetName(rowName)
	row.SetActivatable(true)

	actionRow := adw.NewActionRow()
	// Use interface Name override if set (e.g., sensor source names like
	// "Gigabyte Sensors" instead of the parent device name)
	title := node.DisplayName()
	if iface.Name != "" {
		title = iface.Name
	}
	actionRow.SetTitle(title)
	actionRow.SetTitleLines(2)
	actionRow.SetTooltipText(node.Name)

	// Subtitle comes from the interface - already computed during extraction
	if iface.Subtitle != "" {
		actionRow.SetSubtitle(iface.Subtitle)
	} else if node.Driver != "" {
		actionRow.SetSubtitle(node.Driver)
	}

	// Icon from the interface's category
	iconName := getCategoryIcon(iface.Category)
	if iconName == "" {
		iconName = g.getIconName(node)
	}
	icon := gtk.NewImageFromIconName(iconName)
	actionRow.AddPrefix(icon)

	row.SetChild(actionRow)
	return row
}

// createDeviceRowSimple creates a row for IOMMU group views (no interface context).
func (g *GtkUI) createDeviceRowSimple(node *domain.DeviceNode) *gtk.ListBoxRow {
	row := gtk.NewListBoxRow()
	row.SetName(node.ID)
	row.SetActivatable(true)

	actionRow := adw.NewActionRow()
	actionRow.SetTitle(node.DisplayName())
	actionRow.SetTitleLines(2)
	actionRow.SetTooltipText(node.Name)

	if node.Driver != "" {
		actionRow.SetSubtitle(node.Driver)
	}

	icon := gtk.NewImageFromIconName(g.getIconName(node))
	actionRow.AddPrefix(icon)

	row.SetChild(actionRow)
	return row
}

func (g *GtkUI) onSearchChanged(text string) {
	g.mu.Lock()
	g.filterText = strings.ToLower(strings.TrimSpace(text))
	filter := g.filterText
	g.mu.Unlock()

	g.rebuildDeviceList()

	// Restore category title when search is cleared
	if filter == "" {
		g.mu.RLock()
		view := g.selectedView
		g.mu.RUnlock()
		if view != "" {
			g.listPage.SetTitle(g.titleForView(view))
		} else {
			g.listPage.SetTitle("Devices")
		}
	}

	// Show content when searching in collapsed mode
	if filter != "" {
		g.splitView.SetShowContent(true)
	}
}

func (g *GtkUI) nodeMatchesFilter(node *domain.DeviceNode, filter string) bool {
	checks := []string{
		node.Name, node.Vendor, node.Product, node.Driver,
		node.ID, node.VendorID, node.ProductID, string(node.Class),
	}
	for _, s := range checks {
		if strings.Contains(strings.ToLower(s), filter) {
			return true
		}
	}
	return false
}
