package theme

import (
	"fmt"
	"image/color"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

const (
	maxANSI256  = 255
	cursorDark  = 0.08
	cursorLight = 0.06
	hunkLight   = 0.12
)

type Syntax struct {
	Type     color.Color
	Keyword  color.Color
	Function color.Color
	String   color.Color
	Literal  color.Color
	Comment  color.Color
}

type Theme struct {
	Page        color.Color
	Text        color.Color
	Card        color.Color
	Border      color.Color
	Chip        color.Color
	ChipText    color.Color
	Title       color.Color
	Header      color.Color
	Strong      color.Color
	DirName     color.Color
	Muted       color.Color
	Faint       color.Color
	Dim         color.Color
	Added       color.Color
	AddedBg     color.Color
	Removed     color.Color
	RemovedBg   color.Color
	Hunk        color.Color
	HunkBg      color.Color
	HunkBar     color.Color
	FoldedBar   color.Color
	Stats       color.Color
	Selection   color.Color
	CursorBg    color.Color
	Key         color.Color
	Status      color.Color
	Picked      color.Color
	OnSelection color.Color
	BadgeIdle   color.Color
	BadgeIdleFg color.Color
	BadgeSel    color.Color
	ModalBorder color.Color
	Input       color.Color
	Syntax      Syntax

	autoCursor bool
	autoHunk   bool
}

func Default() Theme {
	none := lipgloss.NoColor{}
	return Theme{
		Page:        none,
		Text:        none,
		Card:        none,
		Border:      lipgloss.BrightBlack,
		Chip:        none,
		ChipText:    lipgloss.Blue,
		Title:       none,
		Header:      lipgloss.BrightBlack,
		Strong:      none,
		DirName:     lipgloss.BrightBlack,
		Muted:       lipgloss.BrightBlack,
		Faint:       lipgloss.BrightBlack,
		Dim:         lipgloss.BrightBlack,
		Added:       lipgloss.Green,
		AddedBg:     none,
		Removed:     lipgloss.Red,
		RemovedBg:   none,
		Hunk:        lipgloss.Cyan,
		HunkBg:      lipgloss.BrightBlack,
		HunkBar:     lipgloss.Cyan,
		FoldedBar:   lipgloss.BrightBlack,
		Stats:       lipgloss.BrightBlack,
		Selection:   lipgloss.Blue,
		CursorBg:    lipgloss.BrightBlack,
		Key:         lipgloss.Yellow,
		Status:      lipgloss.Magenta,
		Picked:      lipgloss.Blue,
		OnSelection: lipgloss.Black,
		BadgeIdle:   none,
		BadgeIdleFg: lipgloss.BrightBlack,
		BadgeSel:    none,
		ModalBorder: lipgloss.BrightBlack,
		Input:       none,
		autoCursor:  true,
		autoHunk:    true,
		Syntax: Syntax{
			Type:     lipgloss.Cyan,
			Keyword:  lipgloss.Magenta,
			Function: lipgloss.Blue,
			String:   lipgloss.Green,
			Literal:  lipgloss.Yellow,
			Comment:  lipgloss.BrightBlack,
		},
	}
}

func New(sections map[string]map[string]string) (Theme, error) {
	t := Default()
	fields := t.fields()
	for _, name := range slices.Sorted(maps.Keys(sections)) {
		section, ok := fields[name]
		if !ok {
			return Theme{}, fmt.Errorf("unknown section theme.%s; use %s", name, Sections())
		}
		if err := apply(section, sections[name], "theme."+name+"."); err != nil {
			return Theme{}, err
		}
	}
	_, cursorSet := sections["base"]["cursor_bg"]
	_, hunkSet := sections["diff"]["hunk_bg"]
	pageUnset := t.Page == lipgloss.NoColor{}
	cardUnset := t.Card == lipgloss.NoColor{}
	t.autoCursor = !cursorSet && pageUnset
	t.autoHunk = !hunkSet && pageUnset && cardUnset
	return t, nil
}

func Moved(key string) (string, bool) {
	switch key {
	case "page", "text", "card", "border", "title", "header", "strong", "muted", "faint", "dim",
		"selection", "on_selection", "cursor_bg":
		return "base." + key, true
	case "added", "added_bg", "removed", "removed_bg", "hunk", "hunk_bg", "hunk_bar", "folded_bar", "stats":
		return "diff." + key, true
	case "dir_name", "picked":
		return "files." + key, true
	case "chip", "chip_text", "key", "status", "badge_idle", "badge_idle_fg", "badge_selected":
		return "bar." + key, true
	case "modal_border":
		return "modal.border", true
	case "input":
		return "modal.input", true
	}
	return "", false
}

func Sections() string {
	var t Theme
	names := slices.Sorted(maps.Keys(t.fields()))
	return strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1]
}

