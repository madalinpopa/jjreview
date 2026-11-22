package jj

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const MinVersion = "0.47"

var (
	ErrNotFound           = errors.New("jj not found in PATH; jj " + MinVersion + " or newer is required")
	ErrUnsupportedVersion = errors.New("requires jj " + MinVersion + " or newer")
	ErrUnknownVersion     = errors.New("unrecognized jj version")
)

type Version struct {
	Major, Minor, Patch int
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

func (v Version) Compare(other Version) int {
	return cmp.Or(
		cmp.Compare(v.Major, other.Major),
		cmp.Compare(v.Minor, other.Minor),
		cmp.Compare(v.Patch, other.Patch),
	)
}

func ParseVersion(output string) (Version, error) {
	output = strings.TrimSpace(output)
	number, _ := strings.CutPrefix(output, "jj ")
	if end := strings.IndexFunc(number, func(r rune) bool { return r != '.' && (r < '0' || r > '9') }); end >= 0 {
		number = number[:end]
	}
	parts := strings.Split(number, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return Version{}, fmt.Errorf("%w %q", ErrUnknownVersion, output)
	}
	var fields [3]int
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return Version{}, fmt.Errorf("%w %q", ErrUnknownVersion, output)
		}
		fields[i] = n
	}
	return Version{Major: fields[0], Minor: fields[1], Patch: fields[2]}, nil
}

func CheckVersion(ctx context.Context, dir string) error {
	minimum, err := ParseVersion(MinVersion)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "jj", "--version")
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if errors.Is(err, exec.ErrNotFound) {
		return ErrNotFound
	}
	if ctxErr := ctx.Err(); err != nil && ctxErr != nil {
		return fmt.Errorf("jj --version: %w", ctxErr)
	}
	if msg := strings.TrimSpace(stderr.String()); err != nil && msg != "" {
		return fmt.Errorf("jj --version: %w: %s", err, msg)
	}
	if err != nil {
		return fmt.Errorf("jj --version: %w", err)
	}
	found, err := ParseVersion(string(out))
	if err != nil {
		return err
	}
	if found.Compare(minimum) < 0 {
		return fmt.Errorf("%w; found jj %s", ErrUnsupportedVersion, found)
	}
	return nil
}
