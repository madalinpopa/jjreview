package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"

	"github.com/madalinpopa/jjreview/internal/config"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/llm"
	"github.com/madalinpopa/jjreview/internal/splittool"
	"github.com/madalinpopa/jjreview/internal/tui"
)

var version = "dev"

const applySplitCommand = "apply-split"

var (
	errUsage    = errors.New("usage: jjr apply-split SPEC LEFT RIGHT")
	errRevision = errors.New("give one revision, either as an argument or with -r")
)

type options struct {
	rev     string
	repo    string
	version bool
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	err := run(ctx, os.Args[1:], os.Stdout)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, "jjr:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) > 0 && args[0] == applySplitCommand {
		if len(args) != 4 {
			return errUsage
		}
		return splittool.Run(args[1], args[2], args[3])
	}

	opts, err := parseArgs(args)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	if opts.version {
		_, err := fmt.Fprintln(stdout, version)
		return err
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate jjr executable: %w", err)
	}

	path, err := config.Path()
	if err != nil {
		return err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return err
	}

	model := tui.New(ctx, jj.New(opts.repo, exe), provider(cfg), opts.rev).
		WithTheme(cfg.Theme).
		WithEditor(editorCommand(os.Getenv("VISUAL"), os.Getenv("EDITOR")))
	if _, err := tea.NewProgram(model, tea.WithContext(ctx)).Run(); err != nil && !errors.Is(err, tea.ErrProgramKilled) {
		return err
	}
	return nil
}

func provider(cfg config.Config) llm.Provider {
	var opts []option.RequestOption
	if cfg.Anthropic.APIKey != "" {
		opts = append(opts, option.WithAPIKey(cfg.Anthropic.APIKey))
	}
	return llm.NewAnthropic(cfg.Anthropic.Model, opts...)
}

func parseArgs(args []string) (options, error) {
	opts := options{rev: "@"}
	flags := flag.NewFlagSet("jjr", flag.ContinueOnError)
	flags.StringVar(&opts.rev, "r", opts.rev, "revision to review; may also be given as the only argument")
	flags.StringVar(&opts.rev, "revision", opts.rev, "revision to review; may also be given as the only argument")
	flags.StringVar(&opts.repo, "R", ".", "path to the jj repository")
	flags.BoolVar(&opts.version, "version", false, "print the version and exit")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	revFlag := false
	flags.Visit(func(f *flag.Flag) {
		revFlag = revFlag || f.Name == "r" || f.Name == "revision"
	})
	switch {
	case flags.NArg() > 1, flags.NArg() == 1 && revFlag:
		return options{}, errRevision
	case flags.NArg() == 1:
		opts.rev = flags.Arg(0)
	}
	return opts, nil
}

func editorCommand(visual, editor string) func(ctx context.Context, path string) *exec.Cmd {
	command := strings.Fields(visual)
	if len(command) == 0 {
		command = strings.Fields(editor)
	}
	if len(command) == 0 {
		command = []string{"vi"}
	}
	return func(ctx context.Context, path string) *exec.Cmd {
		args := append(slices.Clone(command[1:]), path)
		return exec.CommandContext(ctx, command[0], args...) //nolint:gosec
	}
}
