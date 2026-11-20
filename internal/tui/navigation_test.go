package tui_test

import (
	"testing"

	"github.com/madalinpopa/jjreview/internal/tui"
)

type position struct {
	file   string
	cursor int
	files  bool
}

func positionOf(m tui.Model) position {
	return position{file: m.CurrentFile(), cursor: m.Cursor(), files: m.FilesFocused()}
}

func jumpSource() *fakeSource {
	src := bigSource(100)
	other := viewFile()
	other.NewPath = "other.txt"
	src.files = append(src.files, other)
	return src
}

func TestJump_success(t *testing.T) {
	tests := map[string]struct {
		keys []string
		want position
	}{
		"G moves to the last diff row":           {keys: []string{"G"}, want: position{file: "big.txt", cursor: 100}},
		"g moves to the first change":            {keys: []string{"G", "g"}, want: position{file: "big.txt", cursor: 1}},
		"G in the file pane picks the last file": {keys: []string{"ctrl+h", "G"}, want: position{file: "other.txt", cursor: 1, files: true}},
		"g in the file pane picks the first":     {keys: []string{"ctrl+h", "G", "g"}, want: position{file: "big.txt", cursor: 1, files: true}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			m := press(t, start(t, jumpSource(), nil, 80, 24), tt.keys...)

			if got := positionOf(m); got != tt.want {
				t.Errorf("position = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestJump_modeChange(t *testing.T) {
	src := bigSource(10)
	src.files[0].OldMode, src.files[0].NewMode = "100644", "100755"
	m := press(t, start(t, src, nil, 80, 24), "G")
	first := press(t, start(t, src, nil, 80, 24))

	if got, want := press(t, m, "g").Cursor(), first.Cursor(); got != want {
		t.Errorf("Cursor() after g = %d, want %d (the first change, as after loading)", got, want)
	}
}

func TestKeyAlias_success(t *testing.T) {
	tests := map[string]struct {
		keys []string
		same []string
	}{
		"left is ^h":      {keys: []string{"left"}, same: []string{"ctrl+h"}},
		"right is ^l":     {keys: []string{"ctrl+h", "right"}, same: []string{"ctrl+h", "ctrl+l"}},
		"tab is ^h":       {keys: []string{"tab"}, same: []string{"ctrl+h"}},
		"tab twice is ^l": {keys: []string{"tab", "tab"}, same: []string{"ctrl+h", "ctrl+l"}},
		"pgdown is ^d":    {keys: []string{"pgdown"}, same: []string{"ctrl+d"}},
		"pgup is ^u":      {keys: []string{"pgdown", "pgdown", "pgup"}, same: []string{"ctrl+d", "ctrl+d", "ctrl+u"}},
		"home is g":       {keys: []string{"G", "home"}, same: []string{"G", "g"}},
		"end is G":        {keys: []string{"end"}, same: []string{"G"}},
		"end in files":    {keys: []string{"ctrl+h", "end"}, same: []string{"ctrl+h", "G"}},
		"pgdown in files": {keys: []string{"ctrl+h", "pgdown"}, same: []string{"ctrl+h", "ctrl+d"}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := positionOf(press(t, start(t, jumpSource(), nil, 80, 24), tt.keys...))
			want := positionOf(press(t, start(t, jumpSource(), nil, 80, 24), tt.same...))

			if got != want {
				t.Errorf("position after %v = %+v, want %+v as after %v", tt.keys, got, want, tt.same)
			}
		})
	}
}

func TestFileJump_success(t *testing.T) {
	tests := map[string]struct {
		keys []string
		want string
	}{
		"J opens the next file":       {keys: []string{"J"}, want: "src/main.go"},
		"K opens the previous file":   {keys: []string{"J", "J", "K"}, want: "src/main.go"},
		"K clamps at the first file":  {keys: []string{"K"}, want: "src/ui/view.go"},
		"J clamps at the last file":   {keys: []string{"J", "J", "J", "J", "J"}, want: "img.png"},
		"J works from the file pane":  {keys: []string{"ctrl+h", "J"}, want: "src/main.go"},
		"K returns to the first file": {keys: []string{"J", "J", "K", "K"}, want: "src/ui/view.go"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			m := press(t, loaded(t, newSource()), tt.keys...)

			if got := m.CurrentFile(); got != tt.want {
				t.Errorf("CurrentFile() = %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("J keeps the diff pane focused", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "J")

		if m.FilesFocused() {
			t.Error("FilesFocused() = true after J, want false")
		}
	})
}

func TestHunkJump_success(t *testing.T) {
	tests := map[string]struct {
		keys []string
		want int
	}{
		"] moves to the next hunk header":               {keys: []string{"]"}, want: 5},
		"] stays on the last hunk":                      {keys: []string{"]", "]"}, want: 5},
		"[ moves to the start of the current hunk":      {keys: []string{"["}, want: 1},
		"[ moves to the previous hunk":                  {keys: []string{"]", "["}, want: 1},
		"[ stays on the first hunk":                     {keys: []string{"[", "["}, want: 1},
		"] works with the file pane focused":            {keys: []string{"ctrl+h", "]"}, want: 5},
		"[ from inside the last hunk goes to its start": {keys: []string{"]", "j", "j", "["}, want: 5},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			m := press(t, loaded(t, newSource()), "ctrl+h", "j", "ctrl+l")

			m = press(t, m, tt.keys...)

			if got := m.Cursor(); got != tt.want {
				t.Errorf("Cursor() = %d, want %d", got, tt.want)
			}
		})
	}
}
