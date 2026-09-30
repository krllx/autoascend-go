#!/bin/sh
# usage: harness/build.sh <out-dir> [git-rev]
# Builds aabot for the arena (linux/amd64, no FMA, the grid ops in AVX2 when the CPU has it) into <out-dir>/ together
# with the bot.py shim.
# With a git revision, builds that revision from a temporary worktree (e.g. v0-literal-port = frozen reference).
# The build is reproducible: same source -> same bytes.
set -e
G=$(cd "$(dirname "$0")/.." && pwd)
OUT=$1; REV=$2
mkdir -p "$OUT"; OUT=$(cd "$OUT" && pwd)
SRC=$G
if [ -n "$REV" ]; then
  SRC=$(mktemp -d)/src
  git -C "$G" worktree add -q --detach "$SRC" "$REV"
  trap 'git -C "$G" worktree remove --force "$SRC"' EXIT
fi
cd "$SRC"
EXP=; [ -f grid/vec_simd.go ] && EXP=simd # older revisions (go 1.25) have no SIMD code
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 GOEXPERIMENT=$EXP go build -trimpath -buildvcs=false -o "$OUT/aabot" ./cmd/aabot
cp "$SRC/harness/bot.py" "$OUT/bot.py"
