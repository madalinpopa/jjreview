package jj

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"strings"
)

const logTemplate = `"\x1f{\"change_id\":" ++ json(change_id) ++ ",\"commit_id\":" ++ json(commit_id) ++ ",\"description\":" ++ json(description) ++ ",\"author\":" ++ json(author) ++ ",\"timestamp\":" ++ json(committer.timestamp().ago()) ++ ",\"bookmarks\":" ++ json(local_bookmarks.map(|b| b.name())) ++ ",\"workspaces\":" ++ json(working_copies.map(|w| w.name())) ++ ",\"immutable\":" ++ json(immutable) ++ ",\"working_copy\":" ++ json(current_working_copy) ++ "}\n"`

type LogEntry struct {
	ChangeID    string
	CommitID    string
	Description string
	Author      string
	Timestamp   string
	Bookmarks   []string
	Workspaces  []string
	Immutable   bool
	WorkingCopy bool
}

type LogLine struct {
	Graph string
	Entry *LogEntry
}

type logEntryJSON struct {
	ChangeID    string `json:"change_id"`
	CommitID    string `json:"commit_id"`
	Description string `json:"description"`
	Author      struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"author"`
	Timestamp   string   `json:"timestamp"`
	Bookmarks   []string `json:"bookmarks"`
	Workspaces  []string `json:"workspaces"`
	Immutable   bool     `json:"immutable"`
	WorkingCopy bool     `json:"working_copy"`
}

func (c Client) Log(ctx context.Context) ([]LogLine, error) {
	out, err := c.run(ctx, "log", "--revisions=::", "--config", "ui.log-word-wrap=false", "-T", logTemplate)
	if err != nil {
		return nil, err
	}
	var lines []LogLine
	for line := range strings.Lines(string(out)) {
		line = strings.TrimSuffix(line, "\n")
		graph, data, found := strings.Cut(line, "\x1f")
		if !found {
			lines = append(lines, LogLine{Graph: line})
			continue
		}
		var entry logEntryJSON
		if err := json.Unmarshal([]byte(data), &entry); err != nil {
			return nil, fmt.Errorf("jj log: decode entry %q: %w", data, err)
		}
		author := entry.Author.Name
		if author == "" {
			author = entry.Author.Email
		}
		var bookmarks []string
		if len(entry.Bookmarks) > 0 {
			bookmarks = entry.Bookmarks
		}
		var workspaces []string
		if len(entry.Workspaces) > 0 {
			workspaces = entry.Workspaces
		}
		lines = append(lines, LogLine{Graph: graph, Entry: &LogEntry{
			ChangeID:    entry.ChangeID,
			CommitID:    entry.CommitID,
			Description: entry.Description,
			Author:      author,
			Timestamp:   entry.Timestamp,
			Bookmarks:   bookmarks,
			Workspaces:  workspaces,
			Immutable:   entry.Immutable,
			WorkingCopy: entry.WorkingCopy,
		}})
	}
	return lines, nil
}
