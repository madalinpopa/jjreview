package tui_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
)

func applied(t *testing.T, parent string, f diff.File, hunks ...int) string {
	t.Helper()
	got, err := diff.Apply(parent, f, hunks)
	if err != nil {
		t.Fatalf("diff.Apply(%v) error = %v", hunks, err)
	}
	return got
}

func TestCommitModal_success(t *testing.T) {
	t.Run("c without selection asks to select hunks", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "c")

		if m.ModalOpen() {
			t.Error("ModalOpen() = true, want false")
		}
		assertContains(t, view(m), "select hunks first (space / S)")
	})

	t.Run("c shows the modal while generating", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "tab", "j", "tab", "space")

		m, _ = update(t, m, keyMsg("c"))

		assertContains(t, view(m), "New commit", "from kxyzabcd", "src/main.go  @@ -3,2 +3,2 @@  +1 −1",
			"Commit message", "✨ generating…", "ctrl+r regenerate · esc cancel · ctrl+enter / ctrl+s commit")
	})

	t.Run("generated message fills the input from the selected hunks patch", func(t *testing.T) {
		gen := &fakeGenerator{text: "feat(main): use new line\n"}
		m := start(t, newSource(), gen, 120, 30)
		m = press(t, m, "tab", "j", "tab", "space")

		m = press(t, m, "c")

		if got := m.Message(); got != "feat(main): use new line" {
			t.Errorf("Message() = %q, want generated text", got)
		}
		assertContains(t, view(m), "✨ generated from 1 hunk, edit freely")
		f := mainFile()
		f.Hunks = f.Hunks[:1]
		if want := diff.Format([]diff.File{f}); !slices.Equal(gen.patches, []string{want}) {
			t.Errorf("patches = %q, want %q", gen.patches, want)
		}
	})

	t.Run("ctrl+r regenerates", func(t *testing.T) {
		gen := &fakeGenerator{text: "first"}
		m := start(t, newSource(), gen, 120, 30)
		m = press(t, m, "tab", "j", "tab", "space", "c")
		gen.text = "second"

		m = press(t, m, "ctrl+r")

		if got := m.Message(); got != "second" || len(gen.patches) != 2 {
			t.Errorf("Message() = %q after %d generations, want second after 2", got, len(gen.patches))
		}
	})

	t.Run("reopened modal ignores the closed modal's generation", func(t *testing.T) {
		gen := &fakeGenerator{}
		m := start(t, newSource(), gen, 120, 30)
		m = press(t, m, "tab", "j", "tab", "space")
		m, closed := update(t, m, keyMsg("c"))
		m = press(t, m, "esc")
		m, reopened := update(t, m, keyMsg("c"))

		gen.text = "stale"
		m, _ = update(t, m, closed())
		gen.text = "fresh"
		fresh := reopened()
		if err := gen.ctxs[1].Err(); err != nil {
			t.Errorf("reopened generation ctx.Err() = %v, want nil", err)
		}
		m, _ = update(t, m, fresh)

		if got := m.Message(); got != "fresh" {
			t.Errorf("Message() = %q, want fresh", got)
		}
	})

	t.Run("typing edits the message", func(t *testing.T) {
		m := start(t, newSource(), nil, 120, 30)
		m = press(t, m, "tab", "j", "tab", "space", "c")

		m = press(t, m, "f", "i", "x", "q")

		if got := m.Message(); got != "fixq" {
			t.Errorf("Message() = %q, want fixq", got)
		}
	})

	t.Run("without a generator shows the setup hint and starts empty", func(t *testing.T) {
		m := start(t, newSource(), nil, 160, 30)

		m = press(t, m, "tab", "j", "tab", "space", "c")

		if m.Message() != "" {
			t.Errorf("Message() = %q, want empty", m.Message())
		}
		got := view(m)
		assertContains(t, got, "set provider in config.toml to generate")
		assertMissing(t, got, "ANTHROPIC_API_KEY", "generating")
	})

	t.Run("without a generator the footer leaves out regenerate", func(t *testing.T) {
		m := start(t, newSource(), nil, 160, 30)

		m = press(t, m, "tab", "j", "tab", "space", "c")

		got := view(m)
		assertContains(t, got, "esc cancel · ctrl+enter / ctrl+s commit")
		assertMissing(t, got, "ctrl+r", "regenerate")
	})

	t.Run("pasting fills the message", func(t *testing.T) {
		m := start(t, newSource(), nil, 120, 30)
		m = press(t, m, "tab", "j", "tab", "space", "c")

		m, _ = update(t, m, tea.PasteMsg{Content: "feat: pasted"})

		if got := m.Message(); got != "feat: pasted" {
			t.Errorf("Message() = %q, want the pasted text", got)
		}
	})

	t.Run("pasting CRLF text keeps one line break per line", func(t *testing.T) {
		m := start(t, newSource(), nil, 120, 30)
		m = press(t, m, "tab", "j", "tab", "space", "c")

		m, _ = update(t, m, tea.PasteMsg{Content: "feat: pasted\r\n\r\nfirst\r\nsecond\rthird"})

		if got, want := m.Message(), "feat: pasted\n\nfirst\nsecond\nthird"; got != want {
			t.Errorf("Message() = %q, want %q", got, want)
		}
	})

	t.Run("ctrl+v keeps the selected text", func(t *testing.T) {
		m := start(t, newSource(), nil, 120, 30)
		m = press(t, m, "tab", "j", "tab", "space", "c", "a", "b", "c")
		m, _ = update(t, m, tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl})

		m, _ = update(t, m, tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})

		if got := m.Message(); got != "abc" {
			t.Errorf("Message() = %q, want the selected text kept", got)
		}
	})

	t.Run("a paste while generating does not change the message", func(t *testing.T) {
		m := start(t, newSource(), &fakeGenerator{text: "feat: generated"}, 120, 30)
		m = press(t, m, "tab", "j", "tab", "space")
		m, _ = update(t, m, keyMsg("c"))

		m, _ = update(t, m, tea.PasteMsg{Content: "pasted"})

		if got := m.Message(); got != "" {
			t.Errorf("Message() = %q, want the paste ignored while generating", got)
		}
	})

	t.Run("a paste while committing does not change the message", func(t *testing.T) {
		m := start(t, newSource(), nil, 120, 30)
		m = press(t, m, "tab", "j", "tab", "space", "c", "m", "s", "g")
		m, _ = update(t, m, keyMsg("ctrl+s"))

		m, _ = update(t, m, tea.PasteMsg{Content: "pasted"})

		if got := m.Message(); got != "msg" {
			t.Errorf("Message() = %q, want the paste ignored while committing", got)
		}
	})

	t.Run("esc closes the modal and keeps the selection", func(t *testing.T) {
		m := press(t, loaded(t, newSource()), "tab", "j", "tab", "space", "c", "esc")

		if m.ModalOpen() {
			t.Error("ModalOpen() = true, want false")
		}
		assertContains(t, view(m), "1 hunk selected")
	})
}

