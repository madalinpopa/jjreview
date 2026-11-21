package jj

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/madalinpopa/jjreview/internal/diff"
)

const changeTemplate = `"{\"commit\":" ++ json(self) ++ ",\"immutable\":" ++ json(immutable) ++ ",\"working_copy\":" ++ json(current_working_copy) ++ "}\n"`

const splitTool = "jjr-apply"

var (
	ErrNoRevision        = errors.New("revision resolved to no commits")
	ErrMultipleRevisions = errors.New("revision resolved to multiple commits")
	ErrNoSplitFiles      = errors.New("no files selected for split")
	ErrSplitIncomplete   = errors.New("split applied but its result could not be loaded")
	ErrStaleRevision     = errors.New("revision changed since its diff was loaded; restart jjr to review it again")
)

type commitJSON struct {
	ChangeID    string   `json:"change_id"`
	CommitID    string   `json:"commit_id"`
	Parents     []string `json:"parents"`
	Description string   `json:"description"`
	Author      struct {
		Name  string `json:"name"`
		Email string `json:"email"`
	} `json:"author"`
}

type changeJSON struct {
	Commit      commitJSON `json:"commit"`
	Immutable   bool       `json:"immutable"`
	WorkingCopy bool       `json:"working_copy"`
}

type Client struct {
	dir       string
	applyTool string
}

func New(dir, applyTool string) Client {
	return Client{dir: dir, applyTool: applyTool}
}

func (c Client) Split(ctx context.Context, changeID, commitID, message string, files []SplitFile) (selected, rest Change, err error) {
	if len(files) == 0 {
		return Change{}, Change{}, fmt.Errorf("split %q: %w", changeID, ErrNoSplitFiles)
	}
	original, err := c.Change(ctx, changeID)
	if err != nil {
		return Change{}, Change{}, err
	}
	if original.CommitID != commitID {
		return Change{}, Change{}, fmt.Errorf("split %q: %w", changeID, ErrStaleRevision)
	}
	specPath, err := writeSplitSpec(files)
	if err != nil {
		return Change{}, Change{}, fmt.Errorf("split %q: %w", changeID, err)
	}
	splitDone := false
	defer func() {
		removeErr := os.Remove(specPath)
		if removeErr == nil {
			return
		}
		removeErr = fmt.Errorf("split %q: remove spec: %w", changeID, removeErr)
		if splitDone && !errors.Is(err, ErrSplitIncomplete) {
			removeErr = fmt.Errorf("%w: %w", ErrSplitIncomplete, removeErr)
		}
		err = errors.Join(err, removeErr)
	}()
	program, err := json.Marshal(c.applyTool)
	if err != nil {
		return Change{}, Change{}, fmt.Errorf("split %q: encode tool path: %w", changeID, err)
	}
	editArgs, err := json.Marshal([]string{"apply-split", specPath, "$left", "$right"})
	if err != nil {
		return Change{}, Change{}, fmt.Errorf("split %q: encode tool arguments: %w", changeID, err)
	}
	_, err = c.run(ctx, "split",
		"--revision="+original.ChangeID,
		"--message="+message,
		"--tool", splitTool,
		"--config", "merge-tools."+splitTool+".program="+string(program),
		"--config", "merge-tools."+splitTool+".edit-args="+string(editArgs),
	)
	if err != nil {
		return Change{}, Change{}, err
	}
	splitDone = true
	selected, err = c.Change(ctx, original.ChangeID)
	if err != nil {
		return Change{ChangeID: original.ChangeID}, Change{}, fmt.Errorf("split %q: %w: %w", changeID, ErrSplitIncomplete, err)
	}
	rest, err = c.Change(ctx, selected.CommitID+"+")
	if err != nil {
		return selected, Change{}, fmt.Errorf("split %q: %w: %w", changeID, ErrSplitIncomplete, err)
	}
	return selected, rest, nil
}

func writeSplitSpec(files []SplitFile) (string, error) {
	spec, err := os.CreateTemp("", "jjr-split-*.json")
	if err != nil {
		return "", fmt.Errorf("create spec: %w", err)
	}
	err = json.MarshalWrite(spec, files)
	if closeErr := spec.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", errors.Join(fmt.Errorf("write spec: %w", err), os.Remove(spec.Name()))
	}
	return spec.Name(), nil
}

func (c Client) Change(ctx context.Context, rev string) (Change, error) {
	out, err := c.run(ctx, "log", "--revision="+rev, "--no-graph", "-T", changeTemplate)
	if err != nil {
		return Change{}, err
	}
	commits, err := decodeCommits(out)
	if err != nil {
		return Change{}, fmt.Errorf("jj log: revision %q: %w", rev, err)
	}
	if len(commits) == 0 {
		return Change{}, fmt.Errorf("jj log: revision %q: %w", rev, ErrNoRevision)
	}
	if len(commits) > 1 {
		return Change{}, fmt.Errorf("jj log: revision %q: %w (%d)", rev, ErrMultipleRevisions, len(commits))
	}
	commit := commits[0].Commit
	author := commit.Author.Name
	if author == "" {
		author = commit.Author.Email
	}
	return Change{
		ChangeID:    commit.ChangeID,
		CommitID:    commit.CommitID,
		Parents:     commit.Parents,
		Author:      author,
		Description: commit.Description,
		Immutable:   commits[0].Immutable,
		WorkingCopy: commits[0].WorkingCopy,
	}, nil
}

func decodeCommits(out []byte) ([]changeJSON, error) {
	var commits []changeJSON
	dec := jsontext.NewDecoder(bytes.NewReader(out))
	for {
		var commit changeJSON
		err := json.UnmarshalDecode(dec, &commit)
		if errors.Is(err, io.EOF) {
			return commits, nil
		}
		if err != nil {
			return nil, fmt.Errorf("decode output: %w", err)
		}
		commits = append(commits, commit)
	}
}

func (c Client) Diff(ctx context.Context, rev string) ([]diff.File, error) {
	change, err := c.Change(ctx, rev)
	if err != nil {
		return nil, err
	}
	out, err := c.run(ctx, "diff", "--revisions="+change.CommitID, "--git")
	if err != nil {
		return nil, err
	}
	files, err := diff.Parse(bytes.NewReader(out))
	if err != nil {
		return nil, fmt.Errorf("parse jj diff of %q: %w", rev, err)
	}
	return files, nil
}

func (c Client) WorkspaceRoot(ctx context.Context) (string, error) {
	out, err := c.run(ctx, "workspace", "root")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (c Client) FileContent(ctx context.Context, rev, path string) (string, error) {
	out, err := c.run(ctx, "file", "show", "--revision="+rev, fileset(path))
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func fileset(path string) string {
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(path)
	return `root-file:"` + escaped + `"`
}

func (c Client) run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "jj", append([]string{"--no-pager", "--color", "never"}, args...)...) //nolint:gosec
	cmd.Dir = c.dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		subcommand := strings.Join(args[:subcommandLen(args)], " ")
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return nil, fmt.Errorf("jj %s: %w: %s", subcommand, err, msg)
		}
		return nil, fmt.Errorf("jj %s: %w", subcommand, err)
	}
	return out, nil
}

func subcommandLen(args []string) int {
	for i, arg := range args {
		if strings.HasPrefix(arg, "-") {
			return i
		}
	}
	return len(args)
}
