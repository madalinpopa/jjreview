package logview

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestResolve_success(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		msg  tea.KeyPressMsg
		want intent
	}{
		"j moves down":            {msg: tea.KeyPressMsg{Code: 'j', Text: "j"}, want: moveDownIntent},
		"down moves down":         {msg: tea.KeyPressMsg{Code: tea.KeyDown}, want: moveDownIntent},
		"k moves up":              {msg: tea.KeyPressMsg{Code: 'k', Text: "k"}, want: moveUpIntent},
		"up moves up":             {msg: tea.KeyPressMsg{Code: tea.KeyUp}, want: moveUpIntent},
		"ctrl+d pages down":       {msg: tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}, want: halfDownIntent},
		"pgdown pages down":       {msg: tea.KeyPressMsg{Code: tea.KeyPgDown}, want: halfDownIntent},
		"ctrl+u pages up":         {msg: tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}, want: halfUpIntent},
		"pgup pages up":           {msg: tea.KeyPressMsg{Code: tea.KeyPgUp}, want: halfUpIntent},
		"g goes to the top":       {msg: tea.KeyPressMsg{Code: 'g', Text: "g"}, want: topIntent},
		"home goes to the top":    {msg: tea.KeyPressMsg{Code: tea.KeyHome}, want: topIntent},
		"G goes to the bottom":    {msg: tea.KeyPressMsg{Code: 'G', Text: "G"}, want: bottomIntent},
		"end goes to the bottom":  {msg: tea.KeyPressMsg{Code: tea.KeyEnd}, want: bottomIntent},
		"enter opens":             {msg: tea.KeyPressMsg{Code: tea.KeyEnter}, want: openIntent},
		"slash searches":          {msg: tea.KeyPressMsg{Code: '/', Text: "/"}, want: searchIntent},
		"esc dismisses":           {msg: tea.KeyPressMsg{Code: tea.KeyEscape}, want: dismissIntent},
		"l expands":               {msg: tea.KeyPressMsg{Code: 'l', Text: "l"}, want: expandIntent},
		"h collapses":             {msg: tea.KeyPressMsg{Code: 'h', Text: "h"}, want: collapseIntent},
		"a abandons":              {msg: tea.KeyPressMsg{Code: 'a', Text: "a"}, want: abandonIntent},
		"y confirms":              {msg: tea.KeyPressMsg{Code: 'y', Text: "y"}, want: confirmIntent},
		"ctrl+r reloads":          {msg: tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}, want: reloadIntent},
		"R resets":                {msg: tea.KeyPressMsg{Code: 'R', Text: "R"}, want: resetIntent},
		"f fetches":               {msg: tea.KeyPressMsg{Code: 'f', Text: "f"}, want: fetchIntent},
		"P pushes":                {msg: tea.KeyPressMsg{Code: 'P', Text: "P"}, want: pushIntent},
		"b bookmarks":             {msg: tea.KeyPressMsg{Code: 'b', Text: "b"}, want: bookmarkIntent},
		"space marks":             {msg: tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}, want: markIntent},
		"s squashes":              {msg: tea.KeyPressMsg{Code: 's', Text: "s"}, want: squashIntent},
		"an unbound key is no-op": {msg: tea.KeyPressMsg{Code: 'x', Text: "x"}, want: noIntent},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := newKeyMap().resolve(tt.msg)

			if got != tt.want {
				t.Errorf("resolve(%q) = %d, want %d", tt.msg.String(), got, tt.want)
			}
		})
	}
}