func TestCommitModal_failed(t *testing.T) {
	t.Run("generator error is shown", func(t *testing.T) {
		gen := &fakeGenerator{err: errors.New("quota exceeded")}
		m := start(t, newSource(), gen, 160, 30)

		m = press(t, m, "tab", "j", "tab", "space", "c")

		assertContains(t, view(m), "generation failed: quota exceeded")
	})

	t.Run("empty message is rejected", func(t *testing.T) {
		src := newSource()
		m := start(t, src, nil, 160, 30)

		m = press(t, m, "tab", "j", "tab", "space", "c", "ctrl+s")

		if !m.ModalOpen() || len(src.splits) != 0 {
			t.Errorf("ModalOpen() = %v with %d splits, want open modal and no split", m.ModalOpen(), len(src.splits))
		}
		assertContains(t, view(m), "commit message is empty")
	})

	t.Run("merge changes cannot be split", func(t *testing.T) {
		src := newSource()
		src.change.Parents = []string{"p1", "p2"}

		m := press(t, loaded(t, src), "tab", "j", "tab", "space", "c")

		if m.ModalOpen() {
			t.Error("ModalOpen() = true, want false")
		}
		assertContains(t, view(m), "2 parents")
	})

	t.Run("split error keeps the modal open", func(t *testing.T) {
		src := newSource()
		src.splitErr = errors.New("conflict")

		m := press(t, loaded(t, src), "tab", "j", "tab", "space", "c", "ctrl+s")

		if !m.ModalOpen() {
			t.Error("ModalOpen() = false, want true")
		}
		assertContains(t, view(m), "conflict")
	})
}

