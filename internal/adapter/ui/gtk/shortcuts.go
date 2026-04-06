package gtk

import (
	gtklib "github.com/diamondburned/gotk4/pkg/gtk/v4"
)

const shortcutsXML = `<?xml version="1.0" encoding="UTF-8"?>
<interface>
  <object class="GtkShortcutsWindow" id="shortcuts">
    <property name="modal">1</property>
    <child>
      <object class="GtkShortcutsSection">
        <property name="section-name">shortcuts</property>
        <child>
          <object class="GtkShortcutsGroup">
            <property name="title">General</property>
            <child>
              <object class="GtkShortcutsShortcut">
                <property name="title">Search devices</property>
                <property name="accelerator">&lt;Control&gt;f</property>
              </object>
            </child>
            <child>
              <object class="GtkShortcutsShortcut">
                <property name="title">Keyboard shortcuts</property>
                <property name="accelerator">&lt;Control&gt;question</property>
              </object>
            </child>
            <child>
              <object class="GtkShortcutsShortcut">
                <property name="title">Quit</property>
                <property name="accelerator">&lt;Control&gt;q</property>
              </object>
            </child>
          </object>
        </child>
      </object>
    </child>
  </object>
</interface>
`

// showShortcutsDialog presents the keyboard shortcuts window.
func showShortcutsDialog(parent *gtklib.Window) {
	builder := gtklib.NewBuilderFromString(shortcutsXML)
	obj := builder.GetObject("shortcuts")
	if obj == nil {
		return
	}
	win := obj.Cast().(*gtklib.ShortcutsWindow)
	win.SetTransientFor(parent)
	win.Show()
}
