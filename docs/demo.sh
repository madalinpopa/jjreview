#!/bin/sh
set -eu

for tab in review log bookmarks operations; do
	printf 'Set Theme "Rose Pine"\nSource docs/demo/%s.tape\n' "$tab" |
		JJR_DEMO_THEME=docs/demo/dark.toml vhs -q -o "docs/demo/$tab.gif"
	printf 'Set Theme "rose-pine-dawn"\nSource docs/demo/%s.tape\n' "$tab" |
		JJR_DEMO_THEME=docs/demo/light.toml vhs -q -o "docs/demo/$tab-light.gif"
done