func TestCommit_success(t *testing.T) {
	t.Run("splits the selected hunk applied to the parent", func(t *testing.T) {
		src := newSource()
		m := press(t, loaded(t, src), "tab", "j", "tab", "space", "c", "ctrl+enter")

		want := []splitCall{{
			changeID: "kxyzabcdefgh",
			message:  "feat: generated",
			files:    []jj.SplitFile{{Path: "src/main.go", Content: applied(t, mainParent, mainFile(), 0), Mode: "100644"}},
		}}
		if !slices.EqualFunc(src.splits, want, equalSplit) {
			t.Errorf("splits = %+v, want %+v", src.splits, want)
		}
		if m.ModalOpen() {
			t.Error("ModalOpen() = true, want false")
		}
		got := view(m)
		assertContains(t, got, "✓ created kxyzabcd: feat: generated", "▸ ✓ @@ -3,2 +3,2 @@", "nothing selected", "@ rest1zzz")
		if c := m.Cursor(); c != 1 {
			t.Errorf("Cursor() = %d, want 1, on the folded hunk header", c)
		}
	})

	t.Run("folds the committed hunk and keeps the cursor on its line", func(t *testing.T) {
		src := newSource()
		m := press(t, loaded(t, src), "tab", "j", "tab", "space", "j", "j", "j", "j")
		before := m.Cursor()

		m = press(t, m, "c", "ctrl+s")

		if c := m.Cursor(); c != before-2 {
			t.Errorf("Cursor() = %d, want %d, still on the same line below the folded hunk", c, before-2)
		}
		assertContains(t, view(m), "▸ ✓ @@ -3,2 +3,2 @@")
	})

	t.Run("second commit targets the rest change and includes committed hunks", func(t *testing.T) {
		src := newSource()
		m := press(t, loaded(t, src), "tab", "j", "tab", "space", "c", "ctrl+s")

		m = press(t, m, "j", "j", "j", "space", "c", "ctrl+s")

		if len(src.splits) != 2 {
			t.Fatalf("splits = %d, want 2", len(src.splits))
		}
		second := src.splits[1]
		want := []jj.SplitFile{{Path: "src/main.go", Content: applied(t, mainParent, mainFile(), 0, 1), Mode: "100644"}}
		if second.changeID != "rest1zzzzzzzzzz" || !slices.Equal(second.files, want) {
			t.Errorf("second split = %+v, want rest1zzzzzzzzzz with %+v", second, want)
		}
		if n := strings.Count(strings.Join(src.requested, ","), "parent1:src/main.go"); n != 1 {
			t.Errorf("parent content requested %d times, want 1", n)
		}
		assertContains(t, view(m), "✓ main.go", "1/4 ✓", "@ rest2zzz")
	})

	t.Run("whole-file units keep right paths or delete left paths", func(t *testing.T) {
		src := newSource()
		src.files = []diff.File{
			imageFile(),
			{OldPath: "old.txt", NewPath: "new.txt", OldMode: "100644", NewMode: "100644", Status: diff.StatusRenamed},
			{OldPath: "gone.txt", Status: diff.StatusDeleted, Hunks: []diff.Hunk{{OldStart: 1, OldLines: 1, Lines: []diff.Line{{Kind: diff.LineRemoved, Text: "bye", OldNo: 1}}}}},
		}
		m := loaded(t, src)

		press(t, m, "S", "tab", "j", "tab", "S", "tab", "j", "tab", "S", "c", "ctrl+s")

		want := []jj.SplitFile{
			{Path: "gone.txt", Delete: true},
			{Path: "img.png", FromRight: true},
			{Path: "new.txt", FromRight: true},
			{Path: "old.txt", Delete: true},
		}
		if len(src.splits) != 1 || !slices.Equal(src.splits[0].files, want) {
			t.Errorf("splits = %+v, want files %+v", src.splits, want)
		}
	})
}

