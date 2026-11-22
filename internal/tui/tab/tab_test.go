package tab_test

import (
	"testing"

	"github.com/madalinpopa/jjreview/internal/tui/tab"
)

func TestDropPrompt_success(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		key  string
		n    int
		want string
	}{
		"one hunk":   {key: "y", n: 1, want: "press y again to drop 1 selected hunk"},
		"many hunks": {key: "enter", n: 3, want: "press enter again to drop 3 selected hunks"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := tab.DropPrompt(tt.key, tt.n)

			if got != tt.want {
				t.Errorf("DropPrompt(%q, %d) = %q, want %q", tt.key, tt.n, got, tt.want)
			}
		})
	}
}
