package jj

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/madalinpopa/jjreview/internal/diff"
)

const changeTemplate = `"{\"commit\":" ++ json(self) ++ ",\"immutable\":" ++ json(immutable) ++ ",\"working_copy\":" ++ json(current_working_copy) ++ "}\n"`

const abandonTemplate = `"{\"bookmarks\":" ++ json(self.local_bookmarks().map(|b| b.name())) ++ ",\"working_copy\":" ++ json(self.current_working_copy()) ++ "}\n"`

const splitTool = "jjr-apply"

const globalFlags = "--no-pager --color never"

var (
	ErrNoRevision        = errors.New("revision resolved to no commits")
	ErrMultipleRevisions = errors.New("revision resolved to multiple commits")
	ErrNoSplitFiles      = errors.New("no files selected for split")
	ErrSplitIncomplete   = errors.New("split applied but its result could not be loaded")
	ErrStaleRevision     = errors.New("revision changed since its diff was loaded; restart jjr to review it again")
	ErrHiddenRevision    = errors.New("revision was rewritten or abandoned since it was listed")
	ErrBookmarksChanged  = errors.New("bookmarks on the revision changed since it was listed")
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

const operationArgSafe = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789,-./:@_"

type abandonJSON struct {
	Bookmarks   []string `json:"bookmarks"`
	WorkingCopy bool     `json:"working_copy"`
}

type operationJSON struct {
	ID          string            `json:"id"`
	Description string            `json:"description"`
	Attributes  map[string]string `json:"attributes"`
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
	original, err := c.Change(ctx, ChangeRevset(changeID))
	if errors.Is(err, ErrNoRevision) || errors.Is(err, ErrMultipleRevisions) {
		return Change{}, Change{}, fmt.Errorf("split %q: %w: %w", changeID, ErrStaleRevision, err)
	}
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
	editConfig := "merge-tools." + splitTool + ".edit-args=" + string(editArgs)
	baseOp, err := c.headOperation(ctx)
	if err != nil {
		return Change{}, Change{}, fmt.Errorf("split %q: %w", changeID, err)
	}
	_, err = c.run(ctx, "split",
		"--revision=exactly("+ChangeRevset(original.ChangeID)+", 1) & "+CommitRevset(original.CommitID),
		"--message="+message,
		"--tool", splitTool,
		"--config", "merge-tools."+splitTool+".program="+string(program),
		"--config", editConfig,
	)
	if err != nil {
		selected, splitDone, err = c.splitFailed(ctx, changeID, original, baseOp, editConfig, err)
		return selected, Change{}, err
	}
	splitDone = true
	selected, err = c.Change(ctx, ChangeRevset(original.ChangeID))
	if err != nil {
		return Change{ChangeID: original.ChangeID}, Change{}, fmt.Errorf("split %q: %w: %w", changeID, ErrSplitIncomplete, err)
	}
	rest, err = c.Change(ctx, CommitRevset(selected.CommitID)+"+")
	if err != nil {
		return selected, Change{}, fmt.Errorf("split %q: %w: %w", changeID, ErrSplitIncomplete, err)
	}
	return selected, rest, nil
}

func (c Client) splitFailed(ctx context.Context, changeID string, original Change, baseOp, editConfig string, splitErr error) (Change, bool, error) {
	published, err := c.splitPublished(ctx, baseOp, original.CommitID, editConfig)
	if err != nil {
		return Change{}, false, fmt.Errorf("split %q: %w", changeID, errors.Join(splitErr, err))
	}
	if published {
		return Change{ChangeID: original.ChangeID}, true, fmt.Errorf("split %q: %w: %w", changeID, ErrSplitIncomplete, splitErr)
	}
	current, err := c.Change(ctx, ChangeRevset(original.ChangeID))
	if errors.Is(err, ErrNoRevision) || errors.Is(err, ErrMultipleRevisions) || (err == nil && current.CommitID != original.CommitID) {
		return Change{}, false, fmt.Errorf("split %q: %w: %w", changeID, ErrStaleRevision, splitErr)
	}
	return Change{}, false, splitErr
}

func (c Client) headOperation(ctx context.Context) (string, error) {
	out, err := c.run(ctx, "operation", "log", "--ignore-working-copy", "--no-graph", "--limit=1", "--config", "ui.log-word-wrap=false", "-T", "id")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (c Client) splitPublished(ctx context.Context, since, commitID, editConfig string) (bool, error) {
	out, err := c.run(ctx, "operation", "log", "--ignore-working-copy", "--no-graph", "--config", "ui.log-word-wrap=false", "-T", `json(self) ++ "\n"`)
	if err != nil {
		return false, err
	}
	arg := " --config " + operationArg(editConfig) + " "
	dec := jsontext.NewDecoder(bytes.NewReader(out))
	for {
		var op operationJSON
		err := json.UnmarshalDecode(dec, &op)
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("jj operation log: decode output: %w", err)
		}
		if op.ID == since {
			return false, nil
		}
		if op.Description == "split commit "+commitID && strings.Contains(op.Attributes["args"]+" ", arg) {
			return true, nil
		}
	}
}

func operationArg(arg string) string {
	if strings.TrimLeft(arg, operationArgSafe) == "" {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", `\'`) + "'"
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
	out, err := c.run(ctx, "log", "--revision="+rev, "--no-graph", "--config", "ui.log-word-wrap=false", "-T", changeTemplate)
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

func (c Client) Diff(ctx context.Context, commitID string) ([]diff.File, error) {
	if _, err := c.Change(ctx, CommitRevset(commitID)); err != nil {
		return nil, err
	}
	out, err := c.run(ctx, "diff", "--revisions="+CommitRevset(commitID), "--git")
	if err != nil {
		return nil, err
	}
	files, err := diff.Parse(bytes.NewReader(out))
	if err != nil {
		return nil, fmt.Errorf("parse jj diff of %s: %w", commitID, err)
	}
	return files, nil
}

func (c Client) Abandon(ctx context.Context, commitID string, bookmarks []string) (bool, error) {
	target, err := c.listed(ctx, commitID, bookmarks)
	if err != nil {
		return false, fmt.Errorf("abandon %s: %w", commitID, err)
	}
	if _, err := c.run(ctx, "abandon", "-r", "exactly("+VisibleCommitRevset(commitID)+", 1)"); err != nil {
		return false, err
	}
	return target.WorkingCopy, nil
}

func (c Client) listed(ctx context.Context, commitID string, bookmarks []string) (abandonJSON, error) {
	out, err := c.run(ctx, "log", "--revision="+VisibleCommitRevset(commitID), "--no-graph", "--config", "ui.log-word-wrap=false", "-T", abandonTemplate)
	if err != nil {
		return abandonJSON{}, err
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return abandonJSON{}, ErrHiddenRevision
	}
	var target abandonJSON
	if err := json.Unmarshal(out, &target); err != nil {
		return abandonJSON{}, fmt.Errorf("decode output: %w", err)
	}
	if !slices.Equal(slices.Sorted(slices.Values(target.Bookmarks)), slices.Sorted(slices.Values(bookmarks))) {
		return abandonJSON{}, ErrBookmarksChanged
	}
	return target, nil
}

func (c Client) Successor(ctx context.Context, changeID, commitID string) (Change, error) {
	next, err := c.Change(ctx, ChangeRevset(changeID))
	if err != nil {
		return Change{}, err
	}
	found, err := c.evolvedFrom(ctx, next.CommitID, commitID)
	if err != nil {
		return Change{}, err
	}
	if !found {
		return Change{}, fmt.Errorf("successor of %s: %w", commitID, ErrNoRevision)
	}
	return next, nil
}

func (c Client) evolvedFrom(ctx context.Context, commitID, predecessor string) (bool, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	args := []string{"evolog", "-r", CommitRevset(commitID), "--no-graph", "-T", `self.commit().commit_id() ++ "\n"`}
	cmd := c.command(ctx, args)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return false, jjError(args, err, "")
	}
	if err := cmd.Start(); err != nil {
		return false, jjError(args, err, "")
	}
	found := false
	scanner := bufio.NewScanner(out)
	for scanner.Scan() {
		if scanner.Text() == predecessor {
			found = true
			cancel()
			break
		}
	}
	scanErr := scanner.Err()
	waitErr := cmd.Wait()
	switch {
	case found:
		return true, nil
	case scanErr != nil:
		return false, jjError(args, fmt.Errorf("read output: %w", scanErr), "")
	case waitErr != nil:
		return false, jjError(args, waitErr, stderr.String())
	}
	return false, nil
}

func (c Client) WorkspaceRoot(ctx context.Context) (string, error) {
	out, err := c.run(ctx, "workspace", "root")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (c Client) FileContent(ctx context.Context, commitID, path string) (string, error) {
	out, err := c.run(ctx, "file", "show", "--revision="+CommitRevset(commitID), fileset(path))
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func CommitRevset(commitID string) string {
	return "commit_id(" + quote(commitID) + ")"
}

func VisibleRevset(rev string) string {
	return "(" + rev + ") & visible()"
}

func VisibleCommitRevset(commitID string) string {
	return VisibleRevset(CommitRevset(commitID))
}

func WorkingCopyRevset(changeID string) string {
	return "@ & " + ChangeRevset(changeID)
}

func ChangeRevset(changeID string) string {
	return "change_id(" + quote(changeID) + ")"
}

func fileset(path string) string {
	return "root-file:" + quote(path)
}

func quote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func (c Client) run(ctx context.Context, args ...string) ([]byte, error) {
	out, _, err := c.output(ctx, args...)
	return out, err
}

func (c Client) command(ctx context.Context, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "jj", append(strings.Fields(globalFlags), args...)...) //nolint:gosec
	cmd.Dir = c.dir
	return cmd
}

func (c Client) output(ctx context.Context, args ...string) ([]byte, string, error) {
	cmd := c.command(ctx, args)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, "", jjError(args, err, stderr.String())
	}
	return out, stderr.String(), nil
}

func jjError(args []string, err error, stderr string) error {
	subcommand := strings.Join(args[:subcommandLen(args)], " ")
	if msg := strings.TrimSpace(stderr); msg != "" {
		return fmt.Errorf("jj %s: %w: %s", subcommand, err, msg)
	}
	return fmt.Errorf("jj %s: %w", subcommand, err)
}

func subcommandLen(args []string) int {
	for i, arg := range args {
		if strings.HasPrefix(arg, "-") {
			return i
		}
	}
	return len(args)
}
