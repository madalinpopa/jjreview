package review

import (
	"context"
	"fmt"
	"maps"

	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/tui/ui"
)

type pick struct {
	file  int
	units []int
}

type commitJob struct {
	file  diff.File
	lines [][]bool
	whole bool
}

type commitRequest struct {
	changeID string
	headID   string
	message  string
	parentID string
	jobs     []commitJob
	cache    map[contentKey]string
}

type committedMsg struct {
	selected jj.Change
	rest     jj.Change
	fetched  map[contentKey]string
	err      error
}

func (m Model) picks() []pick {
	var out []pick
	for i, r := range m.reviews {
		if units := r.picked(); len(units) > 0 {
			out = append(out, pick{file: i, units: units})
		}
	}
	return out
}

func pickedUnits(picks []pick) int {
	n := 0
	for _, p := range picks {
		n += len(p.units)
	}
	return n
}

func (m Model) partialLines() (lines int, partial bool) {
	for i, r := range m.reviews {
		if wholeOnly(m.files[i]) {
			continue
		}
		n, part := r.pickedLines()
		lines += n
		partial = partial || part
	}
	return lines, partial
}

func (m Model) lineNote() string {
	lines, partial := m.partialLines()
	if !partial {
		return ""
	}
	return fmt.Sprintf(" (%d %s)", lines, ui.Plural(lines, "line"))
}

func (m Model) restricted(picks []pick) ([]diff.File, error) {
	out := make([]diff.File, len(picks))
	for i, p := range picks {
		f := m.files[p.file]
		if !wholeOnly(f) {
			done, taken := m.reviews[p.file].masks()
			step, err := diff.Between(f, done, taken)
			if err != nil {
				return nil, fmt.Errorf("patch for %s: %w", f.Path(), err)
			}
			f = step
		}
		out[i] = f
	}
	return out, nil
}

func (m Model) commitRequest(env Env, message string, picks []pick) commitRequest {
	req := commitRequest{
		changeID: env.Head.ChangeID,
		headID:   env.Head.CommitID,
		message:  message,
		parentID: env.Change.Parents[0],
		cache:    maps.Clone(m.contents),
	}
	for _, p := range picks {
		f := m.files[p.file]
		r := m.reviews[p.file]
		req.jobs = append(req.jobs, commitJob{
			file:  f,
			lines: r.included(),
			whole: wholeOnly(f) || r.complete(),
		})
	}
	return req
}

func runCommit(ctx context.Context, src Source, req commitRequest) tea.Cmd {
	return func() tea.Msg {
		fetched := map[contentKey]string{}
		get := func(key contentKey) (string, error) {
			if content, ok := req.cache[key]; ok {
				return content, nil
			}
			if content, ok := fetched[key]; ok {
				return content, nil
			}
			content, err := src.FileContent(ctx, key.rev, key.path)
			if err != nil {
				return "", fmt.Errorf("read %s at %s: %w", key.path, ui.ShortID(key.rev), err)
			}
			fetched[key] = content
			return content, nil
		}
		var files []jj.SplitFile
		for _, job := range req.jobs {
			split, err := splitFiles(job, get, req.parentID)
			if err != nil {
				return committedMsg{fetched: fetched, err: err}
			}
			files = append(files, split...)
		}
		selected, rest, err := src.Split(ctx, req.changeID, req.headID, req.message, files)
		if err != nil {
			err = fmt.Errorf("split %s: %w", ui.ShortID(req.changeID), err)
		}
		return committedMsg{selected: selected, rest: rest, fetched: fetched, err: err}
	}
}

func splitFiles(job commitJob, get func(contentKey) (string, error), parentID string) ([]jj.SplitFile, error) {
	f := job.file
	var out jj.SplitFile
	switch {
	case job.whole && f.Status == diff.StatusDeleted:
		out = jj.SplitFile{Path: f.OldPath, Delete: true}
	case job.whole && (wholeOnly(f) || f.Status == diff.StatusRenamed):
		out = jj.SplitFile{Path: f.NewPath, FromRight: true}
	default:
		content, err := applyHunks(job, get, parentID)
		if err != nil {
			return nil, err
		}
		out = jj.SplitFile{Path: f.Path(), Content: content, Mode: f.NewMode}
	}
	if f.Status == diff.StatusRenamed {
		return []jj.SplitFile{out, {Path: f.OldPath, Delete: true}}, nil
	}
	return []jj.SplitFile{out}, nil
}

func applyHunks(job commitJob, get func(contentKey) (string, error), parentID string) (string, error) {
	f := job.file
	parent := ""
	if f.Status != diff.StatusAdded {
		var err error
		parent, err = get(contentKey{rev: parentID, path: f.OldPath})
		if err != nil {
			return "", err
		}
	}
	content, err := diff.ApplyLines(parent, f, job.lines)
	if err != nil {
		return "", fmt.Errorf("apply hunks to %s: %w", f.Path(), err)
	}
	return content, nil
}
