package tui

import (
	"reflect"
	"testing"

	"charm.land/bubbles/v2/key"
)

func TestKeyScopes_noSharedKeys(t *testing.T) {
	t.Parallel()
	g := newGlobalKeyMap()
	r := newReviewKeyMap()
	global := map[string]key.Binding{
		"reviewTab":    g.reviewTab,
		"logTab":       g.logTab,
		"bookmarksTab": g.bookmarksTab,
		"help":         g.help,
		"quit":         g.quit,
	}
	helpOverlay := map[string]key.Binding{
		"closeHelp":  g.closeHelp,
		"scrollDown": g.scrollDown,
		"scrollUp":   g.scrollUp,
	}
	review := map[string]key.Binding{
		"switchPane":  r.switchPane,
		"down":        r.down,
		"up":          r.up,
		"halfDown":    r.halfDown,
		"halfUp":      r.halfUp,
		"top":         r.top,
		"bottom":      r.bottom,
		"nextHunk":    r.nextHunk,
		"prevHunk":    r.prevHunk,
		"nextFile":    r.nextFile,
		"prevFile":    r.prevFile,
		"workingCopy": r.workingCopy,
		"selectHunk":  r.selectHunk,
		"selectFile":  r.selectFile,
		"commit":      r.commit,
		"fold":        r.fold,
		"foldAll":     r.foldAll,
		"removed":     r.removed,
		"removedAll":  r.removedAll,
		"reload":      r.reload,
		"edit":        r.edit,
		"full":        r.full,
	}
	if got, want := reflect.TypeFor[globalKeyMap]().NumField(), len(global)+len(helpOverlay)+1; got != want {
		t.Fatalf("globalKeyMap has %d bindings, the scopes list %d; add the new binding to a scope", got, want)
	}
	if got, want := reflect.TypeFor[reviewKeyMap]().NumField(), len(review); got != want {
		t.Fatalf("reviewKeyMap has %d bindings, the review scope lists %d; add the new binding to it", got, want)
	}
	tests := map[string][]map[string]key.Binding{
		"global and review keys": {global, review},
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
