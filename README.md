<h1 align="center">jjr</h1>

<p align="center">
  Review a <a href="https://jj-vcs.github.io/jj/">jj</a> change hunk by hunk
  and split the hunks or lines you select into their own commits, with messages
  drafted by AI.
</p>

<p align="center">
  <a href="https://github.com/madalinpopa/jjreview/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/madalinpopa/jjreview/actions/workflows/ci.yml/badge.svg?branch=main"></a>
  <a href="https://github.com/madalinpopa/jjreview/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/madalinpopa/jjreview?include_prereleases&sort=semver"></a>
  <a href="go.mod"><img alt="Go version" src="https://img.shields.io/github/go-mod/go-version/madalinpopa/jjreview"></a>
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/license-MIT-blue"></a>
</p>

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: light)" srcset="docs/demo/review-light.gif">
    <img alt="jjr review tab demo" src="docs/demo/review.gif" width="880">
  </picture>
</p>

`jjr` shows the diff of a jj change in a terminal UI, lets you pick single
lines, hunks or whole files, and commits the selection with `jj split`, so your
jj configuration and `jj undo` still apply. When a message provider is
configured, the commit message is generated from the selection; either way, you
can edit it before committing.

The interface may change before v1.

## Installation

Homebrew (macOS and Linux):

```sh
brew install --cask madalinpopa/tap/jjr
```

Go 1.27.4 or newer:

```sh
go install github.com/madalinpopa/jjreview/cmd/jjr@latest
```

