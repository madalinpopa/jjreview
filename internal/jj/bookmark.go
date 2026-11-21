package jj

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
)

const bookmarkTemplate = `"{\"name\":" ++ json(name) ++ ",\"remote\":" ++ json(remote) ++ ",\"present\":" ++ json(present) ++ ",\"conflict\":" ++ json(conflict) ++ ",\"tracked\":" ++ json(tracked) ++ ",\"synced\":" ++ json(synced) ++ ",\"ahead\":" ++ if(remote && tracked, json(self.tracking_ahead_count().lower()), "0") ++ ",\"ahead_exact\":" ++ if(remote && tracked, json(self.tracking_ahead_count().exact()), "0") ++ ",\"behind\":" ++ if(remote && tracked, json(self.tracking_behind_count().lower()), "0") ++ ",\"behind_exact\":" ++ if(remote && tracked, json(self.tracking_behind_count().exact()), "0") ++ ",\"target\":" ++ if(normal_target, json(normal_target), "null") ++ ",\"added\":[" ++ if(conflict, added_targets.map(|c| if(c, json(c.commit_id()), "null")).join(",")) ++ "],\"removed\":[" ++ if(conflict, removed_targets.map(|c| if(c, json(c.commit_id()), "null")).join(",")) ++ "]}\n"`

const (
	gitRemote        = "git"
	backwardsRefusal = "Refusing to move bookmark backwards or sideways"
	nothingMoved     = "No bookmarks to update."
)

var (
	ErrBookmarkBackwards = errors.New("bookmark would move backwards or sideways")
	ErrBookmarkChanged   = errors.New("bookmark no longer points to the expected target")
)

var progressLine = regexp.MustCompile(`^(Created|Moved|Deleted|Renamed|Forgot|Started tracking|Stopped tracking) \d+ (remote )?bookmarks|^No bookmarks to (update|delete)\.|^Nothing changed\.`)

type Bookmark struct {
	Name        string
	ChangeID    string
	CommitID    string
	Description string
	Conflict    bool
	Added       []string
	Removed     []string
	Remotes     []RemoteBookmark
}

type RemoteBookmark struct {
	Remote        string
	Ahead         int
	AheadAtLeast  bool
	Behind        int
	BehindAtLeast bool
	Synced        bool
}

type bookmarkJSON struct {
	Name     string      `json:"name"`
	Remote   string      `json:"remote"`
	Present  bool        `json:"present"`
	Conflict bool        `json:"conflict"`
	Tracked  bool        `json:"tracked"`
	Synced   bool        `json:"synced"`
	Ahead    int         `json:"ahead"`
	Behind   int         `json:"behind"`
	AheadEx  *int        `json:"ahead_exact"`
	BehindEx *int        `json:"behind_exact"`
	Target   *commitJSON `json:"target"`
	Added    []*string   `json:"added"`
	Removed  []*string   `json:"removed"`
}

func (c Client) Bookmarks(ctx context.Context) ([]Bookmark, error) {
	out, err := c.run(ctx, "bookmark", "list", "--all-remotes", "-T", bookmarkTemplate)
	if err != nil {
		return nil, err
	}
	var bookmarks []Bookmark
	local := map[string]int{}
	dec := jsontext.NewDecoder(bytes.NewReader(out))
	for {
		var ref bookmarkJSON
		err := json.UnmarshalDecode(dec, &ref)
		if errors.Is(err, io.EOF) {
			return bookmarks, nil
		}
		if err != nil {
			return nil, fmt.Errorf("jj bookmark list: decode output: %w", err)
		}
		switch {
		case ref.Remote == "" && ref.Present:
			local[ref.Name] = len(bookmarks)
			bookmarks = append(bookmarks, newBookmark(ref))
		case ref.Remote != "" && ref.Remote != gitRemote && ref.Tracked:
			if i, ok := local[ref.Name]; ok {
				bookmarks[i].Remotes = append(bookmarks[i].Remotes, RemoteBookmark{
					Remote:        ref.Remote,
					Ahead:         ref.Behind,
					AheadAtLeast:  ref.BehindEx == nil,
					Behind:        ref.Ahead,
					BehindAtLeast: ref.AheadEx == nil,
					Synced:        ref.Synced,
				})
			}
		}
	}
}

func newBookmark(ref bookmarkJSON) Bookmark {
	b := Bookmark{Name: ref.Name, Conflict: ref.Conflict}
	b.Added = targetIDs(ref.Added)
	b.Removed = targetIDs(ref.Removed)
	if ref.Target != nil {
		b.ChangeID = ref.Target.ChangeID
		b.CommitID = ref.Target.CommitID
		b.Description = ref.Target.Description
	}
	return b
}

func targetIDs(targets []*string) []string {
	if len(targets) == 0 {
		return nil
	}
	ids := make([]string, len(targets))
	for i, id := range targets {
		if id != nil {
			ids[i] = *id
		}
	}
	slices.Sort(ids)
	return ids
}

func (c Client) CreateBookmark(ctx context.Context, name, rev string) (string, error) {
	return c.mutate(ctx, "bookmark", "create", "--revision="+rev, "--", quote(name))
}

func (c Client) RenameBookmark(ctx context.Context, oldName, newName string) (string, error) {
	return c.mutate(ctx, "bookmark", "rename", "--", quote(oldName), quote(newName))
}

func (c Client) MoveBookmark(ctx context.Context, name string, from []string, to string, allowBackwards bool) (string, error) {
	sources := make([]string, len(from))
	for i, id := range from {
		sources[i] = CommitRevset(id)
	}
	args := []string{"bookmark", "move", "--from=" + strings.Join(sources, " | "), "--to=" + CommitRevset(to)}
	if allowBackwards {
		args = append(args, "--allow-backwards")
	}
	_, stderr, err := c.output(ctx, append(args, "--", "exact:"+quote(name))...)
	switch {
	case err != nil && strings.Contains(err.Error(), backwardsRefusal):
		return "", fmt.Errorf("%w: %w", ErrBookmarkBackwards, err)
	case err != nil:
		return "", err
	case len(from) == 0 || slices.Contains(strings.Split(stderr, "\n"), nothingMoved):
		return warnings(stderr), fmt.Errorf("move bookmark %s: %w", name, ErrBookmarkChanged)
	}
	bookmarks, err := c.Bookmarks(ctx)
	if err != nil {
		return warnings(stderr), fmt.Errorf("move bookmark %s: verify: %w", name, err)
	}
	i := slices.IndexFunc(bookmarks, func(b Bookmark) bool { return b.Name == name })
	if i < 0 || bookmarks[i].Conflict || bookmarks[i].CommitID != to {
		return warnings(stderr), fmt.Errorf("move bookmark %s: %w", name, ErrBookmarkChanged)
	}
	return warnings(stderr), nil
}

func (c Client) DeleteBookmark(ctx context.Context, name string) (string, error) {
	return c.mutate(ctx, "bookmark", "delete", "--", "exact:"+quote(name))
}

func (c Client) mutate(ctx context.Context, args ...string) (string, error) {
	_, stderr, err := c.output(ctx, args...)
	if err != nil {
		return "", err
	}
	return warnings(stderr), nil
}

func warnings(stderr string) string {
	var kept []string
	inWarning := false
	for line := range strings.Lines(stderr) {
		switch {
		case strings.HasPrefix(line, "Warning:"):
			inWarning = true
		case progressLine.MatchString(line):
			inWarning = false
		}
		if inWarning {
			kept = append(kept, strings.TrimRight(line, "\n"))
		}
	}
	return strings.Join(kept, "\n")
}
