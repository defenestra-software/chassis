package gtk

import (
	"defenestra.io/chassis/internal/shared"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	gtklib "github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// showAboutDialog presents the About dialog.
func showAboutDialog(parent *gtklib.Window) {
	about := adw.NewAboutDialog()

	about.SetApplicationName("Chassis")
	about.SetApplicationIcon("io.defenestra.Chassis")
	about.SetVersion(shared.AppVersion())
	about.SetDeveloperName("Defenestra, LLC")
	about.SetCopyright("\u00a9 2025 Defenestra, LLC")
	about.SetWebsite("https://defenestra.io/products/chassis/")
	about.SetIssueURL("https://forum.defenestra.io/")
	about.SetLicenseType(gtklib.LicenseGPL30)
	about.SetComments("Device Manager for Linux")

	about.AddLink("Wiki", "https://wiki.defenestra.io/")
	about.AddLink("Forum", "https://forum.defenestra.io/")

	about.SetDevelopers([]string{
		"Defenestra, LLC https://defenestra.io",
	})

	about.Present(parent)
}