Prebuilt archives for Linux and macOS (amd64, arm64) are attached to each
[release](https://github.com/madalinpopa/jjreview/releases).

`jjr` requires jj 0.47 or newer in `PATH`. Commit message generation is off by
default (`provider = "none"`), so you write the message yourself. To turn it on,
set `provider = "anthropic"` and provide an Anthropic API key (in the config
file, in `ANTHROPIC_API_KEY`, or through `ant auth login`), or set
`provider = "copilot"` to use a signed-in
[GitHub Copilot CLI](https://github.com/github/copilot-cli).

## Quick start

```sh
jjr
```

1. Move to a hunk and press `space` to select it, press `s` on a changed line to
   select only that line, or press `space` on a file in the file pane to select
   the whole file.
2. Press `c`. The commit dialog lists the selection and, when a provider is
   configured, drafts a message.
3. Write or edit the message and press `ctrl+s` to commit.

Repeat until the change contains only what you want to keep for later.

## Usage

```text
jjr [-r <revision>] [-R <path>] [revision]
```

| Flag | Description |
|------|-------------|
| `-r`, `--revision <revset>` | Revision to review; defaults to `@`. May also be given as the only argument |
| `-R <path>` | Path to the jj repository; defaults to the current directory |
| `--version` | Print the version and exit |

The revision is any jj revset that resolves to exactly one commit.

### Examples

```sh
jjr              # review the working-copy change (@)
jjr @-           # review another revision, as with jj
jjr -r xyz       # the same, with -r / --revision
jjr -R ~/src/app # review a repository elsewhere
```

### Keys

The file tree is on the left and the unified diff of the current file on the
right. The diff shows each hunk as it reads after the change: added and context
lines only. Press `d` on a hunk to see its removed lines too, or `D` for every
hunk in the file. A hunk that is selected in part always shows its removed
lines, so a replaced line is never committed without you seeing both sides.

A selected line has its line number and sign drawn as a solid block in the
`picked` colour. A hunk header shows `○` when nothing in it is
selected, `◐` when some of its lines are, `●` when all of them are and `✓` once
it is committed. Binary files, symlinks and files without hunks can only be
selected whole.

Lines wider than the diff pane are cut at its edge: `l` and `h` scroll the code
sideways while the line numbers and signs stay in place, and `‹` / `›` mark the
edge that hides text.

`/` searches the code lines on screen for the query as plain text, ignoring
case. Removed lines hidden by default and lines in folded hunks are not
searched until `d` or `z` shows them. Every match is highlighted, the current
one in the selection colour, and the status bar shows its position in the
file, such as `/query 3/12`. In the full-file view, `n` / `N` move to the next
file whose changes match, or whose loaded content matches, and the next `n` /
`N` continues inside it.

| Key | Action |
|-----|--------|
| `1` / `2` / `3` / `4` | Show the review tab / the log tab / the bookmarks tab / the operations tab |
| `tab` | Switch between the file pane and the diff pane |
| `j` / `k` | Next / previous file in the file pane, next / previous line in the diff pane |
| `ctrl+d` / `ctrl+u` (`pgdown` / `pgup`) | Half a page down / up in the focused pane |
| `g` / `G` (`home` / `end`) | First / last file in the file pane, first / last line in the diff pane |
| `]` / `[` | Next hunk / start of the current or previous hunk |
| `J` / `K` | Next / previous file from either pane |
| `h` / `l` | Scroll the diff left / right by 8 columns; a new file starts at the first column again |
| `w` | Zoom the diff pane to the full width and hide the file pane, or zoom back out; `tab` also zooms out and focuses the file pane |
| `@` | Go back to the working-copy revision |
| `/` | Search the diff; `enter` jumps to the first match from the cursor on, `esc` cancels |
| `n` / `N` | Next / previous match; past the last match of a file they move on to the next file with one and wrap around at the ends |
| `esc` | Clear the search; while a line range is open it cancels the range instead |
| `s` | Select the changed line under the cursor |
| `v` | Start a line range at the cursor: move to extend it, `s` or `space` selects its changed lines (or unselects them when all are selected), `esc` or `v` cancels |
| `space` | Select the hunk under the cursor (in the file pane: the whole file); on a hunk selected in part it selects the rest, then unselects it |
| `S` | Select every open hunk in the file; on a file selected in part it selects the rest, then unselects it |
| `z` / `enter` | Fold or unfold the hunk under the cursor |
| `Z` | Fold or unfold every hunk in the file |
| `d` | Show or hide the removed lines of the hunk under the cursor |
| `D` | Show or hide the removed lines of every hunk in the file |
| `c` | Commit the selection |
| `R` | Reload the working copy to pick up edits made outside jjr |
| `e` | Edit the current file in `$VISUAL` / `$EDITOR` (default `vi`); working copy only |
| `f` | Switch between changes only and the full file |
| `x` | Dismiss the notification of a finished command |
| `?` | Show all keys |
| `q` | Quit |

`q`, `@`, `R` and `e` drop the selection, so they ask you to press the key
again first. `ctrl+c` quits immediately.

Log tab:

The log tab shows every revision (`::`) with its graph, so nothing is elided. A
bar marks the revision under review. Searching keeps only the revisions whose
change or commit ID starts with the query or whose full description, body
included, contains it, ignoring case.

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: light)" srcset="docs/demo/log-light.gif">
    <img alt="jjr log tab demo" src="docs/demo/log.gif" width="880">
  </picture>
</p>

| Key | Action |
|-----|--------|
| `j` / `k` | Next / previous revision |
| `ctrl+d` / `ctrl+u` (`pgdown` / `pgup`) | Scroll half a page down / up, through long file lists too |
| `g` / `G` (`home` / `end`) | First / last revision |
| `enter` | Review the revision under the cursor; on the revision already under review it just switches to the review and keeps the selection, on another revision it asks before dropping selected hunks |
| `l` / `h` | Show / hide the files changed by the revision |
| `space` | Select / unselect the revision under the cursor |
| `s` | Squash the selected revisions, or the one under the cursor, into another revision (`jj squash`); pick the destination with `j` / `k`, `y` confirms, `esc` cancels |
| `a` | Abandon the revision under the cursor (`jj abandon`); `y` confirms |
| `f` | Fetch from the remote (`jj git fetch`) |
| `P` | Push the bookmarks of the revision under the cursor, or the change when it has none (`jj git push`); `y` confirms |
| `b` | Create a bookmark on the revision under the cursor, or move an existing one to it (`jj bookmark move`); `y` allows a backwards or sideways move |
| `/` | Search by change ID, commit ID or description; `enter` keeps the filter |
| `esc` | Clear the search, or the selection |
| `R` | Clear the selection and hide all files |
| `ctrl+r` | Refresh the log |
| `x` | Dismiss the notification of a finished command |
| `?` | Show all keys |
| `q` | Quit |

`a` asks for confirmation and names the local bookmarks that will be deleted
and the selected hunks that will be dropped; press `y` to abandon, any other key
cancels. As with `jj abandon`, descendants are rebased onto the parent, and
abandoning `@` starts a new empty change. Immutable revisions can't be
abandoned.

`P` asks for confirmation and names the bookmarks it pushes. A revision without
bookmarks is pushed with `jj git push --change`, which creates a bookmark for
it. Fetch and push use jj's default remote and run without a terminal, so a
remote that needs a password or an SSH passphrase must get it from a credential
helper or `ssh-agent`.

Bookmarks tab:

The bookmarks tab lists the local bookmarks with the change ID, commit ID and
subject of their target. A bar marks the bookmark on the revision under review.
Each tracked remote shows `✓` when it is in sync, or how many commits the local
bookmark is ahead (`↑`) or behind (`↓`); a count jj could only bound is shown as
a minimum, such as `↑10+`. Conflicted bookmarks are marked too.

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: light)" srcset="docs/demo/bookmarks-light.gif">
    <img alt="jjr bookmarks tab demo" src="docs/demo/bookmarks.gif" width="880">
  </picture>
