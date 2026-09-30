#!/bin/sh
# usage: harness/speed.sh [base-rev] [seeds]   (default: 57e22a2 = literal port + corpus test; seeds 0,1001,1047)
# Replays corpus seeds one by one in the working tree and at base-rev, alternating 3 times, and prints CPU
# us/step (amd64 under Rosetta on the mac: compare the two numbers, not with the arena).
set -e
G=$(cd "$(dirname "$0")/.." && pwd)
BASE=${1:-57e22a2}; SEEDS=${2:-0,1001,1047}
W=$(mktemp -d)/base
git -C "$G" worktree add -q --detach "$W" "$BASE"
trap 'git -C "$G" worktree remove --force "$W"' EXIT
cp "$G/equiv/speed_test.go" "$W/equiv/"
# before the package split the driver lived in package aa and the observation type in aa or nle
if [ ! -d "$W/brain" ]; then
  sed -i.bak 's#"aago/brain"#"aago/aa"#; s/brain\./aa./g' "$W/equiv/speed_test.go"
  grep -q 'func ObsFromWire' "$W/nle/"*.go 2>/dev/null || sed -i.bak 's#^\t"aago/nle"$##; s/nle\.ObsFromWire/aa.ObsFromWire/' "$W/equiv/speed_test.go"
  rm -f "$W/equiv/speed_test.go.bak"
fi
(cd "$W" && GOARCH=amd64 go test -c -o "$W/equiv.test" ./equiv)
(cd "$G" && GOARCH=amd64 go test -c -o "$W/new.test" ./equiv)
for i in 1 2 3; do
  for v in base new; do
    t=$W/equiv.test; [ $v = new ] && t=$W/new.test
    r=$(cd "$G/equiv" && GOBOT_CORPUS=$G/corpus GOBOT_SPEED=$SEEDS GOGC=400 "$t" -test.run 'TestSpeed$' -test.v | grep -o 'SPEED.*')
    echo "$v $r"
  done
done
