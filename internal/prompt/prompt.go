package prompt

import (
	"fmt"
	"strings"
)

const maxPatchBytes = 200 << 10

const System = `You write commit messages for code changes in the Conventional Commits 1.0.0 format.

Rules:
- The first line is the subject: type(scope): summary
- type is one of feat, fix, docs, style, refactor, perf, test, build, ci, chore, revert.
- scope is the main area of the code that changed, taken from the file paths.
- The summary is in the imperative mood, lowercase after the colon, has no trailing period, and the whole subject stays under 72 characters.
- Add a body only when the change contains several distinct changes. Separate it from the subject with one blank line and write it as short bullet lines starting with "- ".
- Describe only what the diff shows. Do not invent motivation.
- Never credit an AI tool, agent, or model. No Co-authored-by trailer and no "Generated with" line.
- Reply with the commit message only, as plain text. No code fences, no Markdown, no quotes, no explanation.`

func Build(patch string) string {
	var b strings.Builder
	b.WriteString("Write a commit message for the following patch.")
	if len(patch) > maxPatchBytes {
		patch = strings.ToValidUTF8(patch[:maxPatchBytes], "")
		fmt.Fprintf(&b, " The patch was truncated to its first %d bytes; base the message on what is shown.", maxPatchBytes)
	}
	b.WriteString("\n\n<patch>\n")
	b.WriteString(patch)
	if !strings.HasSuffix(patch, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("</patch>")
	return b.String()
}
