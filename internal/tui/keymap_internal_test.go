package tui

import (
	"reflect"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/tui/review"
)

func TestKeyScopes_noSharedKeys(t *testing.T) {
	t.Parallel()
	g := newGlobalKeyMap()
	global := map[string]key.Binding{
		"reviewTab":     g.reviewTab,
		"logTab":        g.logTab,
		"bookmarksTab":  g.bookmarksTab,
		"operationsTab": g.operationsTab,
		"help":          g.help,
		"quit":          g.quit,
	}
	helpOverlay := map[string]key.Binding{
		"closeHelp":  g.closeHelp,
		"scrollDown": g.scrollDown,
		"scrollUp":   g.scrollUp,
	}
	reviewKeys := review.Bindings()
	if got, want := reflect.TypeFor[globalKeyMap]().NumField(), len(global)+len(helpOverlay)+1; got != want {
		t.Fatalf("globalKeyMap has %d bindings, the scopes list %d; add the new binding to a scope", got, want)
	}
	tests := map[string][]map[string]key.Binding{
		"global and review keys": {global, reviewKeys},
		"help overlay keys":      {helpOverlay},
	}
	for name, scope := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			owner := map[string]string{}
			for _, set := range scope {
				for binding, b := range set {
					for _, k := range b.Keys() {
						if other, ok := owner[k]; ok {
							t.Errorf("key %q is bound to both %s and %s", k, other, binding)
						}
						owner[k] = binding
					}
				}
			}
		})
	}
}

func TestGlobalResolve_success(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		msg  tea.KeyPressMsg
		want globalIntent
	}{
		"1 shows the review tab":     {msg: tea.KeyPressMsg{Code: '1', Text: "1"}, want: showReviewIntent},
		"2 shows the log tab":        {msg: tea.KeyPressMsg{Code: '2', Text: "2"}, want: showLogIntent},
		"3 shows the bookmarks tab":  {msg: tea.KeyPressMsg{Code: '3', Text: "3"}, want: showBookmarksIntent},
		"4 shows the operations tab": {msg: tea.KeyPressMsg{Code: '4', Text: "4"}, want: showOperationsIntent},
		"? opens the help":           {msg: tea.KeyPressMsg{Code: '?', Text: "?"}, want: helpIntent},
		"q quits":                    {msg: tea.KeyPressMsg{Code: 'q', Text: "q"}, want: quitIntent},
		"other keys stay with views": {msg: tea.KeyPressMsg{Code: 'j', Text: "j"}, want: noGlobalIntent},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := newGlobalKeyMap().resolve(tt.msg)

			if got != tt.want {
				t.Errorf("resolve(%q) = %v, want %v", tt.msg.String(), got, tt.want)
			}
		})
	}
}

func TestHelpOverlayResolve_success(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		msg  tea.KeyPressMsg
		want helpOverlayIntent
	}{
		"? closes the help":       {msg: tea.KeyPressMsg{Code: '?', Text: "?"}, want: closeHelpIntent},
		"esc closes the help":     {msg: tea.KeyPressMsg{Code: tea.KeyEscape}, want: closeHelpIntent},
		"q closes the help":       {msg: tea.KeyPressMsg{Code: 'q', Text: "q"}, want: closeHelpIntent},
		"j scrolls down":          {msg: tea.KeyPressMsg{Code: 'j', Text: "j"}, want: scrollHelpDownIntent},
		"down arrow scrolls down": {msg: tea.KeyPressMsg{Code: tea.KeyDown}, want: scrollHelpDownIntent},
		"k scrolls up":            {msg: tea.KeyPressMsg{Code: 'k', Text: "k"}, want: scrollHelpUpIntent},
		"up arrow scrolls up":     {msg: tea.KeyPressMsg{Code: tea.KeyUp}, want: scrollHelpUpIntent},
		"other keys are ignored":  {msg: tea.KeyPressMsg{Code: '1', Text: "1"}, want: noHelpOverlayIntent},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := newGlobalKeyMap().resolveHelpOverlay(tt.msg)

			if got != tt.want {
				t.Errorf("resolveHelpOverlay(%q) = %v, want %v", tt.msg.String(), got, tt.want)
			}
		})
	}
}

func TestRegistry_success(t *testing.T) {
	t.Parallel()
	m := New(t.Context(), nil, nil, "@")

	for _, entry := range m.registry() {
		t.Run(entry.label, func(t *testing.T) {
			t.Parallel()
			k := entry.key.Keys()[0]

			got := m.keys.resolve(tea.KeyPressMsg{Code: rune(k[0]), Text: k})

			if got != entry.intent {
				t.Errorf("resolve(%q) = %v, want the %s tab intent %v", k, got, entry.label, entry.intent)
			}
		})
	}
}
