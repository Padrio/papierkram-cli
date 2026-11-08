package cli

import (
	"strings"
	"testing"

	"github.com/Padrio/papierkram-cli/internal/registry"
)

// Review M3: the flag help names the preset sources exactly once, from
// one text (presetNote), for list filters, create fields and path flags
// (person … --company) alike.
func TestPresetFlagHelpFromOneText(t *testing.T) {
	app, _ := apiApp(t)
	root := NewRootCmd(app)
	for _, e := range registry.Entries() {
		if e.Preset == "" || presetsID(e) {
			continue
		}
		for _, f := range e.Filters {
			if f.Flag == e.Preset && strings.Contains(f.Help, "PAPIERKRAM_") {
				t.Errorf("%s %s: Registry-Hilfe von --%s wiederholt die Voreinstellung: %q", e.Resource, e.Action, f.Flag, f.Help)
			}
		}
		cmd, _, _ := root.Find([]string{e.Resource, e.Action})
		fl := cmd.Flags().Lookup(e.Preset)
		if fl == nil || strings.Count(fl.Usage, presetNote(e.Preset)) != 1 || strings.Count(fl.Usage, "PAPIERKRAM_") != 1 {
			t.Errorf("%s %s --%s: Hilfe %q", e.Resource, e.Action, e.Preset, fl.Usage)
		}
	}
}
