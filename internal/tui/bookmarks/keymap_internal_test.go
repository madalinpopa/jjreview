package bookmarks

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func text(k string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: rune(k[0]), Text: k}
}

func code(c rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: c}
}

func ctrl(c rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: c, Mod: tea.ModCtrl}
}

func held(msg tea.KeyPressMsg) tea.KeyPressMsg {
	msg.IsRepeat = true
	return msg
}

func TestResolve_success(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		msg  tea.KeyPressMsg
		want intent
	}{
		"j moves down":           {msg: text("j"), want: moveDownIntent},
		"down moves down":        {msg: code(tea.KeyDown), want: moveDownIntent},
		"k moves up":             {msg: text("k"), want: moveUpIntent},
		"up moves up":            {msg: code(tea.KeyUp), want: moveUpIntent},
		"ctrl+d pages down":      {msg: ctrl('d'), want: halfDownIntent},
		"pgdown pages down":      {msg: code(tea.KeyPgDown), want: halfDownIntent},
		"ctrl+u pages up":        {msg: ctrl('u'), want: halfUpIntent},
		"pgup pages up":          {msg: code(tea.KeyPgUp), want: halfUpIntent},
		"g goes to the first":    {msg: text("g"), want: firstIntent},
		"home goes to the first": {msg: code(tea.KeyHome), want: firstIntent},
		"G goes to the last":     {msg: text("G"), want: lastIntent},
		"end goes to the last":   {msg: code(tea.KeyEnd), want: lastIntent},
		"enter opens":            {msg: code(tea.KeyEnter), want: openIntent},
		"n creates":              {msg: text("n"), want: createIntent},
		"e edits":                {msg: text("e"), want: editIntent},
		"d deletes":              {msg: text("d"), want: deleteIntent},
		"R reloads":              {msg: text("R"), want: reloadIntent},
		"esc means nothing":      {msg: code(tea.KeyEscape), want: noIntent},
		"y means nothing":        {msg: text("y"), want: noIntent},
		"other keys do nothing":  {msg: text("x"), want: noIntent},
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

func TestResolveDialog_success(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		msg  tea.KeyPressMsg
		want intent
	}{
		"esc cancels":                {msg: code(tea.KeyEscape), want: cancelIntent},
		"tab switches field":         {msg: code(tea.KeyTab), want: nextFieldIntent},
		"shift+tab switches field":   {msg: tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}, want: nextFieldIntent},
		"up switches field":          {msg: code(tea.KeyUp), want: nextFieldIntent},
		"down switches field":        {msg: code(tea.KeyDown), want: nextFieldIntent},
		"enter saves":                {msg: code(tea.KeyEnter), want: saveIntent},
		"ctrl+enter saves":           {msg: ctrl(tea.KeyEnter), want: saveIntent},
		"ctrl+s saves":               {msg: ctrl('s'), want: saveIntent},
		"j is typed":                 {msg: text("j"), want: noIntent},
		"y is typed":                 {msg: text("y"), want: noIntent},
		"ctrl+d edits the text":      {msg: ctrl('d'), want: noIntent},
		"home moves the text cursor": {msg: code(tea.KeyHome), want: noIntent},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := newKeyMap().resolveDialog(tt.msg)

			if got != tt.want {
				t.Errorf("resolveDialog(%q) = %v, want %v", tt.msg.String(), got, tt.want)
			}
		})
	}
}

func TestResolveDelete_success(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		msg  tea.KeyPressMsg
		want intent
	}{
		"y confirms":         {msg: text("y"), want: confirmIntent},
		"n cancels":          {msg: text("n"), want: cancelIntent},
		"esc cancels":        {msg: code(tea.KeyEscape), want: cancelIntent},
		"enter cancels":      {msg: code(tea.KeyEnter), want: cancelIntent},
		"capital Y cancels":  {msg: text("Y"), want: cancelIntent},
		"d again cancels":    {msg: text("d"), want: cancelIntent},
		"down cancels":       {msg: code(tea.KeyDown), want: cancelIntent},
		"ctrl+s cancels too": {msg: ctrl('s'), want: cancelIntent},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := newKeyMap().resolveDelete(tt.msg)

			if got != tt.want {
				t.Errorf("resolveDelete(%q) = %v, want %v", tt.msg.String(), got, tt.want)
			}
		})
	}
}

func TestResolvePicker_success(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		msg  tea.KeyPressMsg
		want intent
	}{
		"esc cancels":               {msg: code(tea.KeyEscape), want: cancelIntent},
		"y confirms":                {msg: text("y"), want: confirmIntent},
		"held y does not confirm":   {msg: held(text("y")), want: noIntent},
		"j moves down":              {msg: text("j"), want: moveDownIntent},
		"down moves down":           {msg: code(tea.KeyDown), want: moveDownIntent},
		"k moves up":                {msg: text("k"), want: moveUpIntent},
		"up moves up":               {msg: code(tea.KeyUp), want: moveUpIntent},
		"g goes to the first":       {msg: text("g"), want: firstIntent},
		"home goes to the first":    {msg: code(tea.KeyHome), want: firstIntent},
		"G goes to the last":        {msg: text("G"), want: lastIntent},
		"end goes to the last":      {msg: code(tea.KeyEnd), want: lastIntent},
		"enter chooses":             {msg: code(tea.KeyEnter), want: openIntent},
		"held enter does not":       {msg: held(code(tea.KeyEnter)), want: noIntent},
		"ctrl+d does not page":      {msg: ctrl('d'), want: noIntent},
		"n does not create":         {msg: text("n"), want: noIntent},
		"d does not delete":         {msg: text("d"), want: noIntent},
		"R does not reload":         {msg: text("R"), want: noIntent},
		"other keys do nothing":     {msg: text("x"), want: noIntent},
		"tab does not switch field": {msg: code(tea.KeyTab), want: noIntent},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := newKeyMap().resolvePicker(tt.msg)

			if got != tt.want {
				t.Errorf("resolvePicker(%q) = %v, want %v", tt.msg.String(), got, tt.want)
			}
		})
	}
}
