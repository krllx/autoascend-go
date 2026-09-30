"""Replay a reference trace's actions and dump the observation seen before step N (as .npz).

usage (inside arena image): python replay_dump.py REF_DIR SEED STEP OUT.npz
"""
import json, sys
import numpy as np
from nethackers.arena.environment import make_environment
from nethackers.arena.seeds import trajectory_spec

ref, seed, step, out = sys.argv[1], int(sys.argv[2]), int(sys.argv[3]), sys.argv[4]
acts = [json.loads(l)[0] for l in open(f'{ref}/trace_{seed}.jsonl')][1:]
env = make_environment(1_000_000, 10_000, 'wiz-gno-neu-mal')
obs = env.reset(trajectory_spec('public', 'local', seed))
for a in acts[:step]:
    obs, *_ = env.step(a)
np.savez(out, **{k: np.asarray(v) for k, v in obs.items()})
print('message:', bytes(obs['message']).split(b'\0')[0], 'misc', obs['misc'])
for row in obs['tty_chars']:
    print(repr(bytes(row).decode('latin1')))