</p>

| Key | Action |
|-----|--------|
| `j` / `k` | Next / previous bookmark |
| `ctrl+d` / `ctrl+u` (`pgdown` / `pgup`) | Scroll half a page down / up |
| `g` / `G` (`home` / `end`) | First / last bookmark |
| `enter` | Review the target revision of the bookmark; on the revision already under review it just switches to the review and keeps the selection, on another revision it asks before dropping selected hunks |
| `n` | Create a bookmark; the target defaults to the commit under review (`commit_id("…")`) |
| `e` | Rename the bookmark or move it to another revision; a conflicted bookmark has to be resolved with `jj bookmark set` first |
| `d` | Delete the bookmark: a box shows it and its target, `y` deletes it and any other key cancels |
| `R` | Refresh the bookmarks |
| `x` | Dismiss the notification of a finished command |
| `?` | Show all keys |
| `q` | Quit |

Bookmark dialog:

| Key | Action |
|-----|--------|
| `tab` / `shift+tab` | Switch between the name and the target revision |
| `enter` / `ctrl+s` | Save the bookmark |
| `esc` | Cancel |

jj refuses to move a bookmark backwards or sideways; the dialog then shows the
move and asks you to press `enter` again to confirm it.

Operations tab:

The operations tab lists the latest 500 operations of `jj op log`, newest
first, with the operation ID, description and age. `l` shows the command that
ran the operation under it, one row per line, when jj recorded one. A bar and an `@` mark the
current operation; snapshots and the root operation are dimmed.

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: light)" srcset="docs/demo/operations-light.gif">
    <img alt="jjr operations tab demo" src="docs/demo/operations.gif" width="880">
  </picture>
</p>

| Key | Action |
|-----|--------|
| `j` / `k` | Next / previous operation |
| `ctrl+d` / `ctrl+u` (`pgdown` / `pgup`) | Scroll half a page down / up |
| `g` / `G` (`home` / `end`) | Newest / oldest operation |
| `l` / `h` | Show / hide the command of the operation |
| `r` | Restore the repo to its state after the operation (`jj op restore`); `y` confirms |
| `u` | Undo the operation by applying its inverse (`jj op revert`); `y` confirms |
| `a` | Abandon the operation from the history (`jj op abandon`); the repo is unchanged; `y` confirms |
| `R` | Refresh the operations |
| `x` | Dismiss the notification of a finished command |
| `?` | Show all keys |
| `q` | Quit |

Each action shows the operation, its command line and its age before it runs;
press `y` to go ahead, any other key cancels. Restore and undo create a new
operation, so they can be undone in turn, and drop the selected hunks; the
prompt says so. The current operation can't be abandoned (undo it first), and
the root operation can't be undone or abandoned.

`R` in the log, bookmarks or operations tab refreshes the lists, not the review;
press `R` in the review tab to pick up file changes.

Fetch, push, abandon, squash, the operation actions and bookmark deletion show
a notification in the bottom-right corner with the command, a spinner while it
runs and then its result. A success closes after a few seconds; a failure or a
warning stays until `x` dismisses it or the next command starts. When the
notification does not fit, or a dialog is open, the result is shown in the
status bar instead.

Commit dialog:

| Key | Action |
|-----|--------|
| `ctrl+enter` / `ctrl+s` | Commit with the message |
| `ctrl+r` | Generate a new message (only when a provider is configured) |
| `esc` | Cancel |

### How committing works

Committing runs `jj split`: the selected hunks and lines stay in the reviewed
change with the new message, and the rest moves to a new child change that
keeps the original description. Committed hunks stay on screen, folded and
dimmed. Use `jj undo` to revert a commit made by mistake.

When only some lines of a hunk are committed, those lines are dimmed and the
hunk stays open until its last line is committed. An added line you leave out
is not in the commit; a removed line you leave out is still in it. The screen
keeps showing the original diff, so you can go on selecting from it; when
reviewing the working copy, press `R` to load the diff of what is left. A
rename or a mode change goes into the first commit that takes any line of the
file.

## Configuration

The config file is `$XDG_CONFIG_HOME/jjr/config.toml`, or
`~/.config/jjr/config.toml` when `XDG_CONFIG_HOME` is not set. Every setting is
optional; a missing file means the defaults below. Keep the file private
(`chmod 600`) if it holds an API key.

