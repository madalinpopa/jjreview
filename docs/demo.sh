#!/bin/sh
set -eu

JJR_DEMO_THEME=docs/demo-dark.toml vhs -q -o docs/demo.gif docs/demo.tape
JJR_DEMO_THEME=docs/demo-light.toml vhs -q -o docs/demo-light.gif docs/demo-light.tape
