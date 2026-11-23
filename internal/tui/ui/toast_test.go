package ui_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/madalinpopa/jjreview/internal/theme"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

type timerCall struct {
	after time.Duration
	msg   tea.Msg
}

type fakeTimer struct {
	calls []timerCall
}

func (f *fakeTimer) after(d time.Duration, msg tea.Msg) tea.Cmd {
	f.calls = append(f.calls, timerCall{after: d, msg: msg})
	return func() tea.Msg { return msg }
}

func (f *fakeTimer) last(t *testing.T) timerCall {
	t.Helper()
	if len(f.calls) == 0 {
		t.Fatal("no timer was armed")
	}
	return f.calls[len(f.calls)-1]
}

func dismissHint() ui.KeyHint {
	return ui.KeyHint{Keys: []string{"x"}, Label: "dismiss"}
}

func toastView(t ui.Toast, width int) string {
	return ansi.Strip(t.View(theme.Default(), width, dismissHint()))
}

func TestToast_success(t *testing.T) {
	t.Parallel()

	t.Run("is hidden before the first start", func(t *testing.T) {
		t.Parallel()
		toast := ui.NewToast((&fakeTimer{}).after)

		if toast.Visible() || toastView(toast, 80) != "" {
			t.Errorf("Visible() = %v, View() = %q, want hidden", toast.Visible(), toastView(toast, 80))
		}
	})

	t.Run("shows the title, the command and a spinner while running", func(t *testing.T) {
		t.Parallel()
		timer := &fakeTimer{}
		toast := ui.NewToast(timer.after)

		_, cmd := toast.Start("Fetch", "jj git fetch")

		got := toastView(toast, 80)
		if !toast.Visible() || !strings.Contains(got, "⠋ Fetch") || !strings.Contains(got, "jj git fetch") {
			t.Errorf("View() = %q, want the first spinner frame, the title and the command", got)
		}
		if cmd == nil || timer.last(t).after != time.Second/12 {
			t.Errorf("Start() armed %+v, want a spinner tick", timer.calls)
		}
	})

	t.Run("advances the spinner on its tick and re-arms it", func(t *testing.T) {
		t.Parallel()
		timer := &fakeTimer{}
		toast := ui.NewToast(timer.after)
		toast.Start("Fetch", "jj git fetch")

		cmd, ok := toast.Update(timer.last(t).msg)

		if !ok || cmd == nil || !strings.Contains(toastView(toast, 80), "⠙ Fetch") {
			t.Errorf("Update(tick) = %v, %v, View() = %q, want the next frame and another tick", cmd != nil, ok, toastView(toast, 80))
		}
	})

	t.Run("expires a clean success", func(t *testing.T) {
		t.Parallel()
		timer := &fakeTimer{}
		toast := ui.NewToast(timer.after)
		id, _ := toast.Start("Fetch", "jj git fetch")

		cmd, ok := toast.Finish(id, ui.ToastResult{Text: "fetched"})

		got := toastView(toast, 80)
		if !ok || cmd == nil || timer.last(t).after != 3*time.Second || !strings.Contains(got, "✓ Fetch") || !strings.Contains(got, "fetched") {
			t.Fatalf("Finish() armed %+v, View() = %q, want a success with an expiry", timer.calls, got)
		}
		if strings.Contains(got, "dismiss") {
			t.Errorf("View() = %q, want no dismiss hint on a success that expires", got)
		}
		if _, ok := toast.Update(timer.last(t).msg); !ok || toast.Visible() {
			t.Errorf("Update(expiry) handled = %v, Visible() = %v, want hidden", ok, toast.Visible())
		}
	})

	t.Run("keeps a failure until dismissed", func(t *testing.T) {
		t.Parallel()
		timer := &fakeTimer{}
		toast := ui.NewToast(timer.after)
		id, _ := toast.Start("Push kxyzabcd", "jj git push --change kxyzabcd")

		cmd, ok := toast.Finish(id, ui.ToastResult{Err: errors.New("push kxyzabcd: rejected")})

		got := toastView(toast, 80)
		if !ok || cmd != nil || !strings.Contains(got, "✗ Push kxyzabcd") || !strings.Contains(got, "rejected") || !strings.Contains(got, "x dismiss") {
			t.Fatalf("Finish() cmd = %v, View() = %q, want a failure with a dismiss hint and no expiry", cmd != nil, got)
		}
		if !toast.Dismiss() || toast.Visible() {
			t.Errorf("Dismiss() left Visible() = %v, want hidden", toast.Visible())
		}
	})

	t.Run("keeps a success with a warning", func(t *testing.T) {
		t.Parallel()
		timer := &fakeTimer{}
		toast := ui.NewToast(timer.after)
		id, _ := toast.Start("Squash into bbbbbbbb", "jj squash --from aaaaaaaa --into bbbbbbbb")

		cmd, ok := toast.Finish(id, ui.ToastResult{Text: "squashed aaaaaaaa", Warning: "Warning: conflicts\nin two files"})

		got := toastView(toast, 80)
		if !ok || cmd != nil || !strings.Contains(got, "squashed aaaaaaaa") || !strings.Contains(got, "Warning: conflicts") || !strings.Contains(got, "in two files") {
			t.Errorf("Finish() cmd = %v, View() = %q, want every warning line and no expiry", cmd != nil, got)
		}
	})

	t.Run("accepts a finish before the first tick", func(t *testing.T) {
		t.Parallel()
		timer := &fakeTimer{}
		toast := ui.NewToast(timer.after)
		id, _ := toast.Start("Fetch", "jj git fetch")
		tick := timer.last(t).msg
		toast.Finish(id, ui.ToastResult{Err: errors.New("offline")})
		armed := len(timer.calls)

		cmd, ok := toast.Update(tick)

		if !ok || cmd != nil || len(timer.calls) != armed || !strings.Contains(toastView(toast, 80), "✗ Fetch") {
			t.Errorf("Update(tick) = %v, %v, want a handled tick that stops the spinner", cmd != nil, ok)
		}
	})

	t.Run("dismisses only a finished toast", func(t *testing.T) {
		t.Parallel()
		toast := ui.NewToast((&fakeTimer{}).after)
		toast.Start("Fetch", "jj git fetch")

		if toast.Dismiss() || !toast.Visible() {
			t.Errorf("Dismiss() of a running toast = true or hid it, want it kept")
		}
	})

	t.Run("summarises the toast for the status line", func(t *testing.T) {
		t.Parallel()
		tests := map[string]struct {
			result *ui.ToastResult
			want   string
		}{
			"running":             {want: "Fetch…"},
			"succeeded":           {result: &ui.ToastResult{Text: "fetched"}, want: "fetched"},
			"warned":              {result: &ui.ToastResult{Text: "fetched", Warning: "Warning: one\ntwo"}, want: "fetched · Warning: one"},
			"warned without text": {result: &ui.ToastResult{Warning: "Warning: one"}, want: "Warning: one"},
			"failed":              {result: &ui.ToastResult{Err: errors.New("fetch: offline\ndetails")}, want: "fetch: offline"},
		}
		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				toast := ui.NewToast((&fakeTimer{}).after)
				id, _ := toast.Start("Fetch", "jj git fetch")
				if tt.result != nil {
					toast.Finish(id, *tt.result)
				}

				got := toast.Summary()

				if got != tt.want {
					t.Errorf("Summary() = %q, want %q", got, tt.want)
				}
			})
		}
	})
}