func (t Theme) WithBackground(bg color.Color, dark bool) Theme {
	if bg == nil {
		return t
	}
	if t.autoCursor {
		if dark {
			t.CursorBg = lipgloss.Lighten(bg, cursorDark)
		} else {
			t.CursorBg = lipgloss.Darken(bg, cursorLight)
		}
	}
	if t.autoHunk {
		if dark {
			t.HunkBg = Default().HunkBg
		} else {
			t.HunkBg = lipgloss.Darken(bg, hunkLight)
		}
	}
	return t
}

func apply(fields map[string]*color.Color, values map[string]string, prefix string) error {
	for _, key := range slices.Sorted(maps.Keys(values)) {
		field, ok := fields[key]
		if !ok {
			return fmt.Errorf("unknown colour %s", prefix+key)
		}
		c, err := ParseColor(values[key])
		if err != nil {
			return fmt.Errorf("%s: %w", prefix+key, err)
		}
		*field = c
	}
	return nil
}

func ParseColor(value string) (color.Color, error) {
	name := strings.ToLower(strings.TrimSpace(value))
	name = strings.NewReplacer("_", "-", " ", "-").Replace(name)
	if c, ok := named(name); ok {
		return c, nil
	}
	if n, err := strconv.Atoi(name); err == nil {
		if n < 0 || n > maxANSI256 {
			return nil, fmt.Errorf("colour %q is outside 0-%d", value, maxANSI256)
		}
		if basic := basicColors(); n < len(basic) {
			return basic[n], nil
		}
		return lipgloss.ANSIColor(n), nil
	}
	if regexp.MustCompile(`^#([0-9a-f]{3}|[0-9a-f]{6})$`).MatchString(name) {
		return lipgloss.Color(name), nil
	}
	return nil, fmt.Errorf("unknown colour %q; use none, an ANSI name, 0-%d or #rrggbb", value, maxANSI256)
}

func basicColors() []color.Color {
	return []color.Color{
		lipgloss.Black, lipgloss.Red, lipgloss.Green, lipgloss.Yellow,
		lipgloss.Blue, lipgloss.Magenta, lipgloss.Cyan, lipgloss.White,
		lipgloss.BrightBlack, lipgloss.BrightRed, lipgloss.BrightGreen, lipgloss.BrightYellow,
		lipgloss.BrightBlue, lipgloss.BrightMagenta, lipgloss.BrightCyan, lipgloss.BrightWhite,
	}
}

func named(name string) (color.Color, bool) {
	switch name {
	case "", "none", "default":
		return lipgloss.NoColor{}, true
	case "black":
		return lipgloss.Black, true
	case "red":
		return lipgloss.Red, true
	case "green":
		return lipgloss.Green, true
	case "yellow":
		return lipgloss.Yellow, true
	case "blue":
		return lipgloss.Blue, true
	case "magenta":
		return lipgloss.Magenta, true
	case "cyan":
		return lipgloss.Cyan, true
	case "white":
		return lipgloss.White, true
	case "bright-black", "gray", "grey":
		return lipgloss.BrightBlack, true
	case "bright-red":
		return lipgloss.BrightRed, true
	case "bright-green":
		return lipgloss.BrightGreen, true
	case "bright-yellow":
		return lipgloss.BrightYellow, true
	case "bright-blue":
		return lipgloss.BrightBlue, true
	case "bright-magenta":
		return lipgloss.BrightMagenta, true
	case "bright-cyan":
		return lipgloss.BrightCyan, true
	case "bright-white":
		return lipgloss.BrightWhite, true
	}
	return nil, false
}

func (t *Theme) fields() map[string]map[string]*color.Color {
	return map[string]map[string]*color.Color{
		"base": {
			"page": &t.Page, "text": &t.Text, "card": &t.Card, "border": &t.Border,
			"title": &t.Title, "header": &t.Header, "strong": &t.Strong, "muted": &t.Muted,
			"faint": &t.Faint, "dim": &t.Dim, "selection": &t.Selection,
			"on_selection": &t.OnSelection, "cursor_bg": &t.CursorBg,
		},
		"diff": {
			"added": &t.Added, "added_bg": &t.AddedBg, "removed": &t.Removed, "removed_bg": &t.RemovedBg,
			"hunk": &t.Hunk, "hunk_bg": &t.HunkBg, "hunk_bar": &t.HunkBar, "folded_bar": &t.FoldedBar,
			"stats": &t.Stats,
		},
		"files": {"dir_name": &t.DirName, "picked": &t.Picked},
		"bar": {
			"chip": &t.Chip, "chip_text": &t.ChipText, "key": &t.Key, "status": &t.Status,
			"badge_idle": &t.BadgeIdle, "badge_idle_fg": &t.BadgeIdleFg, "badge_selected": &t.BadgeSel,
		},
		"modal": {"border": &t.ModalBorder, "input": &t.Input},
		"syntax": {
			"type": &t.Syntax.Type, "keyword": &t.Syntax.Keyword, "function": &t.Syntax.Function,
			"string": &t.Syntax.String, "literal": &t.Syntax.Literal, "comment": &t.Syntax.Comment,
		},
	}
}
