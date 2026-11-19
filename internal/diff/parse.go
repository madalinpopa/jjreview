package diff

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

var ErrNotImplemented = errors.New("not implemented")

type phase int

const (
	phaseNone phase = iota
	phaseHeader
	phaseOldPath
	phaseHunks
	phaseBody
	phaseBinary
)

type parser struct {
	files      []File
	phase      phase
	headerLine int
	oldLeft    int
	newLeft    int
	oldNo      int
	newNo      int
	lastBody   bool
}

func Parse(r io.Reader) ([]File, error) {
	var p parser
	br := bufio.NewReader(r)
	n := 0
	for {
		s, err := br.ReadString('\n')
		if s != "" {
			n++
			if lineErr := p.feed(n, strings.TrimSuffix(s, "\n")); lineErr != nil {
				return nil, fmt.Errorf("parse diff: line %d: %w", n, lineErr)
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("parse diff: read line %d: %w", n+1, err)
		}
	}
	if err := p.finish(n + 1); err != nil {
		return nil, err
	}
	return p.files, nil
}

func (p *parser) file() *File {
	return &p.files[len(p.files)-1]
}

func (p *parser) hunk() *Hunk {
	f := p.file()
	return &f.Hunks[len(f.Hunks)-1]
}

func (p *parser) feed(n int, s string) error {
	switch p.phase {
	case phaseBody:
		return p.body(s)
	case phaseOldPath:
		return p.newPathLine(s)
	}
	if rest, ok := strings.CutPrefix(s, "diff --git "); ok {
		if err := p.finish(n); err != nil {
			return err
		}
		return p.start(n, rest)
	}
	switch p.phase {
	case phaseHeader:
		return p.header(s)
	case phaseHunks:
		return p.afterHunk(s)
	}
	return fmt.Errorf("unexpected line %q", s)
}

func (p *parser) finish(n int) error {
	switch p.phase {
	case phaseNone:
		return nil
	case phaseOldPath:
		return fmt.Errorf("parse diff: line %d: missing +++ line", n)
	case phaseBody:
		return fmt.Errorf("parse diff: line %d: unexpected end of hunk", n)
	}
	f := p.file()
	switch f.Status {
	case StatusAdded:
		f.OldPath, f.OldMode = "", ""
	case StatusDeleted:
		f.NewPath, f.NewMode = "", ""
	}
	if f.Path() == "" || (f.Status != StatusAdded && f.OldPath == "") {
		return fmt.Errorf("parse diff: line %d: cannot determine file paths", p.headerLine)
	}
	return nil
}

func (p *parser) start(n int, rest string) error {
	oldPath, newPath, err := splitHeaderPaths(rest)
	if err != nil {
		return err
	}
	p.files = append(p.files, File{OldPath: oldPath, NewPath: newPath})
	p.phase = phaseHeader
	p.headerLine = n
	p.lastBody = false
	return nil
}

func (p *parser) header(s string) error {
	f := p.file()
	var err error
	switch {
	case strings.HasPrefix(s, "--- "):
		f.OldPath, err = prefixedPath(s[len("--- "):], "a/")
		p.phase = phaseOldPath
	case strings.HasPrefix(s, "Binary files ") && strings.HasSuffix(s, " differ"):
		f.Binary = true
		p.phase = phaseBinary
	case strings.HasPrefix(s, "new file mode "):
		f.Status = StatusAdded
		f.NewMode, err = parseMode(s[len("new file mode "):])
	case strings.HasPrefix(s, "deleted file mode "):
		f.Status = StatusDeleted
		f.OldMode, err = parseMode(s[len("deleted file mode "):])
	case strings.HasPrefix(s, "old mode "):
		f.OldMode, err = parseMode(s[len("old mode "):])
	case strings.HasPrefix(s, "new mode "):
		f.NewMode, err = parseMode(s[len("new mode "):])
	case strings.HasPrefix(s, "index "):
		err = parseIndex(f, s[len("index "):])
	case strings.HasPrefix(s, "rename from "):
		f.Status = StatusRenamed
		f.OldPath, err = unquote(s[len("rename from "):])
	case strings.HasPrefix(s, "rename to "):
		f.NewPath, err = unquote(s[len("rename to "):])
	case strings.HasPrefix(s, "copy from "):
		f.Status = StatusCopied
		f.OldPath, err = unquote(s[len("copy from "):])
	case strings.HasPrefix(s, "copy to "):
		f.NewPath, err = unquote(s[len("copy to "):])
	case strings.HasPrefix(s, "similarity index "),
		strings.HasPrefix(s, "dissimilarity index "):
	default:
		return fmt.Errorf("unexpected header line %q", s)
	}
	return err
}

func parseMode(s string) (string, error) {
	if s == "" || strings.TrimLeft(s, "01234567") != "" {
		return "", fmt.Errorf("invalid file mode %q", s)
	}
	return s, nil
}

func parseIndex(f *File, s string) error {
	fields := strings.Split(s, " ")
	switch len(fields) {
	case 1:
		return nil
	case 2:
		mode, err := parseMode(fields[1])
		if err != nil {
			return err
		}
		f.OldMode, f.NewMode = mode, mode
		return nil
	}
	return fmt.Errorf("malformed index line %q", s)
}

func (p *parser) newPathLine(s string) error {
	rest, ok := strings.CutPrefix(s, "+++ ")
	if !ok {
		return fmt.Errorf("missing +++ line, got %q", s)
	}
	path, err := prefixedPath(rest, "b/")
	if err != nil {
		return err
	}
	p.file().NewPath = path
	p.phase = phaseHunks
	return nil
}

func (p *parser) afterHunk(s string) error {
	if strings.HasPrefix(s, "@@ ") {
		return p.startHunk(s)
	}
	if strings.HasPrefix(s, `\`) && p.lastBody {
		h := p.hunk()
		h.Lines[len(h.Lines)-1].NoNewline = true
		p.lastBody = false
		return nil
	}
	return fmt.Errorf("unexpected line %q", s)
}

func (p *parser) startHunk(s string) error {
	h, err := parseHunkHeader(s)
	if err != nil {
		return err
	}
	f := p.file()
	f.Hunks = append(f.Hunks, h)
	p.oldLeft, p.newLeft = h.OldLines, h.NewLines
	p.oldNo, p.newNo = h.OldStart, h.NewStart
	p.lastBody = false
	p.phase = phaseBody
	if p.oldLeft == 0 && p.newLeft == 0 {
		p.phase = phaseHunks
	}
	return nil
}

func (p *parser) body(s string) error {
	if s == "" {
		return errors.New("empty line inside hunk")
	}
	var line Line
	switch s[0] {
	case ' ':
		if p.oldLeft == 0 || p.newLeft == 0 {
			return errors.New("context line exceeds hunk line counts")
		}
		line = Line{Kind: LineContext, OldNo: p.oldNo, NewNo: p.newNo}
		p.oldNo, p.newNo, p.oldLeft, p.newLeft = p.oldNo+1, p.newNo+1, p.oldLeft-1, p.newLeft-1
	case '-':
		if p.oldLeft == 0 {
			return errors.New("removed line exceeds hunk old line count")
		}
		line = Line{Kind: LineRemoved, OldNo: p.oldNo}
		p.oldNo, p.oldLeft = p.oldNo+1, p.oldLeft-1
	case '+':
		if p.newLeft == 0 {
			return errors.New("added line exceeds hunk new line count")
		}
		line = Line{Kind: LineAdded, NewNo: p.newNo}
		p.newNo, p.newLeft = p.newNo+1, p.newLeft-1
	case '\\':
		return p.afterHunk(s)
	default:
		return fmt.Errorf("unexpected hunk line %q", s)
	}
	line.Text = s[1:]
	h := p.hunk()
	h.Lines = append(h.Lines, line)
	p.lastBody = true
	if p.oldLeft == 0 && p.newLeft == 0 {
		p.phase = phaseHunks
	}
	return nil
}

func parseHunkHeader(s string) (Hunk, error) {
	ranges, section, ok := strings.Cut(strings.TrimPrefix(s, "@@ "), " @@")
	if !ok {
		return Hunk{}, fmt.Errorf("malformed hunk header %q", s)
	}
	oldRange, newRange, ok := strings.Cut(ranges, " ")
	if !ok || !strings.HasPrefix(oldRange, "-") || !strings.HasPrefix(newRange, "+") {
		return Hunk{}, fmt.Errorf("malformed hunk header %q", s)
	}
	oldStart, oldLines, err := parseRange(oldRange[1:])
	if err != nil {
		return Hunk{}, fmt.Errorf("malformed hunk header %q: %w", s, err)
	}
	newStart, newLines, err := parseRange(newRange[1:])
	if err != nil {
		return Hunk{}, fmt.Errorf("malformed hunk header %q: %w", s, err)
	}
	return Hunk{
		OldStart: oldStart,
		OldLines: oldLines,
		NewStart: newStart,
		NewLines: newLines,
		Section:  strings.TrimPrefix(section, " "),
	}, nil
}

func parseRange(s string) (start, count int, err error) {
	startText, countText, hasCount := strings.Cut(s, ",")
	start, err = parseCount(startText)
	if err != nil {
		return 0, 0, err
	}
	if !hasCount {
		if start == 0 {
			return 0, 0, errors.New("range starts at line 0")
		}
		return start, 1, nil
	}
	count, err = parseCount(countText)
	if err != nil {
		return 0, 0, err
	}
	if start == 0 && count > 0 {
		return 0, 0, errors.New("range starts at line 0")
	}
	return start, count, nil
}

func parseCount(s string) (int, error) {
	if s == "" || strings.TrimLeft(s, "0123456789") != "" {
		return 0, fmt.Errorf("invalid number %q", s)
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid number %q", s)
	}
	return n, nil
}

func splitHeaderPaths(s string) (oldPath, newPath string, err error) {
	if strings.HasPrefix(s, `"`) {
		quoted, rest, err := cutQuoted(s)
		if err != nil {
			return "", "", err
		}
		rest, ok := strings.CutPrefix(rest, " ")
		if !ok {
			return "", "", fmt.Errorf("malformed diff header %q", s)
		}
		return headerPair(quoted, rest)
	}
	if i := strings.Index(s, ` "b/`); i >= 0 && strings.HasSuffix(s, `"`) {
		return headerPair(s[:i], s[i+1:])
	}
	return splitUnquoted(s)
}

func splitUnquoted(s string) (oldPath, newPath string, err error) {
	var splits []int
	for i := range len(s) {
		if strings.HasPrefix(s[i:], " b/") {
			splits = append(splits, i)
		}
	}
	if len(splits) == 0 {
		return "", "", fmt.Errorf("malformed diff header %q", s)
	}
	for _, i := range splits {
		if s[:i] == "a/"+s[i+len(" b/"):] {
			return headerPair(s[:i], s[i+1:])
		}
	}
	if len(splits) == 1 {
		return headerPair(s[:splits[0]], s[splits[0]+1:])
	}
	if !strings.HasPrefix(s, "a/") {
		return "", "", fmt.Errorf("malformed diff header %q", s)
	}
	return "", "", nil
}

func headerPair(oldToken, newToken string) (oldPath, newPath string, err error) {
	oldPath, err = prefixedPath(oldToken, "a/")
	if err != nil {
		return "", "", err
	}
	newPath, err = prefixedPath(newToken, "b/")
	if err != nil {
		return "", "", err
	}
	return oldPath, newPath, nil
}

func prefixedPath(field, prefix string) (string, error) {
	if field == devNull {
		return "", nil
	}
	path, err := unquote(field)
	if err != nil {
		return "", err
	}
	rest, ok := strings.CutPrefix(path, prefix)
	if !ok {
		return "", fmt.Errorf("path %q lacks %q prefix", field, prefix)
	}
	return rest, nil
}

func unquote(s string) (string, error) {
	if !strings.HasPrefix(s, `"`) {
		return s, nil
	}
	path, rest, err := cutQuoted(s)
	if err != nil {
		return "", err
	}
	if rest != "" {
		return "", fmt.Errorf("unexpected text after quoted path %q", s)
	}
	return path, nil
}

func cutQuoted(s string) (path, rest string, err error) {
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			path, err := strconv.Unquote(s[:i+1])
			if err != nil {
				return "", "", fmt.Errorf("invalid quoted path %q", s[:i+1])
			}
			return path, s[i+1:], nil
		}
	}
	return "", "", fmt.Errorf("unterminated quoted path %q", s)
}

func Full(f File, newText string) []Line {
	newLines := splitLines(newText)
	lastNoNewline := !strings.HasSuffix(newText, "\n")
	var out []Line
	oldNo, newNo := 1, 1
	appendContext := func(until int) {
		for ; newNo < until && newNo <= len(newLines); oldNo, newNo = oldNo+1, newNo+1 {
			out = append(out, Line{
				Kind:      LineContext,
				Text:      newLines[newNo-1],
				OldNo:     oldNo,
				NewNo:     newNo,
				NoNewline: lastNoNewline && newNo == len(newLines),
			})
		}
	}
	for _, h := range f.Hunks {
		appendContext(firstLine(h.NewStart, h.NewLines))
		oldNo, newNo = firstLine(h.OldStart, h.OldLines), firstLine(h.NewStart, h.NewLines)
		for _, l := range h.Lines {
			out = append(out, l)
			if l.Kind != LineAdded {
				oldNo = l.OldNo + 1
			}
			if l.Kind != LineRemoved {
				newNo = l.NewNo + 1
			}
		}
	}
	appendContext(len(newLines) + 1)
	return out
}

func firstLine(start, count int) int {
	if count == 0 {
		return start + 1
	}
	return start
}

func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}