func TestCommitStale_failed(t *testing.T) {
	src := newSource()
	m := loaded(t, src)
	src.head = "commit2"

	m = press(t, m, "tab", "j", "tab", "space", "c", "ctrl+s")

	if !m.ModalOpen() {
		t.Error("ModalOpen() = false, want true")
	}
	if len(src.splits) != 0 {
		t.Errorf("splits = %+v, want none", src.splits)
	}
	got := view(m)
	assertContains(t, got, "revision changed since its diff was loaded", "jjr to review it again")
	for _, unwanted := range []string{"split done; restart jjr to continue", "▸ ✓ @@ -3,2 +3,2 @@"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("View() contains %q:\n%s", unwanted, got)
		}
	}
}

func TestCommitIncomplete_success(t *testing.T) {
	src := newSource()
	src.splitErr = fmt.Errorf("split: %w: reload failed", jj.ErrSplitIncomplete)
	m := press(t, loaded(t, src), "tab", "j", "tab", "space", "c", "ctrl+s")

	if m.ModalOpen() {
		t.Error("ModalOpen() = true, want false")
	}
	assertContains(t, view(m), "▸ ✓ @@ -3,2 +3,2 @@", "kxyzabcd", "split done; restart jjr to continue")

	src.splitErr = nil
	m = press(t, m, "j", "j", "j", "space", "c")

	if m.ModalOpen() || len(src.splits) != 0 {
		t.Errorf("ModalOpen() = %v with %d splits, want commits blocked", m.ModalOpen(), len(src.splits))
	}
	assertContains(t, view(m), "split done; restart jjr to continue")
}

func TestCommitModes_success(t *testing.T) {
	t.Run("mode-only change is a whole-file unit taken from the right", func(t *testing.T) {
		src := newSource()
		src.files = []diff.File{{OldPath: "run.sh", NewPath: "run.sh", OldMode: "100644", NewMode: "100755"}}
		m := loaded(t, src)

		assertContains(t, view(m), "mode 100644 → 100755")

		m = press(t, m, "S")
		assertContains(t, view(m), "● run.sh", "1 hunk selected")
		press(t, m, "c", "ctrl+s")

		want := []jj.SplitFile{{Path: "run.sh", FromRight: true}}
		if len(src.splits) != 1 || !slices.Equal(src.splits[0].files, want) {
			t.Errorf("splits = %+v, want files %+v", src.splits, want)
		}
	})

	t.Run("symlink is a whole-file unit", func(t *testing.T) {
		src := newSource()
		src.files = []diff.File{{OldPath: "link", NewPath: "link", OldMode: "120000", NewMode: "120000", Hunks: []diff.Hunk{
			{OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1, Lines: []diff.Line{
				{Kind: diff.LineRemoved, Text: "a", OldNo: 1},
				{Kind: diff.LineAdded, Text: "b", NewNo: 1},
			}},
		}}}
		m := loaded(t, src)

		m = press(t, m, "space")
		assertContains(t, view(m), "nothing selected")

		m = press(t, m, "S", "c", "ctrl+s")

		want := []jj.SplitFile{{Path: "link", FromRight: true}}
		if len(src.splits) != 1 || !slices.Equal(src.splits[0].files, want) {
			t.Errorf("splits = %+v, want files %+v", src.splits, want)
		}
		assertContains(t, view(m), "✓ link")

		got := view(press(t, m, "tab", "tab"))
		assertContains(t, got, "file 1/1 · 1 hunk")
		if strings.Contains(got, "folded") {
			t.Errorf("View() reports a folded hunk for a whole-only file:\n%s", got)
		}
	})
}