func TestToastStale_success(t *testing.T) {
	t.Parallel()

	t.Run("ignores a finish for an older start", func(t *testing.T) {
		t.Parallel()
		toast := ui.NewToast((&fakeTimer{}).after)
		old, _ := toast.Start("Fetch", "jj git fetch")
		toast.Start("Push kxyzabcd", "jj git push --change kxyzabcd")

		cmd, ok := toast.Finish(old, ui.ToastResult{Text: "fetched"})

		if ok || cmd != nil || !strings.Contains(toastView(toast, 80), "Push kxyzabcd") || strings.Contains(toastView(toast, 80), "fetched") {
			t.Errorf("Finish(old) changed the toast: %q", toastView(toast, 80))
		}
	})

	t.Run("drops the tick and expiry of an older start", func(t *testing.T) {
		t.Parallel()
		timer := &fakeTimer{}
		toast := ui.NewToast(timer.after)
		id, _ := toast.Start("Fetch", "jj git fetch")
		tick := timer.calls[0].msg
		toast.Finish(id, ui.ToastResult{Text: "fetched"})
		expiry := timer.last(t).msg
		toast.Start("Push kxyzabcd", "jj git push --change kxyzabcd")
		armed := len(timer.calls)

		tickCmd, tickOK := toast.Update(tick)
		expiryCmd, expiryOK := toast.Update(expiry)

		if !tickOK || !expiryOK || tickCmd != nil || expiryCmd != nil || len(timer.calls) != armed {
			t.Errorf("stale Update() = (%v, %v), (%v, %v), want both handled without a command", tickCmd != nil, tickOK, expiryCmd != nil, expiryOK)
		}
		if !toast.Visible() || !strings.Contains(toastView(toast, 80), "⠋ Push") {
			t.Errorf("View() = %q, want the newer toast unchanged", toastView(toast, 80))
		}
	})

	t.Run("ignores a duplicate finish after a dismiss", func(t *testing.T) {
		t.Parallel()
		toast := ui.NewToast((&fakeTimer{}).after)
		id, _ := toast.Start("Fetch", "jj git fetch")
		toast.Finish(id, ui.ToastResult{Err: errors.New("offline")})
		toast.Dismiss()

		cmd, ok := toast.Finish(id, ui.ToastResult{Err: errors.New("offline")})

		if ok || cmd != nil || toast.Visible() {
			t.Errorf("Finish() after Dismiss() shows the toast again")
		}
	})

	t.Run("leaves other messages alone", func(t *testing.T) {
		t.Parallel()
		toast := ui.NewToast((&fakeTimer{}).after)

		if _, ok := toast.Update(tea.KeyPressMsg{Code: 'x'}); ok {
			t.Error("Update(key) handled = true, want false")
		}
	})
}

