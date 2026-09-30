#!/bin/sh
# usage (on the mac): run_local.sh <label> <bot-dir> <parallel> <seeds...> [-- extra goinproc args]
# Same as run_remote.sh, but local Docker (linux/amd64 emulation) and gobot/localcheck for faketime/out.
# gobot/corpus is mounted as /corpus: pass `-- --record /corpus` to write the observation corpus.
G=$(cd "$(dirname "$0")/.." && pwd)
LABEL=$1; BOT=$(cd "$2" && pwd); PAR=$3; shift 3
SEEDS=""; while [ $# -gt 0 ] && [ "$1" != "--" ]; do SEEDS="$SEEDS $1"; shift; done
[ "$1" = "--" ] && shift
OUT=$G/localcheck/out/$LABEL; rm -rf "$OUT"; mkdir -p "$OUT" "$G/corpus"
export G BOT OUT EXTRA="$*"
for s in $SEEDS; do echo $s; done | xargs -P "$PAR" -n 1 sh -c '
docker run --rm --platform linux/amd64 --network none \
  -e PYTHONWARNINGS=ignore -e AABOT_ACTION_TIMEOUT=${AABOT_ACTION_TIMEOUT:-118} -e GOMAXPROCS=${GOMAXPROCS:-2} \
  -e LD_PRELOAD=/g/faketime.so -e FAKE_EPOCH=1700000000 \
  -v $BOT:/sol:ro -v $G/localcheck:/g -v $G/corpus:/corpus -v $G/harness:/h:ro --entrypoint python \
  ghcr.io/dunnolab/nethackers-arena@sha256:0d0b0e779ebda4a05b5a22b39cfafcd2d2d9ef739ea60d9aae79052d55c767ae \
  /h/goinproc.py /sol $0 $EXTRA > $OUT/$0.log 2>&1'
cat $OUT/*.log | grep '^{' | python3 -c "
import json,sys
rows=[json.loads(l) for l in sys.stdin]
for r in sorted(rows, key=lambda r: r['seed']):
    mm = r['mismatch']
    print(r['seed'], 'steps', r['steps'], 'prog', r['progress'], 'us/step', r['us_per_step'], 'wall', r['wall_s'],
          'MATCH' if r['match'] else ('first diff @%s' % mm['step'] if mm else ''), r.get('death') or '')
print(len(rows), 'rows;', sum(1 for r in rows if r['match']), 'full matches')
"
