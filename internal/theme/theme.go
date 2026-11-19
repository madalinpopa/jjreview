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

const maxANSI256 = 255

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

func New(colors, syntax map[string]string) (Theme, error) {
	t := Default()
	if err := apply(t.colorFields(), colors, ""); err != nil {
		return Theme{}, err
	}
	if err := apply(t.Syntax.fields(), syntax, "syntax."); err != nil {
		return Theme{}, err
	}
	return t, nil
}

func apply(fields map[string]*color.Color, values map[string]string, prefix string) error {
	for _, key := range slices.Sorted(maps.Keys(values)) {
		field, ok := fields[key]
		if !ok {
			return fmt.Errorf("theme: unknown colour %q", prefix+key)
		}
		c, err := ParseColor(values[key])
		if err != nil {
			return fmt.Errorf("theme: %s: %w", prefix+key, err)
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

func (t *Theme) colorFields() map[string]*color.Color {
	return map[string]*color.Color{
		"page": &t.Page, "text": &t.Text, "card": &t.Card, "border": &t.Border,
		"chip": &t.Chip, "chip_text": &t.ChipText, "title": &t.Title, "header": &t.Header,
		"strong": &t.Strong, "dir_name": &t.DirName, "muted": &t.Muted, "faint": &t.Faint,
		"dim": &t.Dim, "added": &t.Added, "added_bg": &t.AddedBg, "removed": &t.Removed,
		"removed_bg": &t.RemovedBg, "hunk": &t.Hunk, "hunk_bg": &t.HunkBg, "hunk_bar": &t.HunkBar,
		"folded_bar": &t.FoldedBar, "stats": &t.Stats, "selection": &t.Selection, "cursor_bg": &t.CursorBg,
		"key": &t.Key, "status": &t.Status, "picked": &t.Picked, "on_selection": &t.OnSelection,
		"badge_idle": &t.BadgeIdle, "badge_idle_fg": &t.BadgeIdleFg, "badge_selected": &t.BadgeSel,
		"modal_border": &t.ModalBorder, "input": &t.Input,
	}
}

func (s *Syntax) fields() map[string]*color.Color {
	return map[string]*color.Color{
		"type": &s.Type, "keyword": &s.Keyword, "function": &s.Function,
		"string": &s.String, "literal": &s.Literal, "comment": &s.Comment,
	}
}