func TestGenerating_success(t *testing.T) {
	t.Run("typing is ignored while generating", func(t *testing.T) {
		gen := &fakeGenerator{text: "generated"}
		m := start(t, newSource(), gen, 120, 30)
		m = press(t, m, "tab", "j", "tab", "space")
		m, genCmd := update(t, m, keyMsg("c"))

		m, _ = update(t, m, keyMsg("x"))
		m = run(t, m, genCmd)

		if got := m.Message(); got != "generated" {
			t.Errorf("Message() = %q, want generated", got)
		}
	})

	t.Run("ctrl+r cancels the in-flight generation", func(t *testing.T) {
		gen := &fakeGenerator{text: "generated"}
		m := start(t, newSource(), gen, 120, 30)
		m = press(t, m, "tab", "j", "tab", "space")
		m, first := update(t, m, keyMsg("c"))
		first()

		press(t, m, "ctrl+r")

		if len(gen.ctxs) != 2 || gen.ctxs[0].Err() == nil {
			t.Errorf("first generation context err = %v after ctrl+r, want canceled", gen.ctxs[0].Err())
		}
	})
}

func TestModalErrors_success(t *testing.T) {
	t.Run("long generation errors wrap instead of being clipped", func(t *testing.T) {
		gen := &fakeGenerator{err: errors.New("the model endpoint refused the request because the account quota has been fully consumed for today tailword")}
		m := start(t, newSource(), gen, 80, 30)

		m = press(t, m, "tab", "j", "tab", "space", "c")

		assertContains(t, view(m), "tailword")
	})

	t.Run("long commit errors wrap instead of being clipped", func(t *testing.T) {
		src := newSource()
		src.splitErr = errors.New("jj split failed with a very long explanation of what went wrong in the working copy endword")
		m := start(t, src, &fakeGenerator{text: "msg"}, 80, 30)

		m = press(t, m, "tab", "j", "tab", "space", "c", "ctrl+s")

		assertContains(t, view(m), "endword")
	})
}

func TestModalErrors_footerVisible(t *testing.T) {
	diagnostic := "jj split: exit status 1: Error: " + strings.Repeat("diagnostic detail ", 16) + "tailword"
	tests := map[string]struct {
		err      error
		want     string
		wantGone string
	}{
		"stale revision shows a short recovery message": {
			err:      fmt.Errorf("split %q: %w: %w", "kxyzabcdefgh", jj.ErrStaleRevision, errors.New(diagnostic)),
			want:     "revision changed since its diff was loaded",
			wantGone: "tailword",
		},
		"a long error is cut to the room left": {
			err:  errors.New(strings.Repeat(diagnostic+" ", 3)),
			want: "diagnostic detail",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			src := newSource()
			src.splitErr = tt.err
			m := start(t, src, &fakeGenerator{text: "msg"}, 80, 22)

			m = press(t, m, "tab", "j", "tab", "space", "c", "ctrl+s")

			got := view(m)
			if !m.ModalOpen() {
				t.Fatal("ModalOpen() = false, want true")
			}
			assertContains(t, got, tt.want, "esc cancel", "ctrl+s commit")
			if tt.wantGone != "" {
				assertMissing(t, got, tt.wantGone)
			}
		})
	}
}

