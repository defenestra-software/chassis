package gtk

import (
	"fmt"
	"sort"
	"strings"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"defenestra.io/chassis/internal/core/domain"
)

func (g *GtkUI) buildSidebarPage() *adw.NavigationPage {
	// Header bar with hamburger menu
	headerBar := adw.NewHeaderBar()
	headerBar.SetShowEndTitleButtons(false)

	menuBtn := gtk.NewMenuButton()
	menuBtn.SetIconName("open-menu-symbolic")
	menuBtn.SetMenuModel(g.buildAppMenu())
	headerBar.PackEnd(menuBtn)

	// Refresh button
	refreshBtn := gtk.NewButtonFromIconName("view-refresh-symbolic")
	refreshBtn.SetTooltipText("Rescan Devices")
	refreshBtn.ConnectClicked(func() {
		if g.onRefresh != nil {
			go func() {
				_ = g.onRefresh()
			}()
		}
	})
	headerBar.PackStart(refreshBtn)

	// Sidebar content
	contentBox := gtk.NewBox(gtk.OrientationVertical, 0)

	// Categories section
	catLabel := gtk.NewLabel("Categories")
	catLabel.AddCSSClass("heading")
	catLabel.AddCSSClass("dim-label")
	catLabel.SetHAlign(gtk.AlignStart)
	catLabel.SetMarginStart(12)
	catLabel.SetMarginTop(12)
	catLabel.SetMarginBottom(6)
	contentBox.Append(catLabel)

	g.categoryList = gtk.NewListBox()
	g.categoryList.AddCSSClass("navigation-sidebar")
	g.categoryList.SetSelectionMode(gtk.SelectionSingle)
	g.categoryList.ConnectRowSelected(func(row *gtk.ListBoxRow) {
		if row == nil || g.restoringSelection {
			return
		}
		// Deselect IOMMU list when category is selected
		if g.iommuList != nil {
			g.iommuList.UnselectAll()
		}
		g.selectView("class:" + row.Name())
	})
	contentBox.Append(g.categoryList)

	// Separator between categories and IOMMU groups
	g.iommuSection = gtk.NewBox(gtk.OrientationVertical, 0)
	g.iommuSection.SetVisible(false)

	iommuSep := gtk.NewSeparator(gtk.OrientationHorizontal)
	iommuSep.SetMarginTop(6)
	iommuSep.SetMarginBottom(6)
	iommuSep.SetMarginStart(12)
	iommuSep.SetMarginEnd(12)
	g.iommuSection.Append(iommuSep)

	g.iommuList = gtk.NewListBox()
	g.iommuList.AddCSSClass("navigation-sidebar")
	g.iommuList.SetSelectionMode(gtk.SelectionSingle)
	g.iommuList.ConnectRowSelected(func(row *gtk.ListBoxRow) {
		if row == nil || g.restoringSelection {
			return
		}
		// Deselect category list when IOMMU is selected
		g.categoryList.UnselectAll()
		g.selectView("iommu:all")
	})
	g.iommuSection.Append(g.iommuList)
	contentBox.Append(g.iommuSection)

	// Scrolled window for sidebar
	scroll := gtk.NewScrolledWindow()
	scroll.SetChild(contentBox)
	scroll.SetVExpand(true)
	scroll.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)

	// Wrap in toolbar view
	toolbarView := adw.NewToolbarView()
	toolbarView.AddTopBar(headerBar)
	toolbarView.SetContent(scroll)

	return adw.NewNavigationPage(toolbarView, "Chassis")
}

