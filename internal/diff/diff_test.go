package diff_test

import (
	"testing"

	"github.com/madalinpopa/jjreview/internal/diff"
)

func TestFilePath_success(t *testing.T) {
	tests := map[string]struct {
		file diff.File
		want string
	}{
		"modified uses new path": {file: diff.File{OldPath: "a.go", NewPath: "a.go", Status: diff.StatusModified}, want: "a.go"},
		"renamed uses new path":  {file: diff.File{OldPath: "old.go", NewPath: "new.go", Status: diff.StatusRenamed}, want: "new.go"},
		"added uses new path":    {file: diff.File{NewPath: "b.go", Status: diff.StatusAdded}, want: "b.go"},
		"deleted uses old path":  {file: diff.File{OldPath: "c.go", Status: diff.StatusDeleted}, want: "c.go"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			got := tt.file.Path()

			if got != tt.want {
				t.Errorf("Path() = %q, want %q", got, tt.want)
			}
		})
	}
}
