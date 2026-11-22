package ui_test

import (
	"slices"
	"testing"

	"charm.land/bubbles/v2/key"

	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

func TestNewNavKeys_success(t *testing.T) {
	t.Parallel()
	nav := ui.NewNavKeys("next item", "previous item", "first item", "last item")
	tests := map[string]struct {
		binding  key.Binding
		keys     []string
		helpKey  string
		helpDesc string
	}{
		"down":      {binding: nav.Down, keys: []string{"j", "down"}, helpKey: "j", helpDesc: "next item"},
		"up":        {binding: nav.Up, keys: []string{"k", "up"}, helpKey: "k", helpDesc: "previous item"},
		"half down": {binding: nav.HalfDown, keys: []string{"ctrl+d", "pgdown"}, helpKey: "^d", helpDesc: "half page down"},
		"half up":   {binding: nav.HalfUp, keys: []string{"ctrl+u", "pgup"}, helpKey: "^u", helpDesc: "half page up"},
		"top":       {binding: nav.Top, keys: []string{"g", "home"}, helpKey: "g", helpDesc: "first item"},
		"bottom":    {binding: nav.Bottom, keys: []string{"G", "end"}, helpKey: "G", helpDesc: "last item"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := tt.binding.Keys(); !slices.Equal(got, tt.keys) {
				t.Errorf("%s keys = %q, want %q", name, got, tt.keys)
			}
			if got := tt.binding.Help(); got.Key != tt.helpKey || got.Desc != tt.helpDesc {
				t.Errorf("%s help = %q %q, want %q %q", name, got.Key, got.Desc, tt.helpKey, tt.helpDesc)
			}
		})
	}
}
