package tui

import (
	"reflect"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

func TestKeyScopes_noSharedKeys(t *testing.T) {
	t.Parallel()
	g := newGlobalKeyMap()
	r := newReviewKeyMap()
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

func TestReviewResolve_success(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		msg  tea.KeyPressMsg
		want reviewIntent
	}{
		"tab switches the pane":             {msg: tea.KeyPressMsg{Code: tea.KeyTab}, want: reviewSwitchPaneIntent},
		"j moves down":                      {msg: tea.KeyPressMsg{Code: 'j', Text: "j"}, want: reviewDownIntent},
		"down arrow moves down":             {msg: tea.KeyPressMsg{Code: tea.KeyDown}, want: reviewDownIntent},
		"k moves up":                        {msg: tea.KeyPressMsg{Code: 'k', Text: "k"}, want: reviewUpIntent},
		"up arrow moves up":                 {msg: tea.KeyPressMsg{Code: tea.KeyUp}, want: reviewUpIntent},
		"ctrl+d pages half down":            {msg: tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}, want: reviewHalfDownIntent},
		"pgdown pages half down":            {msg: tea.KeyPressMsg{Code: tea.KeyPgDown}, want: reviewHalfDownIntent},
		"ctrl+u pages half up":              {msg: tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}, want: reviewHalfUpIntent},
		"pgup pages half up":                {msg: tea.KeyPressMsg{Code: tea.KeyPgUp}, want: reviewHalfUpIntent},
		"g goes to the top":                 {msg: tea.KeyPressMsg{Code: 'g', Text: "g"}, want: reviewTopIntent},
		"home goes to the top":              {msg: tea.KeyPressMsg{Code: tea.KeyHome}, want: reviewTopIntent},
		"G goes to the bottom":              {msg: tea.KeyPressMsg{Code: 'G', Text: "G"}, want: reviewBottomIntent},
		"end goes to the bottom":            {msg: tea.KeyPressMsg{Code: tea.KeyEnd}, want: reviewBottomIntent},
		"] jumps to the next hunk":          {msg: tea.KeyPressMsg{Code: ']', Text: "]"}, want: reviewNextHunkIntent},
		"[ jumps to the previous hunk":      {msg: tea.KeyPressMsg{Code: '[', Text: "["}, want: reviewPrevHunkIntent},
		"J moves to the next file":          {msg: tea.KeyPressMsg{Code: 'J', Text: "J"}, want: reviewNextFileIntent},
		"K moves to the previous file":      {msg: tea.KeyPressMsg{Code: 'K', Text: "K"}, want: reviewPrevFileIntent},
		"@ loads the working copy":          {msg: tea.KeyPressMsg{Code: '@', Text: "@"}, want: reviewWorkingCopyIntent},
		"R reloads the change":              {msg: tea.KeyPressMsg{Code: 'R', Text: "R"}, want: reviewReloadIntent},
		"f toggles the full file":           {msg: tea.KeyPressMsg{Code: 'f', Text: "f"}, want: reviewFullIntent},
		"c opens the commit dialog":         {msg: tea.KeyPressMsg{Code: 'c', Text: "c"}, want: reviewCommitIntent},
		"e opens the editor":                {msg: tea.KeyPressMsg{Code: 'e', Text: "e"}, want: reviewEditIntent},
		"z folds the hunk":                  {msg: tea.KeyPressMsg{Code: 'z', Text: "z"}, want: reviewFoldIntent},
		"enter folds the hunk":              {msg: tea.KeyPressMsg{Code: tea.KeyEnter}, want: reviewFoldIntent},
		"d shows removed lines of the hunk": {msg: tea.KeyPressMsg{Code: 'd', Text: "d"}, want: reviewRemovedIntent},
		"D shows removed lines of all":      {msg: tea.KeyPressMsg{Code: 'D', Text: "D"}, want: reviewRemovedAllIntent},
		"Z folds all hunks":                 {msg: tea.KeyPressMsg{Code: 'Z', Text: "Z"}, want: reviewFoldAllIntent},
		"space selects the hunk":            {msg: tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}, want: reviewSelectHunkIntent},
		"S selects the file":                {msg: tea.KeyPressMsg{Code: 'S', Text: "S"}, want: reviewSelectFileIntent},
		"unbound keys do nothing":           {msg: tea.KeyPressMsg{Code: 'x', Text: "x"}, want: noReviewIntent},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := newReviewKeyMap().resolve(tt.msg)

			if got != tt.want {
				t.Errorf("resolve(%q) = %v, want %v", tt.msg.String(), got, tt.want)
			}
		})
	}
}

func TestCommitDialogResolve_success(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		msg  tea.KeyPressMsg
		want commitDialogIntent
	}{
		"esc cancels":                {msg: tea.KeyPressMsg{Code: tea.KeyEscape}, want: cancelCommitIntent},
		"ctrl+r regenerates":         {msg: tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}, want: regenerateMessageIntent},
		"ctrl+enter commits":         {msg: tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl}, want: submitCommitIntent},
		"ctrl+s commits":             {msg: tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}, want: submitCommitIntent},
		"text goes to the textarea":  {msg: tea.KeyPressMsg{Code: 'q', Text: "q"}, want: typeMessageIntent},
		"enter goes to the textarea": {msg: tea.KeyPressMsg{Code: tea.KeyEnter}, want: typeMessageIntent},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := resolveCommitDialog(tt.msg)

			if got != tt.want {
				t.Errorf("resolveCommitDialog(%q) = %v, want %v", tt.msg.String(), got, tt.want)
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
