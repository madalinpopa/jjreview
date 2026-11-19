package diff

import (
	"fmt"
	"strconv"
	"strings"
)

const devNull = "/dev/null"

func Format(files []File) string {
	var b strings.Builder
	for _, f := range files {
		formatFile(&b, f)
	}
	return b.String()
}

func formatFile(b *strings.Builder, f File) {
	oldPath, newPath := f.OldPath, f.NewPath
	switch f.Status {
	case StatusAdded:
		oldPath = newPath
	case StatusDeleted:
		newPath = oldPath
	}
	fmt.Fprintf(b, "diff --git %s %s\n", quotePath("a/"+oldPath), quotePath("b/"+newPath))
	formatExtendedHeader(b, f)
	oldSide, newSide := sidePath("a/", f.OldPath), sidePath("b/", f.NewPath)
	if f.Binary {
		fmt.Fprintf(b, "Binary files %s and %s differ\n", oldSide, newSide)
		return
	}
	if len(f.Hunks) > 0 || (f.Status == StatusModified && f.OldPath != f.NewPath) {
		fmt.Fprintf(b, "--- %s\n+++ %s\n", oldSide, newSide)
	}
	for _, h := range f.Hunks {
		formatHunk(b, h)
	}
}

func formatExtendedHeader(b *strings.Builder, f File) {
	switch f.Status {
	case StatusAdded:
		fmt.Fprintf(b, "new file mode %s\n", modeOrDefault(f.NewMode))
		return
	case StatusDeleted:
		fmt.Fprintf(b, "deleted file mode %s\n", modeOrDefault(f.OldMode))
		return
	case StatusRenamed:
		fmt.Fprintf(b, "rename from %s\nrename to %s\n", quotePath(f.OldPath), quotePath(f.NewPath))
	case StatusCopied:
		fmt.Fprintf(b, "copy from %s\ncopy to %s\n", quotePath(f.OldPath), quotePath(f.NewPath))
	}
	formatModes(b, f.OldMode, f.NewMode)
}

func formatModes(b *strings.Builder, oldMode, newMode string) {
	if oldMode == newMode {
		if oldMode != "" {
			fmt.Fprintf(b, "index 0000000..0000000 %s\n", oldMode)
		}
		return
	}
	if oldMode != "" {
		fmt.Fprintf(b, "old mode %s\n", oldMode)
	}
	if newMode != "" {
		fmt.Fprintf(b, "new mode %s\n", newMode)
	}
}

func modeOrDefault(mode string) string {
	if mode == "" {
		return "100644"
	}
	return mode
}

func formatHunk(b *strings.Builder, h Hunk) {
	fmt.Fprintf(b, "@@ -%d,%d +%d,%d @@", h.OldStart, h.OldLines, h.NewStart, h.NewLines)
	if h.Section != "" {
		b.WriteString(" " + h.Section)
	}
	b.WriteByte('\n')
	for _, l := range h.Lines {
		b.WriteByte(linePrefix(l.Kind))
		b.WriteString(l.Text)
		b.WriteByte('\n')
		if l.NoNewline {
			b.WriteString("\\ No newline at end of file\n")
		}
	}
}

func linePrefix(kind LineKind) byte {
	switch kind {
	case LineAdded:
		return '+'
	case LineRemoved:
		return '-'
	default:
		return ' '
	}
}

func sidePath(prefix, path string) string {
	if path == "" {
		return devNull
	}
	return quotePath(prefix + path)
}

func quotePath(path string) string {
	quoted := strconv.Quote(path)
	if quoted[1:len(quoted)-1] == path {
		return path
	}
	return quoted
}
