package tui

import (
	"context"
	"fmt"
	"maps"

	tea "charm.land/bubbletea/v2"
	"github.com/madalinpopa/jjreview/internal/diff"
	"github.com/madalinpopa/jjreview/internal/jj"
)

type pick struct {
	file  int
	units []int
}

type commitJob struct {
	file  diff.File
	hunks []int
	whole bool
}

type commitRequest struct {
	changeID string
	headID   string
	message  string
	commitID string
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

func (m Model) restricted(picks []pick) []diff.File {
	out := make([]diff.File, len(picks))
	for i, p := range picks {
		f := m.files[p.file]
		if !wholeOnly(f) {
			hunks := make([]diff.Hunk, len(p.units))
			for j, k := range p.units {
				hunks[j] = f.Hunks[k]
			}
			f.Hunks = hunks
		}
		out[i] = f
	}
	return out
}

func (m Model) commitRequest(message string, picks []pick) commitRequest {
	req := commitRequest{
		changeID: m.target,
		headID:   m.targetHead,
		message:  message,
		commitID: m.change.CommitID,
		parentID: m.change.Parents[0],
		cache:    maps.Clone(m.contents),
	}
	for _, p := range picks {
		f := m.files[p.file]
		included := m.reviews[p.file].included()
		req.jobs = append(req.jobs, commitJob{
			file:  f,
			hunks: included,
			whole: wholeOnly(f) || len(included) == len(f.Hunks),
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
				return "", fmt.Errorf("read %s at %s: %w", key.path, shortID(key.rev), err)
			}
			fetched[key] = content
			return content, nil
		}
		var files []jj.SplitFile
		for _, job := range req.jobs {
			split, err := splitFiles(job, get, req.commitID, req.parentID)
			if err != nil {
				return committedMsg{fetched: fetched, err: err}
			}
			files = append(files, split...)
		}
		selected, rest, err := src.Split(ctx, req.changeID, req.headID, req.message, files)
		if err != nil {
			err = fmt.Errorf("split %s: %w", shortID(req.changeID), err)
		}
		return committedMsg{selected: selected, rest: rest, fetched: fetched, err: err}
	}
}

func splitFiles(job commitJob, get func(contentKey) (string, error), commitID, parentID string) ([]jj.SplitFile, error) {
	f := job.file
	var out jj.SplitFile
	switch {
	case job.whole && f.Status == diff.StatusDeleted:
		out = jj.SplitFile{Path: f.OldPath, Delete: true}
	case job.whole && (wholeOnly(f) || f.Status == diff.StatusRenamed):
		content, err := get(contentKey{rev: commitID, path: f.NewPath})
		if err != nil {
			return nil, err
		}
		out = jj.SplitFile{Path: f.NewPath, Content: content, Mode: f.NewMode}
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
	content, err := diff.Apply(parent, f, job.hunks)
	if err != nil {
		return "", fmt.Errorf("apply hunks to %s: %w", f.Path(), err)
	}
	return content, nil
}
