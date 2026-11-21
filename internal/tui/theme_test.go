package tui_test

import (
	"fmt"
	"image/color"
	"os/exec"
	"reflect"
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/llm"
	"github.com/madalinpopa/jjreview/internal/theme"
	"github.com/madalinpopa/jjreview/internal/tui"
)

const (
	lightKeyword = "91"
	darkKeyword  = "94"
	lightStrong  = "92"
	darkStrong   = "96"
)

func darkBg() tea.BackgroundColorMsg  { return tea.BackgroundColorMsg{Color: color.Black} }
func lightBg() tea.BackgroundColorMsg { return tea.BackgroundColorMsg{Color: color.White} }

func profiles(mode theme.Mode) theme.Set {
	light, dark := theme.Default(), theme.Default()
	light.Syntax.Keyword, light.Strong = lipgloss.BrightRed, lipgloss.BrightGreen
	dark.Syntax.Keyword, dark.Strong = lipgloss.BrightBlue, lipgloss.BrightCyan
	return theme.Set{Light: light, Dark: dark, Mode: mode}
}

func themed(t *testing.T, src tui.Source, gen llm.Provider, mode theme.Mode) tui.Model {
	t.Helper()
	m := tui.New(t.Context(), src, gen, "@").WithTheme(profiles(mode))
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	return run(t, m, m.Init())
}

func keywordSource() *fakeSource {
	return goSource(
		diff.Line{Kind: diff.LineContext, Text: "package main", OldNo: 1, NewNo: 1},
		diff.Line{Kind: diff.LineAdded, Text: `var s = "hello"`, NewNo: 2},
	)
}

func assertKeyword(t *testing.T, m tui.Model, want string) {
	t.Helper()
	if got := sgrBefore(t, m.View().Content, "package"); !hasSGR(got, want) {
		t.Errorf("SGR before package = %q, want %s", got, want)
	}
}

func messages(t *testing.T, cmd tea.Cmd) []tea.Msg {
	t.Helper()
	if cmd == nil {
		return nil
	}
	msg := cmd()
	v := reflect.ValueOf(msg)
	if v.Kind() != reflect.Slice || v.Type().Elem() != reflect.TypeFor[tea.Cmd]() {
		return []tea.Msg{msg}
	}
	var out []tea.Msg
	for i := range v.Len() {
		next, ok := reflect.TypeAssert[tea.Cmd](v.Index(i))
		if !ok {
			t.Fatalf("command %d has type %T, want tea.Cmd", i, v.Index(i).Interface())
		}
		out = append(out, messages(t, next)...)
	}
	return out
}

func isBackgroundRequest(msg tea.Msg) bool {
	return reflect.TypeOf(msg) == reflect.TypeOf(tea.RequestBackgroundColor())
}

func isExec(t *testing.T) func(tea.Msg) bool {
	t.Helper()
	want := reflect.TypeOf(tea.ExecProcess(exec.CommandContext(t.Context(), "true"), nil)())
	return func(msg tea.Msg) bool { return reflect.TypeOf(msg) == want }
}

func queryBackground(t *testing.T) tea.Msg {
	t.Helper()
	m := tui.New(t.Context(), newSource(), nil, "@")
	for _, msg := range messages(t, m.Init()) {
		if reflect.TypeOf(msg).Name() == "queryBackgroundMsg" {
			return msg
		}
	}
	t.Fatal("Init() did not schedule a background query")
	return nil
}

func initMessages(t *testing.T, m tui.Model) []tea.Msg {
	t.Helper()
	var out []tea.Msg
	for _, msg := range messages(t, m.Init()) {
		_, cmd := update(t, m, msg)
		out = append(out, msg)
		out = append(out, messages(t, cmd)...)
	}
	return out
}