func TestToastView_success(t *testing.T) {
	t.Parallel()

	t.Run("is at most 48 columns wide", func(t *testing.T) {
		t.Parallel()
		toast := ui.NewToast((&fakeTimer{}).after)
		toast.Start("Fetch", "jj git fetch")

		if got := lipgloss.Width(toast.View(theme.Default(), 200, dismissHint())); got != 48 {
			t.Errorf("View() width = %d, want 48", got)
		}
		if got := lipgloss.Width(toast.View(theme.Default(), 30, dismissHint())); got != 28 {
			t.Errorf("View() width in 30 columns = %d, want 28", got)
		}
	})

	t.Run("renders nothing in a narrow terminal", func(t *testing.T) {
		t.Parallel()
		toast := ui.NewToast((&fakeTimer{}).after)
		toast.Start("Fetch", "jj git fetch")

		if got := toast.View(theme.Default(), 23, dismissHint()); got != "" {
			t.Errorf("View() at width 23 = %q, want empty", got)
		}
	})

	t.Run("truncates a long title and command", func(t *testing.T) {
		t.Parallel()
		toast := ui.NewToast((&fakeTimer{}).after)
		toast.Start("Push "+strings.Repeat("a", 80), "jj git push --bookmark "+strings.Repeat("b", 80))

		got := toastView(toast, 200)

		if lipgloss.Width(got) != 48 || strings.Count(got, "…") != 2 {
			t.Errorf("View() = %q, want both lines cut with an ellipsis inside 48 columns", got)
		}
	})

	t.Run("truncates wide characters by their width", func(t *testing.T) {
		t.Parallel()
		toast := ui.NewToast((&fakeTimer{}).after)
		toast.Start("Delete bookmark "+strings.Repeat("界", 40), "jj bookmark delete")

		got := toastView(toast, 200)

		for line := range strings.SplitSeq(got, "\n") {
			if w := lipgloss.Width(line); w != 48 {
				t.Errorf("View() line %q width = %d, want 48", line, w)
			}
		}
	})

	t.Run("wraps a long result into at most three lines", func(t *testing.T) {
		t.Parallel()
		toast := ui.NewToast((&fakeTimer{}).after)
		id, _ := toast.Start("Fetch", "jj git fetch")
		toast.Finish(id, ui.ToastResult{Err: errors.New(strings.Repeat("word ", 60))})

		got := toastView(toast, 200)

		if lines := strings.Count(got, "word"); lines == 0 || lipgloss.Height(got) != 8 || !strings.Contains(got, "…") {
			t.Errorf("View() = %q, want three result lines ending in an ellipsis", got)
		}
	})

	t.Run("shows only the first line of an error", func(t *testing.T) {
		t.Parallel()
		toast := ui.NewToast((&fakeTimer{}).after)
		id, _ := toast.Start("Fetch", "jj git fetch")
		toast.Finish(id, ui.ToastResult{Err: errors.New("fetch: offline\nHint: details")})

		got := toastView(toast, 80)

		if !strings.Contains(got, "fetch: offline") || strings.Contains(got, "Hint") {
			t.Errorf("View() = %q, want the first error line only", got)
		}
	})

	t.Run("escapes control characters", func(t *testing.T) {
		t.Parallel()
		toast := ui.NewToast((&fakeTimer{}).after)
		id, _ := toast.Start("Delete bookmark a\x1b[5mb", "jj bookmark delete a\tb")
		toast.Finish(id, ui.ToastResult{Err: errors.New("bad\x07name")})

		got := toast.View(theme.Default(), 80, dismissHint())

		if strings.Contains(got, "\x1b[5m") || strings.Contains(got, "\x07") || strings.Contains(got, "\t") {
			t.Errorf("View() = %q, want control characters escaped", got)
		}
		if plain := ansi.Strip(got); !strings.Contains(plain, `a\x1b[5mb`) || !strings.Contains(plain, `a\tb`) || !strings.Contains(plain, `bad\aname`) {
			t.Errorf("View() = %q, want the escaped text shown", plain)
		}
	})
}
