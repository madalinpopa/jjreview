package operations

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
		"j moves down":           {msg: tea.KeyPressMsg{Code: 'j', Text: "j"}, want: moveDownIntent},
		"down moves down":        {msg: tea.KeyPressMsg{Code: tea.KeyDown}, want: moveDownIntent},
		"k moves up":             {msg: tea.KeyPressMsg{Code: 'k', Text: "k"}, want: moveUpIntent},
		"up moves up":            {msg: tea.KeyPressMsg{Code: tea.KeyUp}, want: moveUpIntent},
		"ctrl+d pages down":      {msg: tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}, want: halfPageDownIntent},
		"pgdown pages down":      {msg: tea.KeyPressMsg{Code: tea.KeyPgDown}, want: halfPageDownIntent},
		"ctrl+u pages up":        {msg: tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}, want: halfPageUpIntent},
		"pgup pages up":          {msg: tea.KeyPressMsg{Code: tea.KeyPgUp}, want: halfPageUpIntent},
		"g goes to the top":      {msg: tea.KeyPressMsg{Code: 'g', Text: "g"}, want: topIntent},
		"home goes to the top":   {msg: tea.KeyPressMsg{Code: tea.KeyHome}, want: topIntent},
		"G goes to the bottom":   {msg: tea.KeyPressMsg{Code: 'G', Text: "G"}, want: bottomIntent},
		"end goes to the bottom": {msg: tea.KeyPressMsg{Code: tea.KeyEnd}, want: bottomIntent},
		"l expands":              {msg: tea.KeyPressMsg{Code: 'l', Text: "l"}, want: expandIntent},
		"h collapses":            {msg: tea.KeyPressMsg{Code: 'h', Text: "h"}, want: collapseIntent},
		"r restores":             {msg: tea.KeyPressMsg{Code: 'r', Text: "r"}, want: restoreIntent},
		"u undoes":               {msg: tea.KeyPressMsg{Code: 'u', Text: "u"}, want: undoIntent},
		"a abandons":             {msg: tea.KeyPressMsg{Code: 'a', Text: "a"}, want: abandonIntent},
		"R reloads":              {msg: tea.KeyPressMsg{Code: 'R', Text: "R"}, want: reloadIntent},
		"y means nothing":        {msg: tea.KeyPressMsg{Code: 'y', Text: "y"}, want: noIntent},
		"x means nothing":        {msg: tea.KeyPressMsg{Code: 'x', Text: "x"}, want: noIntent},
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

func TestResolveConfirm_success(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		msg  tea.KeyPressMsg
		want intent
	}{
		"y confirms":             {msg: tea.KeyPressMsg{Code: 'y', Text: "y"}, want: confirmIntent},
		"a held y does nothing":  {msg: tea.KeyPressMsg{Code: 'y', Text: "y", IsRepeat: true}, want: noIntent},
		"n does nothing":         {msg: tea.KeyPressMsg{Code: 'n', Text: "n"}, want: noIntent},
		"r does nothing":         {msg: tea.KeyPressMsg{Code: 'r', Text: "r"}, want: noIntent},
		"enter does nothing":     {msg: tea.KeyPressMsg{Code: tea.KeyEnter}, want: noIntent},
		"j does not move either": {msg: tea.KeyPressMsg{Code: 'j', Text: "j"}, want: noIntent},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := newKeyMap().resolveConfirm(tt.msg)

			if got != tt.want {
				t.Errorf("resolveConfirm(%q) = %d, want %d", tt.msg.String(), got, tt.want)
			}
		})
	}
}
