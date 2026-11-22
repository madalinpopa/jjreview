package review

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

func TestBindings_listEveryKey(t *testing.T) {
	t.Parallel()

	got := len(Bindings())

	if want := reflect.TypeFor[keyMap]().NumField() - 1 + reflect.TypeFor[ui.NavKeys]().NumField(); got != want {
		t.Errorf("Bindings() lists %d bindings, keyMap has %d; add the new binding to Bindings", got, want)
	}
}

func TestReviewResolve_success(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		msg  tea.KeyPressMsg
		want intent
	}{
		"tab switches the pane":             {msg: tea.KeyPressMsg{Code: tea.KeyTab}, want: switchPaneIntent},
		"j moves down":                      {msg: tea.KeyPressMsg{Code: 'j', Text: "j"}, want: downIntent},
		"down arrow moves down":             {msg: tea.KeyPressMsg{Code: tea.KeyDown}, want: downIntent},
		"k moves up":                        {msg: tea.KeyPressMsg{Code: 'k', Text: "k"}, want: upIntent},
		"up arrow moves up":                 {msg: tea.KeyPressMsg{Code: tea.KeyUp}, want: upIntent},
		"ctrl+d pages half down":            {msg: tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}, want: halfDownIntent},
		"pgdown pages half down":            {msg: tea.KeyPressMsg{Code: tea.KeyPgDown}, want: halfDownIntent},
		"ctrl+u pages half up":              {msg: tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}, want: halfUpIntent},
		"pgup pages half up":                {msg: tea.KeyPressMsg{Code: tea.KeyPgUp}, want: halfUpIntent},
		"g goes to the top":                 {msg: tea.KeyPressMsg{Code: 'g', Text: "g"}, want: topIntent},
		"home goes to the top":              {msg: tea.KeyPressMsg{Code: tea.KeyHome}, want: topIntent},
		"G goes to the bottom":              {msg: tea.KeyPressMsg{Code: 'G', Text: "G"}, want: bottomIntent},
		"end goes to the bottom":            {msg: tea.KeyPressMsg{Code: tea.KeyEnd}, want: bottomIntent},
		"] jumps to the next hunk":          {msg: tea.KeyPressMsg{Code: ']', Text: "]"}, want: nextHunkIntent},
		"[ jumps to the previous hunk":      {msg: tea.KeyPressMsg{Code: '[', Text: "["}, want: prevHunkIntent},
		"J moves to the next file":          {msg: tea.KeyPressMsg{Code: 'J', Text: "J"}, want: nextFileIntent},
		"K moves to the previous file":      {msg: tea.KeyPressMsg{Code: 'K', Text: "K"}, want: prevFileIntent},
		"@ loads the working copy":          {msg: tea.KeyPressMsg{Code: '@', Text: "@"}, want: workingCopyIntent},
		"R reloads the change":              {msg: tea.KeyPressMsg{Code: 'R', Text: "R"}, want: reloadIntent},
		"f toggles the full file":           {msg: tea.KeyPressMsg{Code: 'f', Text: "f"}, want: fullIntent},
		"c opens the commit dialog":         {msg: tea.KeyPressMsg{Code: 'c', Text: "c"}, want: commitIntent},
		"e opens the editor":                {msg: tea.KeyPressMsg{Code: 'e', Text: "e"}, want: editIntent},
		"z folds the hunk":                  {msg: tea.KeyPressMsg{Code: 'z', Text: "z"}, want: foldIntent},
		"enter folds the hunk":              {msg: tea.KeyPressMsg{Code: tea.KeyEnter}, want: foldIntent},
		"d shows removed lines of the hunk": {msg: tea.KeyPressMsg{Code: 'd', Text: "d"}, want: removedIntent},
		"D shows removed lines of all":      {msg: tea.KeyPressMsg{Code: 'D', Text: "D"}, want: removedAllIntent},
		"Z folds all hunks":                 {msg: tea.KeyPressMsg{Code: 'Z', Text: "Z"}, want: foldAllIntent},
		"s selects the line":                {msg: tea.KeyPressMsg{Code: 's', Text: "s"}, want: selectLineIntent},
		"v starts a range":                  {msg: tea.KeyPressMsg{Code: 'v', Text: "v"}, want: selectRangeIntent},
		"esc cancels a range":               {msg: tea.KeyPressMsg{Code: tea.KeyEscape}, want: cancelRangeIntent},
		"space selects the hunk":            {msg: tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}, want: selectHunkIntent},
		"S selects the file":                {msg: tea.KeyPressMsg{Code: 'S', Text: "S"}, want: selectFileIntent},
		"h scrolls the diff left":           {msg: tea.KeyPressMsg{Code: 'h', Text: "h"}, want: scrollLeftIntent},
		"l scrolls the diff right":          {msg: tea.KeyPressMsg{Code: 'l', Text: "l"}, want: scrollRightIntent},
		"unbound keys do nothing":           {msg: tea.KeyPressMsg{Code: 'x', Text: "x"}, want: noIntent},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := newKeyMap().resolve(tt.msg)

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