func TestModalLayout_success(t *testing.T) {
	stale := fmt.Errorf("split %q: %w: %w", "kxyzabcdefgh", jj.ErrStaleRevision, errors.New("jj split: exit status 1"))

	t.Run("a short terminal keeps the footer and the stale message with four previewed hunks", func(t *testing.T) {
		src := newSource()
		src.splitErr = stale
		m := start(t, src, &fakeGenerator{text: "msg"}, 80, 18)
		m = press(t, m, "tab", "space", "j", "space", "j", "space", "c")
		assertContains(t, view(m), "generated from 4 hunks")

		m = press(t, m, "ctrl+s")

		assertContains(t, view(m), "revision changed since its diff was loaded", "review it again", "esc cancel", "ctrl+s commit")
	})

	t.Run("a narrow terminal shows the commit shortcut", func(t *testing.T) {
		m := start(t, newSource(), &fakeGenerator{text: "msg"}, 40, 30)

		m = press(t, m, "tab", "j", "tab", "space", "c")

		assertContains(t, view(m), "ctrl+s commit", "esc cancel")
	})

	t.Run("long message lines in a small dialog keep the footer", func(t *testing.T) {
		line := strings.Repeat("x", 28) + "y"
		m := start(t, newSource(), &fakeGenerator{text: strings.Join([]string{line, line, line, line}, "\n")}, 40, 16)

		m = press(t, m, "tab", "j", "tab", "space", "c")

		assertContains(t, view(m), "ctrl+s commit", "esc")
	})

	t.Run("a very narrow terminal keeps the commit and cancel keys", func(t *testing.T) {
		m := start(t, newSource(), &fakeGenerator{text: "msg"}, 25, 30)

		m = press(t, m, "tab", "j", "tab", "space", "c")

		assertContains(t, view(m), "^s commit · esc")
	})

	t.Run("a very short terminal keeps the title, message and footer", func(t *testing.T) {
		m := start(t, newSource(), &fakeGenerator{text: "msg"}, 80, 10)

		m = press(t, m, "tab", "j", "tab", "space", "c")

		got := view(m)
		assertContains(t, got, "New commit", "msg", "esc cancel", "ctrl+s commit")
		lines := strings.Split(got, "\n")
		if last := lines[len(lines)-1]; !strings.Contains(last, "╰") || !strings.Contains(last, "╯") {
			t.Errorf("View() last line = %q, want the dialog's bottom border", last)
		}
	})

	t.Run("the minimum height keeps the footer", func(t *testing.T) {
		m := start(t, newSource(), &fakeGenerator{text: "msg"}, 80, 8)

		m = press(t, m, "tab", "j", "tab", "space", "c")

		assertContains(t, view(m), "New commit", "msg", "esc cancel", "ctrl+s commit")
	})

	t.Run("a very short terminal keeps the error and footer", func(t *testing.T) {
		src := newSource()
		src.splitErr = stale
		m := start(t, src, &fakeGenerator{text: "msg"}, 80, 10)
		m = press(t, m, "tab", "j", "tab", "space", "c")

		m = press(t, m, "ctrl+s")

		assertContains(t, view(m), "revision changed", "esc cancel", "ctrl+s commit")
	})

	t.Run("keeps the error and footer at every short height", func(t *testing.T) {
		for height := 8; height <= 14; height++ {
			t.Run(fmt.Sprint(height), func(t *testing.T) {
				src := newSource()
				src.splitErr = stale
				m := start(t, src, &fakeGenerator{text: "msg"}, 80, height)
				m = press(t, m, "tab", "j", "tab", "space", "c")

				m = press(t, m, "ctrl+s")

				assertContains(t, view(m), "New commit", "revision changed", "esc cancel", "ctrl+s commit")
			})
		}
	})

	t.Run("keeps the whole recovery instruction once two note rows fit", func(t *testing.T) {
		for height := 10; height <= 16; height++ {
			t.Run(fmt.Sprint(height), func(t *testing.T) {
				src := newSource()
				src.splitErr = stale
				m := start(t, src, &fakeGenerator{text: "msg"}, 80, height)
				m = press(t, m, "tab", "j", "tab", "space", "c")

				m = press(t, m, "ctrl+s")

				assertContains(t, view(m), "revision changed", "restart", "jjr to review it again", "esc cancel", "ctrl+s commit")
			})
		}
	})

	t.Run("a renamed whole file escapes its source path", func(t *testing.T) {
		src := newSource()
		src.files = []diff.File{
			{OldPath: "old\npart\nname.go", NewPath: "new.go", OldMode: "100644", NewMode: "100644", Status: diff.StatusRenamed},
		}
		m := start(t, src, &fakeGenerator{text: "msg"}, 80, 12)
		m = press(t, m, "S")

		m = press(t, m, "c")

		assertContains(t, view(m), `new.go  renamed from old\npart\nname.go`, "esc cancel", "ctrl+s commit")
	})

	t.Run("literal backslashes in paths are escaped", func(t *testing.T) {
		src := newSource()
		src.files = []diff.File{
			{OldPath: `old\n.go`, NewPath: `new\r.go`, OldMode: "100644", NewMode: "100644", Status: diff.StatusRenamed},
		}
		m := start(t, src, &fakeGenerator{text: "msg"}, 80, 12)
		m = press(t, m, "S")

		m = press(t, m, "c")

		assertContains(t, view(m), `new\\r.go  renamed from old\\n.go`)
	})

	t.Run("the minimum height shows the error in place of the message heading", func(t *testing.T) {
		m := start(t, newSource(), nil, 80, 8)
		m = press(t, m, "tab", "j", "tab", "space", "c")

		m = press(t, m, "ctrl+s")

		got := view(m)
		assertContains(t, got, "commit message is empty", "esc cancel", "ctrl+s commit")
		assertMissing(t, got, "Commit message")
	})

	t.Run("a filename with line breaks is escaped in the preview", func(t *testing.T) {
		src := newSource()
		src.files[1].OldPath, src.files[1].NewPath = "src/ma\nin\r.go", "src/ma\nin\r.go"
		src.contents["parent1:src/ma\nin\r.go"] = mainParent
		m := start(t, src, &fakeGenerator{text: "msg"}, 80, 14)
		m = press(t, m, "tab", "j", "tab", "space")

		m = press(t, m, "c")

		assertContains(t, view(m), `src/ma\nin\r.go`, "esc cancel", "ctrl+s commit")
	})

	t.Run("the stale message wins over a generation error", func(t *testing.T) {
		src := newSource()
		src.splitErr = stale
		gen := &fakeGenerator{err: errors.New(strings.Repeat("the model endpoint refused the request ", 6) + "tailword")}
		m := start(t, src, gen, 80, 18)
		m = press(t, m, "tab", "j", "tab", "space", "c", "x")

		m = press(t, m, "ctrl+s")

		got := view(m)
		assertContains(t, got, "revision changed since its diff was loaded", "review it again", "esc cancel", "ctrl+s commit")
		assertMissing(t, got, "tailword")
	})
}

