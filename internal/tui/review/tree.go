package review

import (
	"strings"

	"github.com/madalinpopa/jjreview/internal/diff"
)

type treeRow struct {
	dir   bool
	depth int
	name  string
	count int
	file  int
}

func treeKey(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if i < len(parts)-1 {
			parts[i] = "0" + p
		} else {
			parts[i] = "1" + p
		}
	}
	return strings.Join(parts, "/")
}

func compareTree(a, b diff.File) int {
	return strings.Compare(treeKey(a.Path()), treeKey(b.Path()))
}

func (m Model) treeRows() []treeRow {
	var rows []treeRow
	var prev []string
	for i, f := range m.files {
		parts := strings.Split(f.Path(), "/")
		dirs := parts[:len(parts)-1]
		common := commonPrefix(prev, dirs)
		for d := common; d < len(dirs); d++ {
			prefix := strings.Join(dirs[:d+1], "/") + "/"
			rows = append(rows, treeRow{dir: true, depth: d, name: dirs[d] + "/", count: m.countUnder(prefix), file: -1})
		}
		rows = append(rows, treeRow{depth: len(dirs), name: parts[len(parts)-1], file: i})
		prev = dirs
	}
	return rows
}

func commonPrefix(a, b []string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}

func (m Model) countUnder(prefix string) int {
	n := 0
	for _, f := range m.files {
		if strings.HasPrefix(f.Path(), prefix) {
			n++
		}
	}
	return n
}

func (m Model) currentTreeRow() int {
	for i, r := range m.treeRows() {
		if r.file == m.fileIdx {
			return i
		}
	}
	return 0
}
