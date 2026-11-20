#!/bin/sh
set -eu

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

vhs -q -o "$tmp/dark.gif" docs/demo.tape
docs/demo-compose.sh "$tmp/dark.gif" docs/demo.gif 0.8
vhs -q -o "$tmp/light.gif" docs/demo-light.tape
docs/demo-compose.sh "$tmp/light.gif" docs/demo-light.gif 0.75