func equalSplit(a, b splitCall) bool {
	return a.changeID == b.changeID && a.message == b.message && slices.Equal(a.files, b.files)
}

func TestImmutable_success(t *testing.T) {
	immutable := func() *fakeSource {
		src := newSource()
		src.change.Immutable = true
		return src
	}

	t.Run("marks the review read only", func(t *testing.T) {
		got := view(loaded(t, immutable()))

		assertContains(t, got, "read only")
		for _, hint := range []string{"select hunk", "select file", "commit selected"} {
			if strings.Contains(got, hint) {
				t.Errorf("View() of an immutable revision shows %q", hint)
			}
		}
	})

	t.Run("does not select hunks or files", func(t *testing.T) {
		for name, keys := range map[string][]string{
			"space in the diff pane": {"tab", "j", "tab", "space"},
			"S":                      {"tab", "j", "tab", "S"},
			"space in the file pane": {"tab", "j", "space"},
		} {
			t.Run(name, func(t *testing.T) {
				m := press(t, loaded(t, immutable()), keys...)

				assertContains(t, view(m), "nothing selected", "immutable revision: read only")
			})
		}
	})

	t.Run("does not open the commit dialog", func(t *testing.T) {
		src := immutable()

		m := press(t, loaded(t, src), "tab", "j", "tab", "c")

		if m.ModalOpen() {
			t.Error("ModalOpen() = true, want false")
		}
		assertContains(t, view(m), "immutable revision: read only")
	})
}