func (g *GtkUI) rebuildSidebar() {
	// Clear category list
	for {
		row := g.categoryList.RowAtIndex(0)
		if row == nil {
			break
		}
		g.categoryList.Remove(row)
	}

	g.mu.RLock()
	byCat := g.interfacesByCategory
	groups := g.iommuGroups
	g.mu.RUnlock()

	// Build sorted category list - show all categories even if empty,
	// so they appear immediately when devices are added on refresh.
	cats := make([]categoryInfo, len(categoryRegistry))
	copy(cats, categoryRegistry)
	sort.Slice(cats, func(i, j int) bool { return cats[i].sortOrder < cats[j].sortOrder })

	for _, cat := range cats {
		count := len(byCat[cat.class])
		row := gtk.NewListBoxRow()
		row.SetName(string(cat.class))

		box := gtk.NewBox(gtk.OrientationHorizontal, 12)
		box.SetMarginTop(8)
		box.SetMarginBottom(8)
		box.SetMarginStart(12)
		box.SetMarginEnd(12)

		icon := gtk.NewImageFromIconName(cat.iconName)
		box.Append(icon)

		label := gtk.NewLabel(cat.displayName)
		label.SetHAlign(gtk.AlignStart)
		label.SetHExpand(true)
		box.Append(label)

		countLabel := gtk.NewLabel(fmt.Sprintf("%d", count))
		countLabel.AddCSSClass("dim-label")
		box.Append(countLabel)

		row.SetChild(box)
		g.categoryList.Append(row)
	}

	// IOMMU Groups - separate list below the separator
	for {
		row := g.iommuList.RowAtIndex(0)
		if row == nil {
			break
		}
		g.iommuList.Remove(row)
	}

	var groupCount int
	for num, devices := range groups {
		if num >= 0 && len(devices) > 0 {
			groupCount++
		}
	}
	g.hasIOMMU = groupCount > 0
	g.iommuSection.SetVisible(g.hasIOMMU)

	if g.hasIOMMU {
		row := gtk.NewListBoxRow()
		row.SetName("__iommu__")

		box := gtk.NewBox(gtk.OrientationHorizontal, 12)
		box.SetMarginTop(8)
		box.SetMarginBottom(8)
		box.SetMarginStart(12)
		box.SetMarginEnd(12)

		icon := gtk.NewImageFromIconName("network-workgroup-symbolic")
		box.Append(icon)

		label := gtk.NewLabel("IOMMU Groups")
		label.SetHAlign(gtk.AlignStart)
		label.SetHExpand(true)
		box.Append(label)

		countLabel := gtk.NewLabel(fmt.Sprintf("%d", groupCount))
		countLabel.AddCSSClass("dim-label")
		box.Append(countLabel)

		row.SetChild(box)
		g.iommuList.Append(row)
	}

	// Restore previously selected row without triggering selectView
	g.mu.RLock()
	selectedView := g.selectedView
	g.mu.RUnlock()

	if selectedView != "" {
		g.restoringSelection = true
		if selectedView == "iommu:all" {
			// Select the IOMMU row
			if row := g.iommuList.RowAtIndex(0); row != nil {
				g.iommuList.SelectRow(row)
			}
		} else if strings.HasPrefix(selectedView, "class:") {
			targetName := strings.TrimPrefix(selectedView, "class:")
			for i := 0; ; i++ {
				row := g.categoryList.RowAtIndex(i)
				if row == nil {
					break
				}
				if row.Name() == targetName {
					g.categoryList.SelectRow(row)
					break
				}
			}
		}
		g.restoringSelection = false
	}
}

func (g *GtkUI) selectView(view string) {
	g.mu.Lock()
	g.selectedView = view
	g.mu.Unlock()

	// Pop back to list page if on detail page
	if g.onDetailPage {
		g.navView.Pop()
		g.onDetailPage = false
	}

	// Update list page title
	g.listTitle = g.titleForView(view)
	g.listPage.SetTitle(g.listTitle)

	g.rebuildDeviceList()

	// When collapsed (narrow window), navigate from sidebar to content
	g.splitView.SetShowContent(true)
}

func (g *GtkUI) titleForView(view string) string {
	if view == "iommu:all" {
		return "IOMMU Groups"
	}
	if strings.HasPrefix(view, "iommu:") {
		num := strings.TrimPrefix(view, "iommu:")
		return fmt.Sprintf("IOMMU Group %s", num)
	}
	class := domain.DeviceClass(strings.TrimPrefix(view, "class:"))
	for _, cat := range categoryRegistry {
		if cat.class == class {
			return cat.displayName
		}
	}
	return "Devices"
}
