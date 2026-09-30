"""Replay a reference trace and report the first step whose observation hash differs from the trace."""
import hashlib, json, sys
import numpy as np
from nethackers.arena.environment import make_environment
from nethackers.arena.seeds import trajectory_spec

ref, seed = sys.argv[1], int(sys.argv[2])
rows = [json.loads(l) for l in open(f'{ref}/trace_{seed}.jsonl')]
def h(obs):
    return hashlib.sha1(b"".join(np.ascontiguousarray(obs[k]).tobytes() for k in sorted(obs))).hexdigest()[:12]
env = make_environment(1_000_000, 10_000, 'wiz-gno-neu-mal')
obs = env.reset(trajectory_spec('public', 'local', seed))
print('reset', h(obs) == rows[0][1])
for k, r in enumerate(rows[1:]):
    obs, *_ = env.step(r[0])
    if h(obs) != r[1]:
        print('first hash diff after action', k, 'msg', bytes(obs['message']).split(b'\0')[0], 'ref msg', r[2])
        break
else:
    print('all', len(rows) - 1, 'steps identical')