```toml
provider = "none"             # "anthropic" or "copilot" to generate messages

[providers.anthropic]
api_key = ""                  # falls back to ANTHROPIC_API_KEY / ant auth login
model = "claude-haiku-5-5"

[providers.copilot]
model = "auto"

[theme]
mode = "auto"

[theme.base]
page = "none"
text = "none"
card = "none"
border = "bright-black"
title = "none"
header = "bright-black"
strong = "none"
muted = "bright-black"
faint = "bright-black"
dim = "bright-black"
selection = "blue"
on_selection = "black"
cursor_bg = "bright-black"

[theme.diff]
added = "green"
added_bg = "none"
removed = "red"
removed_bg = "none"
hunk = "cyan"
hunk_bg = "bright-black"
hunk_bar = "cyan"
folded_bar = "bright-black"
stats = "bright-black"

[theme.files]
dir_name = "bright-black"
picked = "blue"

[theme.bar]
chip = "none"
chip_text = "blue"
key = "yellow"
status = "magenta"
badge_idle = "none"
badge_idle_fg = "bright-black"
badge_selected = "none"

[theme.modal]
border = "bright-black"
input = "none"

[theme.syntax]
type = "cyan"
keyword = "magenta"
function = "blue"
string = "green"
literal = "yellow"
comment = "bright-black"
```

A colour is `none` (terminal default), an ANSI name such as `red` or
`bright-blue`, a number from 0 to 255, `#rgb` or `#rrggbb`. ANSI names and 0-15
follow the terminal theme. Unknown keys and invalid colours stop `jjr` with an
error.

When `base.cursor_bg` or `diff.hunk_bg` is unset, `jjr` may derive it from the
terminal's background colour.

### Light and dark profiles

Colours under `[theme.<section>]` apply to both profiles;
`[theme.light.<section>]` and `[theme.dark.<section>]` override them for one
profile:

```toml
[theme]
mode = "auto"

[theme.diff]
added = "green"

[theme.light.base]
cursor_bg = "#e8e4df"

[theme.dark.syntax]
comment = "#6e6a86"
```

`mode = "light"` or `"dark"` always uses that profile. With `auto` (the
default), `jjr` follows the terminal's background colour and switches when it
changes. Terminals that don't report their background get the dark profile.

### GitHub Copilot

With `provider = "copilot"`, `jjr` uses the `copilot` CLI from `PATH`, or the
one in `COPILOT_CLI_PATH`. Sign in once by running `copilot`. If the CLI is
missing or too old, the message box shows the error. Requests run without tools,
MCP servers or custom instructions, time out after two minutes, and count
against your Copilot plan.

### Data sent to the model

Only the selected hunks and lines are sent, formatted as a unified diff,
together with the commit message instructions. Nothing is sent when you write
the message yourself without credentials.

## Development

Requires Go 1.27.4+, jj 0.47+, [Task](https://taskfile.dev), and
[golangci-lint](https://golangci-lint.run). The demo GIFs are recorded with
[VHS](https://github.com/charmbracelet/vhs).

| Command | Description |
|---------|-------------|
| `task build` | Build `bin/jjr` |
| `task test` | Run the tests with the race detector |
| `task lint` | Lint and verify `go.mod` and `go.sum` are tidy |
| `task format` | Format the code |
| `task check` | Test, format, then lint |
| `docs/demo.sh` | Record a dark and a light demo GIF of each tab in a throwaway repository with the Rosé Pine themes |

| Package | Responsibility |
|---------|----------------|
| [`cmd/jjr`](cmd/jjr) | Entry point; flags, config loading, and the `jjr apply-split` subcommand |
| [`internal/jj`](internal/jj) | jj operations through the `jj` binary |
| [`internal/diff`](internal/diff) | Parsing `jj diff --git` output, applying selected hunks, formatting diffs |
| [`internal/splittool`](internal/splittool) | The `jjr apply-split` diff editor used by `jj split` |
| [`internal/config`](internal/config) | Loading the TOML config file |
| [`internal/theme`](internal/theme) | Colours and colour parsing |
| [`internal/prompt`](internal/prompt) | Commit message system prompt and prompt construction |
| [`internal/llm`](internal/llm) | Commit message `Provider` interface and the Anthropic and Copilot implementations |
| [`internal/tui`](internal/tui) | The Bubble Tea interface |

## Contributing

1. Open an issue for significant changes before starting work.
2. Create a branch from `main`.
3. Write commits in the
   [Conventional Commits](https://www.conventionalcommits.org/) format.
4. Run `task check` before pushing.
5. Open a pull request with a clear description of the change and its
   motivation.

## License

[MIT](LICENSE)
