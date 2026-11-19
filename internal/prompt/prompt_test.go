package prompt_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/madalinpopa/jjreview/internal/prompt"
)

const samplePatch = "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-package old\n+package main\n"

func TestSystem_success(t *testing.T) {
	for _, want := range []string{"Conventional Commits", "Never credit an AI tool"} {
		if !strings.Contains(prompt.System, want) {
			t.Errorf("System does not mention %q", want)
		}
	}
}

func TestBuild_success(t *testing.T) {
	t.Run("wraps the patch in patch tags", func(t *testing.T) {
		got := prompt.Build(samplePatch)

		if !strings.Contains(got, "<patch>\n"+samplePatch+"</patch>") {
			t.Errorf("Build() = %q, want the patch inside <patch> tags", got)
		}
		if strings.Contains(got, "truncated") {
			t.Errorf("Build() mentions truncation for a small patch: %q", got)
		}
	})

	t.Run("ends the patch with a newline before the closing tag", func(t *testing.T) {
		got := prompt.Build("+line")

		if !strings.HasSuffix(got, "+line\n</patch>") {
			t.Errorf("Build() = %q, want a newline before </patch>", got)
		}
	})

	t.Run("truncates a large patch and says so", func(t *testing.T) {
		large := samplePatch + strings.Repeat("+line of data é\n", 30_000)

		got := prompt.Build(large)

		if len(got) >= len(large) {
			t.Errorf("Build() length = %d, want less than patch length %d", len(got), len(large))
		}
		if !strings.Contains(got, "truncated") {
			t.Errorf("Build() does not mention truncation")
		}
		if !strings.Contains(got, samplePatch) {
			t.Errorf("Build() does not keep the start of the patch")
		}
		if !utf8.ValidString(got) {
			t.Errorf("Build() returned invalid UTF-8")
		}
	})
}
