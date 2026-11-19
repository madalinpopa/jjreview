<h1 align="center">jjr</h1>

<p align="center">
  Review a <a href="https://jj-vcs.github.io/jj/">jj</a> change hunk by hunk and split the hunks you select into their own commits, with messages drafted by AI.
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

`jjr` shows the diff of a jj change in a terminal UI, lets you pick hunks or whole files, and commits the selection with `jj split`, so your jj configuration and `jj undo` still apply. The commit message is generated from the selected hunks and can be edited before committing.

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

Prebuilt archives for Linux and macOS (amd64, arm64) are attached to each [release](https://github.com/madalinpopa/jjreview/releases).

`jjr` requires jj 0.47 or newer in `PATH`. Generated commit messages need an Anthropic API key in the config file, in `ANTHROPIC_API_KEY`, or a profile created with `ant auth login`. Without credentials the message box starts empty and you write the message yourself.

## Quick start

```sh
export ANTHROPIC_API_KEY=<your-api-key>
jjr
```

1. Move to a hunk and press `space` to select it, or press `space` on a file in the file pane to select the whole file.
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

The file tree is on the left and the unified diff of the current file on the right. The diff shows each hunk as it reads after the change: added and context lines only. Press `d` on a hunk to see its removed lines too.

| Key | Action |
|-----|--------|
| `h` / `l` | Focus the file pane / the diff pane |
| `j` / `k` | Next / previous file in the file pane, next / previous line in the diff pane |
| `ctrl+d` / `ctrl+u` | Half a page down / up in the focused pane |
| `ctrl+n` / `ctrl+p` | Next / previous file |
| `space` | Select the hunk under the cursor (in the file pane: the whole file) |
| `S` | Select every open hunk in the file |
| `z` / `enter` | Fold or unfold the hunk under the cursor |
| `Z` | Fold or unfold every hunk in the file |
| `d` | Show or hide the removed lines of the hunk under the cursor |
| `c` | Commit the selected hunks |
| `e` | Open the current file in `$VISUAL` / `$EDITOR` (default `vi`); the change reloads when the editor exits |
| `f` | Switch between changes only and the full file |
| `?` | Show every key in a popup; the bottom bar lists only the essentials |
| `q` | Quit |

In the commit dialog the message is generated from the selected hunks and can be edited freely:

| Key | Action |
|-----|--------|
| `ctrl+enter` / `ctrl+s` | Commit with the message |
| `ctrl+r` | Generate a new message |
| `esc` | Cancel |

### How committing works

Committing runs `jj split` on the reviewed change. The selected hunks stay in the original change, which gets the new message; the rest moves to a new child change that keeps the original description, and later commits split from it. Committed hunks stay on screen, dimmed and marked with the new change ID. Use `jj undo` to revert a commit made by mistake.

## Configuration

The config file is `$XDG_CONFIG_HOME/jjr/config.toml`, or `~/.config/jjr/config.toml` when `XDG_CONFIG_HOME` is not set. Every setting is optional; a missing file means the defaults below. Keep the file private (`chmod 600`) if it holds an API key.

```toml
provider = "anthropic"

[providers.anthropic]
api_key = ""                  # falls back to ANTHROPIC_API_KEY / ant auth login
model = "claude-haiku-5-5"

[theme]
page = "none"
text = "none"
card = "none"
border = "bright-black"
chip = "none"
chip_text = "blue"
title = "none"
header = "bright-black"
strong = "none"
dir_name = "bright-black"
muted = "bright-black"
faint = "bright-black"
dim = "bright-black"
added = "green"
added_bg = "none"
removed = "red"
removed_bg = "none"
hunk = "cyan"
hunk_bg = "bright-black"
hunk_bar = "cyan"
folded_bar = "bright-black"
stats = "bright-black"
selection = "blue"
cursor_bg = "bright-black"
key = "yellow"
status = "magenta"
picked = "blue"
on_selection = "black"
badge_idle = "none"
badge_idle_fg = "bright-black"
badge_selected = "none"
modal_border = "bright-black"
input = "none"

[theme.syntax]
type = "cyan"
keyword = "magenta"
function = "blue"
string = "green"
literal = "yellow"
comment = "bright-black"
```

A colour is `none` (the terminal default), one of the 16 ANSI names (`black`, `red`, `green`, `yellow`, `blue`, `magenta`, `cyan`, `white` and their `bright-` forms), a number from 0 to 255, `#rgb` or `#rrggbb`. ANSI names and 0-15 follow the terminal theme. Unknown keys and invalid colours stop `jjr` with an error naming the key.

### Data sent to the model

Only the selected hunks are sent, formatted as a unified diff, together with the commit message instructions. Nothing is sent when you write the message yourself without credentials.

## Development

Requires Go 1.27.4+, jj 0.47+, [Task](https://taskfile.dev), and [golangci-lint](https://golangci-lint.run). The demo GIFs are recorded with [VHS](https://github.com/charmbracelet/vhs).

| Command | Description |
|---------|-------------|
| `task build` | Build `bin/jjr` |
| `task test` | Run the tests with the race detector |
| `task lint` | Lint and verify `go.mod` and `go.sum` are tidy |
| `task format` | Format the code |
| `task check` | Test, format, then lint |
| `vhs docs/demo.tape` | Record the demo in a throwaway repository (`docs/demo-light.tape` for the light theme) |

| Package | Responsibility |
|---------|----------------|
| [`cmd/jjr`](cmd/jjr) | Entry point; flags, config loading, and the `jjr apply-split` subcommand |
| [`internal/jj`](internal/jj) | jj operations through the `jj` binary |
| [`internal/diff`](internal/diff) | Parsing `jj diff --git` output, applying selected hunks, formatting diffs |
| [`internal/splittool`](internal/splittool) | The `jjr apply-split` diff editor used by `jj split` |
| [`internal/config`](internal/config) | Loading the TOML config file |
| [`internal/theme`](internal/theme) | Colours and colour parsing |
| [`internal/prompt`](internal/prompt) | Commit message system prompt and prompt construction |
| [`internal/llm`](internal/llm) | Commit message `Provider` interface and the Anthropic implementation |
| [`internal/tui`](internal/tui) | The Bubble Tea interface |

## Contributing

1. Open an issue for significant changes before starting work.
2. Create a branch from `main`.
3. Write commits in the [Conventional Commits](https://www.conventionalcommits.org/) format.
4. Run `task check` before pushing.
5. Open a pull request with a clear description of the change and its motivation.

## License

[MIT](LICENSE)
