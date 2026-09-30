"""Where does arena env.step time go? Replays a reference trace and splits env.step into the raw
NetHack C step (nle._pynethack via Nethack.step) and everything else (gym wrappers, obs dict, progress)."""
import json, sys, time
from nethackers.arena.environment import make_environment
from nethackers.arena.seeds import trajectory_spec

ref, seed = sys.argv[1], int(sys.argv[2])
acts = [json.loads(l)[0] for l in open(f'{ref}/trace_{seed}.jsonl')][1:]
env = make_environment(1_000_000, 10_000, 'wiz-gno-neu-mal')
nh = env._env.nethack
raw_step = nh.step
acc = [0.0]
def timed_step(*a, **k):
    t = time.perf_counter()
    r = raw_step(*a, **k)
    acc[0] += time.perf_counter() - t
    return r
nh.step = timed_step
env.reset(trajectory_spec('public', 'local', seed))
t0 = time.perf_counter(); n = 0
for a in acts:
    _, _, term, trunc = env.step(a)
    n += 1
    if term or trunc:
        break
tot = time.perf_counter() - t0
print(f'seed {seed}: {n} steps, env.step {1e6*tot/n:.1f} us/step, raw C step {1e6*acc[0]/n:.1f} us/step, python wrappers {1e6*(tot-acc[0])/n:.1f} us/step')
