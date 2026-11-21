<h1 align="center">jjr</h1>

<p align="center">
  Review a <a href="https://jj-vcs.github.io/jj/">jj</a> change hunk by hunk
  and split the hunks you select into their own commits, with messages drafted
  by AI.
</p>

<p align="center">
  <a href="https://github.com/madalinpopa/jjreview/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/madalinpopa/jjreview/actions/workflows/ci.yml/badge.svg?branch=main"></a>
  <a href="https://github.com/madalinpopa/jjreview/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/madalinpopa/jjreview?include_prereleases&sort=semver"></a>
  <a href="go.mod"><img alt="Go version" src="https://img.shields.io/github/go-mod/go-version/madalinpopa/jjreview"></a>
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/license-MIT-blue"></a>
</p>

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: light)" srcset="docs/demo-light.gif">
    <img alt="jjr demo" src="docs/demo.gif" width="880">
  </picture>
</p>

`jjr` shows the diff of a jj change in a terminal UI, lets you pick hunks or
whole files, and commits the selection with `jj split`, so your jj configuration
and `jj undo` still apply. The commit message is generated from the selected
hunks and can be edited before committing.

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

`jjr` requires jj 0.47 or newer in `PATH`. To generate commit messages, provide
an Anthropic API key (in the config file, in `ANTHROPIC_API_KEY`, or through
`ant auth login`), or set `provider = "copilot"` to use a signed-in
[GitHub Copilot CLI](https://github.com/github/copilot-cli). Without either, you
write the message yourself.

## Quick start

```sh
export ANTHROPIC_API_KEY=<your-api-key>
jjr
```

1. Move to a hunk and press `space` to select it, or press `space` on a file in
   the file pane to select the whole file.
2. Press `c`. The commit dialog lists the selection and drafts a message.
3. Edit the message if needed and press `ctrl+s` to commit.

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
hunk in the file.

| Key | Action |
|-----|--------|
| `1` / `2` | Show the review tab / the log tab |
| `tab` | Switch between the file pane and the diff pane |
| `j` / `k` | Next / previous file in the file pane, next / previous line in the diff pane |
| `ctrl+d` / `ctrl+u` (`pgdown` / `pgup`) | Half a page down / up in the focused pane |
| `g` / `G` (`home` / `end`) | First / last file in the file pane, first / last line in the diff pane |
| `]` / `[` | Next hunk / start of the current or previous hunk |
| `J` / `K` | Next / previous file from either pane |
| `@` | Go back to the working-copy revision |
| `space` | Select the hunk under the cursor (in the file pane: the whole file) |
| `S` | Select every open hunk in the file |
| `z` / `enter` | Fold or unfold the hunk under the cursor |
| `Z` | Fold or unfold every hunk in the file |
| `d` | Show or hide the removed lines of the hunk under the cursor |
| `D` | Show or hide the removed lines of every hunk in the file |
| `c` | Commit the selected hunks |
| `R` | Reload to pick up edits made outside jjr; working-copy revision only, and only while it is still `@` (press `@` after `jj new`) |
| `e` | Edit the current file in `$VISUAL` / `$EDITOR` (default `vi`); working-copy revision only, and only while it is still `@` |
| `f` | Switch between changes only and the full file |
| `?` | Show all keys |
| `q` | Quit |

`q`, `@`, `R` and `e` drop the selection, so they ask you to press the key
again first. `ctrl+c` quits immediately.

Log tab:

The log tab shows every revision (`::`) with its graph, so nothing is elided. A
bar marks the revision under review. Searching keeps only the revisions whose
change or commit ID starts with the query or whose full description, body
included, contains it, ignoring case.

| Key | Action |
|-----|--------|
| `j` / `k` | Next / previous revision |
| `ctrl+d` / `ctrl+u` (`pgdown` / `pgup`) | Scroll half a page down / up, through long file lists too |
| `g` / `G` (`home` / `end`) | First / last revision |
| `enter` | Review the revision under the cursor |
| `l` / `h` | Show / hide the files changed by the revision |
| `/` | Search by change ID, commit ID or description; `enter` keeps the filter |
| `esc` | Clear the search |
| `R` | Refresh the log |
| `?` | Show all keys |
| `q` | Quit |

`enter` drops the selection, so it asks you to press it again first.

Commit dialog:

| Key | Action |
|-----|--------|
| `ctrl+enter` / `ctrl+s` | Commit with the message |
| `ctrl+r` | Generate a new message |
| `esc` | Cancel |

### How committing works

Committing runs `jj split` on the reviewed change. The selected hunks stay in
the original change, which gets the new message; the rest moves to a new child
change that keeps the original description, and later commits split from it.
Committed hunks stay on screen, folded, dimmed and marked with the new change
ID. Use `jj undo` to revert a commit made by mistake.

## Configuration

The config file is `$XDG_CONFIG_HOME/jjr/config.toml`, or
`~/.config/jjr/config.toml` when `XDG_CONFIG_HOME` is not set. Every setting is
optional; a missing file means the defaults below. Keep the file private
(`chmod 600`) if it holds an API key.

```toml
provider = "anthropic"

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

If the terminal reports its background, `jjr` derives the cursor line from it
when `base.cursor_bg` and `base.page` are unset, and on light backgrounds
derives hunk headers when `diff.hunk_bg`, `base.card` and `base.page` are unset.

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

Only the selected hunks are sent, formatted as a unified diff, together with the
commit message instructions. Nothing is sent when you write the message yourself
without credentials.

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
| `docs/demo.sh` | Record both demo GIFs in a throwaway repository with the Rosé Pine themes |

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
