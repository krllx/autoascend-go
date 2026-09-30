#!/bin/sh
# usage (on blackbox): run_remote.sh <label> <bot-dir> <parallel> <seeds...> [-- extra goinproc args]
# One container per seed; output: ~/nethackers-work/gobot/out/<label>/<seed>.log
W=$HOME/nethackers-work
IMG=ghcr.io/dunnolab/nethackers-arena@sha256:0d0b0e779ebda4a05b5a22b39cfafcd2d2d9ef739ea60d9aae79052d55c767ae
LABEL=$1; BOT=$(realpath "$2"); PAR=$3; shift 3
SEEDS=""; while [ $# -gt 0 ] && [ "$1" != "--" ]; do SEEDS="$SEEDS $1"; shift; done
[ "$1" = "--" ] && shift
OUT=$W/gobot/out/$LABEL; rm -rf "$OUT"; mkdir -p "$OUT"
for s in $SEEDS; do echo $s; done | xargs -P "$PAR" -I{} sh -c "docker run --rm --network none --cap-drop ALL --user $(id -u):$(id -g) -e HOME=/tmp \
  -e PYTHONWARNINGS=ignore -e AABOT_ACTION_TIMEOUT=${AABOT_ACTION_TIMEOUT:-118} -e GOMAXPROCS=${GOMAXPROCS:-2} -e LD_PRELOAD=/w/faketime.so -e FAKE_EPOCH=\${FAKE_EPOCH:-1700000000} \
  -v $BOT:/sol:ro -v $W/perf:/w:ro -v $W/gobot/harness:/h:ro -v $OUT:/out -e AABOT_CPUPROFILE=${AABOT_CPUPROFILE:-} -e AABOT_MEMPROFILE=${AABOT_MEMPROFILE:-} -e GOGC=${GOGC:-} -e GOMEMLIMIT=${GOMEMLIMIT:-} --entrypoint python $IMG /h/goinproc.py /sol {} $* > $OUT/{}.log 2>&1"
cat $OUT/*.log | grep '^{' | python3 -c "
import json,sys
rows=[json.loads(l) for l in sys.stdin]
for r in sorted(rows, key=lambda r: r['seed']):
    mm = r['mismatch']
    print(r['seed'], 'steps', r['steps'], 'prog', r['progress'], 'us/step', r['us_per_step'], 'cpu', r['bot_cpu_s'],
          'MATCH' if r['match'] else ('first diff @%s' % mm['step'] if mm else ''), r.get('death') or '')
print(len(rows), 'rows;', sum(1 for r in rows if r['match']), 'full matches')
"
