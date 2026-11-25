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
	anthropicoption "github.com/anthropics/anthropic-sdk-go/v2/option"
	"github.com/charmbracelet/x/ansi"
	openaioption "github.com/openai/openai-go/v3/option"
	"google.golang.org/genai"

	"github.com/madalinpopa/jjreview/internal/config"
	"github.com/madalinpopa/jjreview/internal/jj"
	"github.com/madalinpopa/jjreview/internal/llm"
	"github.com/madalinpopa/jjreview/internal/splittool"
	"github.com/madalinpopa/jjreview/internal/theme"
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
	if err := jj.CheckVersion(ctx, opts.repo); err != nil {
		if errors.Is(err, context.Canceled) {
			return nil
		}
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

	gen, err := provider(ctx, cfg)
	if err != nil {
		return err
	}
	if c, ok := gen.(io.Closer); ok {
		defer func() {
			if err := c.Close(); err != nil {
				fmt.Fprintln(os.Stderr, "jjr: warning:", err)
			}
		}()
	}

	model := tui.New(ctx, jj.New(opts.repo, exe), gen, opts.rev).
		WithTheme(cfg.Theme).
		WithEditor(editorCommand(os.Getenv("VISUAL"), os.Getenv("EDITOR")))
	_, err = tea.NewProgram(model, tea.WithContext(ctx), tea.WithOutput(stdout)).Run()
	return finishProgram(stdout, cfg.Theme.Mode, err)
}

func finishProgram(w io.Writer, mode theme.Mode, err error) error {
	if err != nil && !errors.Is(err, tea.ErrProgramKilled) {
		return err
	}
	if resetErr := resetColorScheme(w, mode); resetErr != nil {
		fmt.Fprintln(os.Stderr, "jjr: warning:", resetErr)
	}
	if err == nil || err == tea.ErrProgramKilled || errors.Is(err, tea.ErrInterrupted) || errors.Is(err, context.Canceled) { //nolint:errorlint
		return nil
	}
	return err
}

func resetColorScheme(w io.Writer, mode theme.Mode) error {
	if mode != theme.ModeAuto {
		return nil
	}
	_, err := io.WriteString(w, ansi.ResetModeLightDark)
	return err
}

func provider(ctx context.Context, cfg config.Config) (llm.Provider, error) {
	switch cfg.Provider {
	case config.ProviderNone:
		return nil, nil
	case config.ProviderAnthropic:
		var opts []anthropicoption.RequestOption
		if cfg.Anthropic.APIKey != "" {
			opts = append(opts, anthropicoption.WithAPIKey(cfg.Anthropic.APIKey))
		}
		return llm.NewAnthropic(cfg.Anthropic.Model, opts...), nil
	case config.ProviderCopilot:
		return llm.NewCopilot(cfg.Copilot.Model), nil
	case config.ProviderOpenAI:
		var opts []openaioption.RequestOption
		if cfg.OpenAI.APIKey != "" {
			opts = append(opts, openaioption.WithAPIKey(cfg.OpenAI.APIKey))
		}
		return llm.NewOpenAI(cfg.OpenAI.Model, opts...), nil
	case config.ProviderGemini:
		return llm.NewGemini(ctx, cfg.Gemini.Model, cfg.Gemini.APIKey, genai.HTTPOptions{}), nil
	case config.ProviderClaude:
		return llm.NewClaude(cfg.Claude.Model), nil
	case config.ProviderCodex:
		return llm.NewCodex(cfg.Codex.Model), nil
	default:
		return nil, fmt.Errorf("unknown provider %q", cfg.Provider)
	}
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