func TestTheme_success(t *testing.T) {
	t.Run("auto renders the dark profile until the terminal answers", func(t *testing.T) {
		m := themed(t, keywordSource(), nil, theme.ModeAuto)

		assertKeyword(t, m, darkKeyword)
	})

	t.Run("auto switches from light to dark and back", func(t *testing.T) {
		m := themed(t, keywordSource(), nil, theme.ModeAuto)

		m, _ = update(t, m, lightBg())
		assertKeyword(t, m, lightKeyword)

		m, _ = update(t, m, darkBg())
		assertKeyword(t, m, darkKeyword)

		m, _ = update(t, m, lightBg())
		assertKeyword(t, m, lightKeyword)
	})

	t.Run("keeps the active profile when a later request gets no answer", func(t *testing.T) {
		m := themed(t, keywordSource(), nil, theme.ModeAuto)
		m, _ = update(t, m, lightBg())

		m, _ = update(t, m, tea.FocusMsg{})

		assertKeyword(t, m, lightKeyword)
	})

	t.Run("explicit modes apply their profile whatever the background", func(t *testing.T) {
		tests := map[string]struct {
			mode theme.Mode
			bg   tea.BackgroundColorMsg
			want string
		}{
			"light on a dark background": {mode: theme.ModeLight, bg: darkBg(), want: lightKeyword},
			"dark on a light background": {mode: theme.ModeDark, bg: lightBg(), want: darkKeyword},
		}

		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				m := themed(t, keywordSource(), nil, tt.mode)
				assertKeyword(t, m, tt.want)

				m, _ = update(t, m, tt.bg)

				assertKeyword(t, m, tt.want)
			})
		}
	})

	t.Run("auto applies colour scheme notifications before the terminal answers", func(t *testing.T) {
		tests := map[string]struct {
			before tea.Msg
			msg    tea.Msg
			want   string
		}{
			"light notification on the dark profile":       {msg: uv.LightColorSchemeEvent{}, want: lightKeyword},
			"dark notification after a light notification": {before: uv.LightColorSchemeEvent{}, msg: uv.DarkColorSchemeEvent{}, want: darkKeyword},
			"light notification after an unparsable reply": {before: tea.BackgroundColorMsg{}, msg: uv.LightColorSchemeEvent{}, want: lightKeyword},
		}

		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				m := themed(t, keywordSource(), nil, theme.ModeAuto)
				if tt.before != nil {
					m, _ = update(t, m, tt.before)
				}

				m, cmd := update(t, m, tt.msg)

				assertKeyword(t, m, tt.want)
				if got := messages(t, cmd); !slices.ContainsFunc(got, isBackgroundRequest) {
					t.Errorf("Update(%T) messages = %v, want a background colour request", tt.msg, got)
				}
			})
		}
	})

	t.Run("auto lets the background reply pick the profile once the terminal has answered", func(t *testing.T) {
		tests := map[string]struct {
			bg   tea.BackgroundColorMsg
			msg  tea.Msg
			want string
		}{
			"dark notification on a light background": {bg: lightBg(), msg: uv.DarkColorSchemeEvent{}, want: lightKeyword},
			"light notification on a dark background": {bg: darkBg(), msg: uv.LightColorSchemeEvent{}, want: darkKeyword},
		}

		for name, tt := range tests {
			t.Run(name, func(t *testing.T) {
				m := themed(t, keywordSource(), nil, theme.ModeAuto)
				m, _ = update(t, m, tt.bg)

				m, cmd := update(t, m, tt.msg)

				assertKeyword(t, m, tt.want)
				if got := messages(t, cmd); !slices.ContainsFunc(got, isBackgroundRequest) {
					t.Errorf("Update(%T) messages = %v, want a background colour request", tt.msg, got)
				}
			})
		}
	})

	t.Run("auto switches when the reply after a notification disagrees with the profile", func(t *testing.T) {
		m := themed(t, keywordSource(), nil, theme.ModeAuto)
		m, _ = update(t, m, darkBg())
		m, _ = update(t, m, uv.LightColorSchemeEvent{})

		m, _ = update(t, m, lightBg())

		assertKeyword(t, m, lightKeyword)
	})

	t.Run("auto ignores an unparsable background reply", func(t *testing.T) {
		m := themed(t, keywordSource(), nil, theme.ModeAuto)
		m, _ = update(t, m, lightBg())

		m, _ = update(t, m, tea.BackgroundColorMsg{})

		assertKeyword(t, m, lightKeyword)
	})

	t.Run("a new background of the same brightness reshades the view", func(t *testing.T) {
		m := themed(t, keywordSource(), nil, theme.ModeAuto)
		m, _ = update(t, m, tea.BackgroundColorMsg{Color: color.RGBA{R: 0x10, G: 0x10, B: 0x10, A: 0xff}})
		before := m.View().Content

		m, _ = update(t, m, tea.BackgroundColorMsg{Color: color.RGBA{R: 0x30, G: 0x30, B: 0x30, A: 0xff}})

		if m.View().Content == before {
			t.Error("View() after a new dark background is unchanged, want the cursor line reshaded")
		}
	})

	t.Run("auto recolours plain text when only the text colour differs between profiles", func(t *testing.T) {
		set := theme.Set{Light: theme.Default(), Dark: theme.Default(), Mode: theme.ModeAuto}
		set.Light.Text = lipgloss.Black
		m := tui.New(t.Context(), keywordSource(), nil, "@").WithTheme(set)
		m, _ = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
		m = run(t, m, m.Init())

		m, _ = update(t, m, lightBg())

		if got := sgrBefore(t, m.View().Content, "main"); !hasSGR(got, "30") {
			t.Errorf("SGR before main = %q, want 30", got)
		}
	})

	t.Run("explicit modes ignore colour scheme notifications", func(t *testing.T) {
		m := themed(t, keywordSource(), nil, theme.ModeLight)

		m, cmd := update(t, m, uv.DarkColorSchemeEvent{})

		assertKeyword(t, m, lightKeyword)
		if got := messages(t, cmd); slices.ContainsFunc(got, isBackgroundRequest) {
			t.Errorf("Update(DarkColorSchemeEvent) messages = %v, want no background colour request", got)
		}
	})

	t.Run("auto requests the background on every focus", func(t *testing.T) {
		tests := map[string][]tea.Msg{
			"before any notification":          nil,
			"after a dark notification":        {uv.DarkColorSchemeEvent{}},
			"after a light notification":       {uv.LightColorSchemeEvent{}},
			"after a notification and a reply": {uv.LightColorSchemeEvent{}, lightBg()},
		}

		for name, before := range tests {
			t.Run(name, func(t *testing.T) {
				m := themed(t, keywordSource(), nil, theme.ModeAuto)
				for _, msg := range before {
					m, _ = update(t, m, msg)
				}

				_, cmd := update(t, m, tea.FocusMsg{})

				if got := messages(t, cmd); !slices.ContainsFunc(got, isBackgroundRequest) {
					t.Errorf("Update(FocusMsg) messages = %v, want a background colour request", got)
				}
			})
		}
	})

	t.Run("explicit modes do not request the background on focus", func(t *testing.T) {
		for _, mode := range []theme.Mode{theme.ModeLight, theme.ModeDark} {
			t.Run(string(mode), func(t *testing.T) {
				m := themed(t, keywordSource(), nil, mode)

				_, cmd := update(t, m, tea.FocusMsg{})

				if got := messages(t, cmd); slices.ContainsFunc(got, isBackgroundRequest) {
					t.Errorf("Update(FocusMsg) messages = %v, want no background colour request", got)
				}
			})
		}
	})

	t.Run("commit modal keeps its message and takes the new colours", func(t *testing.T) {
		m := themed(t, newSource(), &fakeGenerator{text: "feat: keep me"}, theme.ModeAuto)
		m = press(t, m, "tab", "j", "tab", "space", "c")

		m, _ = update(t, m, lightBg())

		if got := m.Message(); got != "feat: keep me" {
			t.Errorf("Message() = %q, want %q", got, "feat: keep me")
		}
		if got := sgrBefore(t, m.View().Content, "feat: keep me"); !hasSGR(got, lightStrong) {
			t.Errorf("SGR before message = %q, want %s", got, lightStrong)
		}
	})
}

func TestThemeInit_success(t *testing.T) {
	tests := map[string]struct {
		mode theme.Mode
		want bool
	}{
		"auto enables colour scheme reports": {mode: theme.ModeAuto, want: true},
		"light leaves them off":              {mode: theme.ModeLight, want: false},
		"dark leaves them off":               {mode: theme.ModeDark, want: false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			m := tui.New(t.Context(), newSource(), nil, "@").WithTheme(profiles(tt.mode))

			got := initMessages(t, m)

			if !slices.ContainsFunc(got, isBackgroundRequest) {
				t.Errorf("Init() messages = %v, want a background colour request", got)
			}
			if enabled := slices.Contains(got, tea.Msg(tea.RawMsg{Msg: ansi.SetModeLightDark})); enabled != tt.want {
				t.Errorf("Init() enables mode 2031 = %t, want %t", enabled, tt.want)
			}
		})
	}
}

func TestThemeView_success(t *testing.T) {
	tests := map[string]struct {
		mode theme.Mode
		want bool
	}{
		"auto reports focus":        {mode: theme.ModeAuto, want: true},
		"light does not need focus": {mode: theme.ModeLight, want: false},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			m := tui.New(t.Context(), newSource(), nil, "@").WithTheme(profiles(tt.mode))

			got := m.View().ReportFocus

			if got != tt.want {
				t.Errorf("View().ReportFocus = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestThemeEditor_success(t *testing.T) {
	t.Run("auto resets colour scheme reports before the editor runs", func(t *testing.T) {
		var opened []string
		m := withEditor(answered(t, press(t, themed(t, newSource(), nil, theme.ModeAuto), "tab", "j", "tab")), &opened)

		_, cmd := edit(t, m)

		if top := cmd(); reflect.TypeOf(top) == reflect.TypeFor[tea.BatchMsg]() {
			t.Fatalf("Update(e) returned a batch, want an ordered sequence")
		}
		got := messages(t, cmd)
		reset := slices.Index(got, tea.Msg(tea.RawMsg{Msg: ansi.ResetModeLightDark}))
		run := slices.IndexFunc(got, isExec(t))
		if reset < 0 || run < 0 || reset > run {
			t.Errorf("Update(e) messages = %v, want the mode 2031 reset before the editor", got)
		}
	})

	t.Run("explicit modes run the editor without touching colour scheme reports", func(t *testing.T) {
		var opened []string
		m := withEditor(answered(t, press(t, themed(t, newSource(), nil, theme.ModeDark), "tab", "j", "tab")), &opened)

		_, cmd := edit(t, m)

		got := messages(t, cmd)
		if len(got) != 1 || !isExec(t)(got[0]) {
			t.Errorf("Update(e) messages = %v, want only the editor", got)
		}
	})

	t.Run("auto re-enables colour scheme reports and asks for the background after the editor", func(t *testing.T) {
		tests := map[string]tui.EditorFinishedMsg{
			"editor succeeded": {},
			"editor failed":    {Err: exec.ErrNotFound},
		}

		for name, msg := range tests {
			t.Run(name, func(t *testing.T) {
				m := press(t, themed(t, newSource(), nil, theme.ModeAuto), "tab", "j", "tab")

				_, cmd := update(t, m, msg)

				got := messages(t, cmd)
				if !slices.Contains(got, tea.Msg(tea.RawMsg{Msg: ansi.SetModeLightDark})) || !slices.ContainsFunc(got, isBackgroundRequest) {
					t.Errorf("Update(EditorFinishedMsg) messages = %v, want mode 2031 enabled and a background request", got)
				}
			})
		}
	})

	t.Run("explicit modes leave colour scheme reports off after the editor", func(t *testing.T) {
		m := press(t, themed(t, newSource(), nil, theme.ModeLight), "tab", "j", "tab")

		_, cmd := update(t, m, tui.EditorFinishedMsg{})

		if got := messages(t, cmd); slices.Contains(got, tea.Msg(tea.RawMsg{Msg: ansi.SetModeLightDark})) {
			t.Errorf("Update(EditorFinishedMsg) messages = %v, want mode 2031 left off", got)
		}
	})
}

func pendingEditor(t *testing.T, opened *[]string) tui.Model {
	t.Helper()
	return withEditor(press(t, themed(t, newSource(), nil, theme.ModeAuto), "tab", "j", "tab"), opened)
}

func TestThemeEditorHold_success(t *testing.T) {
	t.Parallel()

	t.Run("holds the editor while a background query is unanswered", func(t *testing.T) {
		t.Parallel()
		var opened []string
		m := pendingEditor(t, &opened)

		_, cmd := edit(t, m)

		if got := messages(t, cmd); slices.ContainsFunc(got, isExec(t)) {
			t.Errorf("Update(e) messages = %v, want the editor held", got)
		}
	})

	t.Run("runs the held editor when the reply arrives", func(t *testing.T) {
		t.Parallel()
		replies := map[string]tea.BackgroundColorMsg{
			"a colour":            darkBg(),
			"an unparsable reply": {},
		}

		for name, reply := range replies {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				var opened []string
				m, _ := update(t, pendingEditor(t, &opened), keyMsg("e"))
				m, check := update(t, m, reply)

				_, cmd := update(t, m, check())

				if got := messages(t, cmd); !slices.ContainsFunc(got, isExec(t)) {
					t.Errorf("Update(BackgroundColorMsg) messages = %v, want the editor", got)
				}
			})
		}
	})

	t.Run("runs the held editor when the reply times out", func(t *testing.T) {
		t.Parallel()
		var opened []string
		m, hold := edit(t, pendingEditor(t, &opened))

		var got []tea.Msg
		for _, msg := range messages(t, hold) {
			var cmd tea.Cmd
			m, cmd = update(t, m, msg)
			got = append(got, messages(t, cmd)...)
		}

		if !slices.ContainsFunc(got, isExec(t)) {
			t.Errorf("messages after the timeout = %v, want the editor", got)
		}
	})

	t.Run("does not run the held editor when the working copy moved during the hold", func(t *testing.T) {
		t.Parallel()
		var opened []string
		src := newSource()
		m := withEditor(press(t, themed(t, src, nil, theme.ModeAuto), "tab", "j", "tab"), &opened)
		m, _ = update(t, m, keyMsg("e"))
		src.revErrs = map[string]error{jj.WorkingCopyRevset("kxyzabcdefgh"): fmt.Errorf("jj log: %w", jj.ErrNoRevision)}
		m, check := update(t, m, darkBg())

		m, cmd := update(t, m, check())

		if got := messages(t, cmd); slices.ContainsFunc(got, isExec(t)) || len(opened) != 0 {
			t.Errorf("messages after the check = %v, opened = %v, want no editor", got, opened)
		}
		assertContains(t, view(m), "working copy moved; press @ to review it")
	})

	t.Run("checks the working copy again when a query starts during the check", func(t *testing.T) {
		t.Parallel()
		var opened []string
		src := newSource()
		m := withEditor(answered(t, press(t, themed(t, src, nil, theme.ModeAuto), "tab", "j", "tab")), &opened)
		m, check := update(t, m, keyMsg("e"))
		m, _ = update(t, m, tea.FocusMsg{})
		m, held := update(t, m, check())
		if got := messages(t, held); slices.ContainsFunc(got, isExec(t)) {
			t.Fatalf("messages after the check = %v, want the editor held", got)
		}
		src.revErrs = map[string]error{jj.WorkingCopyRevset("kxyzabcdefgh"): fmt.Errorf("jj log: %w", jj.ErrNoRevision)}
		m, recheck := update(t, m, darkBg())

		m, cmd := update(t, m, recheck())

		if got := messages(t, cmd); slices.ContainsFunc(got, isExec(t)) || len(opened) != 0 {
			t.Errorf("messages after the second check = %v, opened = %v, want no editor", got, opened)
		}
		assertContains(t, view(m), "working copy moved; press @ to review it")
	})

	t.Run("checks the working copy again when a query starts and is answered during the check", func(t *testing.T) {
		t.Parallel()
		var opened []string
		src := newSource()
		m := withEditor(answered(t, press(t, themed(t, src, nil, theme.ModeAuto), "tab", "j", "tab")), &opened)
		m, check := update(t, m, keyMsg("e"))
		m, _ = update(t, m, tea.FocusMsg{})
		m, _ = update(t, m, darkBg())
		m, recheck := update(t, m, check())
		if got := messages(t, recheck); slices.ContainsFunc(got, isExec(t)) {
			t.Fatalf("messages after the check = %v, want another check", got)
		}
		src.revErrs = map[string]error{jj.WorkingCopyRevset("kxyzabcdefgh"): fmt.Errorf("jj log: %w", jj.ErrNoRevision)}

		m, cmd := update(t, m, recheck())

		if got := messages(t, cmd); slices.ContainsFunc(got, isExec(t)) || len(opened) != 0 {
			t.Errorf("messages after the second check = %v, opened = %v, want no editor", got, opened)
		}
		assertContains(t, view(m), "working copy moved; press @ to review it")
	})

	t.Run("defers background queries once the editor is queued", func(t *testing.T) {
		t.Parallel()
		events := map[string]tea.Msg{
			"focus":            tea.FocusMsg{},
			"dark scheme":      uv.DarkColorSchemeEvent{},
			"light scheme":     uv.LightColorSchemeEvent{},
			"background query": queryBackground(t),
		}

		for name, event := range events {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				var opened []string
				m := withEditor(answered(t, press(t, themed(t, newSource(), nil, theme.ModeAuto), "tab", "j", "tab")), &opened)
				m, cmd := edit(t, m)
				if got := messages(t, cmd); !slices.ContainsFunc(got, isExec(t)) {
					t.Fatalf("Update(e) messages = %v, want the editor", got)
				}

				m, cmd = update(t, m, event)

				if got := messages(t, cmd); slices.ContainsFunc(got, isBackgroundRequest) {
					t.Errorf("Update(%T) messages = %v, want no background request before the editor runs", event, got)
				}

				_, cmd = update(t, m, tui.EditorFinishedMsg{})

				if got := messages(t, cmd); slices.IndexFunc(got, isBackgroundRequest) < 0 {
					t.Errorf("Update(EditorFinishedMsg) messages = %v, want a background request", got)
				}
			})
		}
	})

	t.Run("asks for the deferred background after the editor in explicit modes", func(t *testing.T) {
		t.Parallel()
		var opened []string
		m := withEditor(answered(t, press(t, themed(t, newSource(), nil, theme.ModeDark), "tab", "j", "tab")), &opened)
		m, _ = edit(t, m)
		m, cmd := update(t, m, queryBackground(t))
		if got := messages(t, cmd); slices.ContainsFunc(got, isBackgroundRequest) {
			t.Fatalf("Update(queryBackgroundMsg) messages = %v, want the query deferred", got)
		}

		_, cmd = update(t, m, tui.EditorFinishedMsg{})

		if got := messages(t, cmd); !slices.ContainsFunc(got, isBackgroundRequest) {
			t.Errorf("Update(EditorFinishedMsg) messages = %v, want the deferred background request", got)
		}
	})

	t.Run("a stale timeout does not release a later hold", func(t *testing.T) {
		t.Parallel()
		var opened []string
		m, stale := update(t, pendingEditor(t, &opened), keyMsg("e"))
		m, check := update(t, m, darkBg())
		m, _ = update(t, m, check())
		m, _ = update(t, m, tui.EditorFinishedMsg{Err: exec.ErrNotFound})
		m, _ = update(t, m, keyMsg("e"))

		var got []tea.Msg
		for _, msg := range messages(t, stale) {
			var released tea.Cmd
			m, released = update(t, m, msg)
			for _, msg := range messages(t, released) {
				var cmd tea.Cmd
				m, cmd = update(t, m, msg)
				got = append(got, messages(t, cmd)...)
			}
		}

		if slices.ContainsFunc(got, isExec(t)) {
			t.Errorf("messages after a stale timeout = %v, want the editor still held", got)
		}
	})
	t.Run("a timeout after the reply keeps later queries pending", func(t *testing.T) {
		t.Parallel()
		var opened []string
		m, stale := update(t, pendingEditor(t, &opened), keyMsg("e"))
		m, check := update(t, m, darkBg())
		m, _ = update(t, m, check())
		m, _ = update(t, m, tui.EditorFinishedMsg{Err: exec.ErrNotFound})
		for _, msg := range messages(t, stale) {
			m, _ = update(t, m, msg)
		}

		_, cmd := edit(t, m)

		if got := messages(t, cmd); slices.ContainsFunc(got, isExec(t)) {
			t.Errorf("Update(e) messages = %v, want the editor held", got)
		}
	})
}
