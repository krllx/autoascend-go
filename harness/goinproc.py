"""Play arena seeds in-process with a bot dir (Python AutoAscend or the Go shim) and compare with a reference trace.

usage (inside the arena image): python goinproc.py BOT_DIR SEED[,SEED...] [--ref DIR] [--full] [--max-steps N] [--dump DIR]
                                [--record DIR]
- with --dump: writes DIR/trace_<seed>.jsonl in the reference format ([action, None, message] per row, row 0 = reset);
- with --record: writes DIR/<seed>.nhc, the observation corpus read by gobot/internal/corpus: b'NHC1', uint32 obs size,
  then one zlib stream of obs0, action0, obs1, action1, ..., obs_n (each obs XOR the previous one, packed exactly like
  bot.py sends it; action = uint16 index);
- with --ref: compares actions step by step with DIR/trace_<seed>.jsonl; stops at the first mismatch unless --full;
- reports bot CPU: for the Go shim the child's CPU (utime+stime from /proc), else process CPU minus env time.
"""
import argparse, json, os, struct, sys, time, zlib

ap = argparse.ArgumentParser()
ap.add_argument('bot')
ap.add_argument('seeds')
ap.add_argument('--ref', default=None)
ap.add_argument('--full', action='store_true')
ap.add_argument('--max-steps', type=int, default=1_000_000)
ap.add_argument('--char', default='wiz-gno-neu-mal')
ap.add_argument('--dump', default=None)
ap.add_argument('--record', default=None)
a = ap.parse_args()

# an empty bot dir (Docker Desktop mounts an unshared host path as an empty dir) would silently import the
# bot.py next to this script instead
if not os.path.exists(os.path.join(a.bot, 'bot.py')):
    sys.exit(f'no bot.py in {a.bot}')
sys.path.insert(0, a.bot)
os.chdir(a.bot)
import numpy as np  # noqa
from nethackers.arena.environment import make_environment  # noqa
from nethackers.arena.sandbox import _readonly  # noqa
from nethackers.arena.seeds import trajectory_spec  # noqa
import bot as botmod  # noqa

TICK = os.sysconf('SC_CLK_TCK')
KEYS = [('glyphs', np.int16), ('chars', np.uint8), ('specials', np.uint8), ('blstats', np.int64),
        ('message', np.uint8), ('tty_chars', np.uint8), ('tty_cursor', np.uint8), ('misc', np.int32),
        ('inv_strs', np.uint8), ('inv_letters', np.uint8), ('inv_oclasses', np.uint8), ('inv_glyphs', np.int16)]


class Recorder:
    """Observation corpus writer (format in the module docstring)."""

    def __init__(self, path):
        self.f = open(path, 'wb')
        self.z = zlib.compressobj(6)
        self.prev = None

    def obs(self, o):
        cur = np.concatenate([np.ascontiguousarray(o[k], dtype=t).view(np.uint8).ravel() for k, t in KEYS])
        if self.prev is None:
            self.f.write(b'NHC1' + struct.pack('<I', cur.size))
            d = cur
        else:
            d = cur ^ self.prev
        self.prev = cur
        self.f.write(self.z.compress(d.tobytes()))

    def action(self, a):
        self.f.write(self.z.compress(struct.pack('<H', a)))

    def close(self):
        self.f.write(self.z.flush())
        self.f.close()


def proc_cpu(pid):
    try:
        f = open(f'/proc/{pid}/stat').read().rsplit(')', 1)[1].split()
        return (int(f[11]) + int(f[12])) / TICK
    except OSError:
        return None


for seed in map(int, a.seeds.split(',')):
    spec = trajectory_spec('public', 'local', seed)
    ref = refmsg = None
    if a.ref:
        rows = [json.loads(l) for l in open(os.path.join(a.ref, f'trace_{seed}.jsonl'))]
        ref = [r[0] for r in rows[1:]]
        refmsg = [r[2] for r in rows]
    env = make_environment(a.max_steps, 10_000, a.char)
    agent = botmod.make_agent()
    obs = env.reset(spec)
    c0, w0 = time.process_time(), time.perf_counter()
    agent.reset(_readonly({k: v.copy() for k, v in obs.items()}))
    env_s, steps, mismatch = 0.0, 0, None
    dump = open(os.path.join(a.dump, f'trace_{seed}.jsonl'), 'w') if a.dump else None
    rec = Recorder(os.path.join(a.record, f'{seed}.nhc')) if a.record else None
    if rec:
        rec.obs(obs)

    def msg(o):
        return bytes(o['message']).split(b'\0')[0].decode('latin1')

    if dump:
        dump.write(json.dumps([None, None, msg(obs)]) + '\n')
    while steps < a.max_steps:
        action = agent.act(_readonly({k: v.copy() for k, v in obs.items()}))
        if ref is not None and mismatch is None and (steps >= len(ref) or ref[steps] != action):
            mismatch = {'step': steps, 'go': action, 'ref': ref[steps] if steps < len(ref) else None,
                        'msgs': refmsg[max(0, steps - 5):steps + 2]}
            if not a.full:
                break
        t = time.perf_counter()
        obs, _r, term, trunc = env.step(action)
        env_s += time.perf_counter() - t
        if rec:
            rec.action(action)
            rec.obs(obs)
        if dump:
            dump.write(json.dumps([action, None, msg(obs)]) + '\n')
        steps += 1
        if term or trunc:
            break
    cpu, wall = time.process_time() - c0, time.perf_counter() - w0
    gpid = getattr(getattr(agent, 'proc', None), 'pid', None)
    gcpu = proc_cpu(gpid) if gpid else None
    agent.close()
    if dump:
        dump.close()
    if rec:
        rec.close()
    m = env.metrics()
    env.close()
    if ref is not None and mismatch is None and steps != len(ref):
        mismatch = {'step': steps, 'go': 'end', 'ref': len(ref)}
    bot_cpu = gcpu if gcpu is not None else cpu - env_s
    print(json.dumps({'seed': seed, 'steps': steps, 'turns': m.turns, 'progress': round(m.progress, 5),
                      'depth': getattr(m, 'max_depth', None), 'end': getattr(m, 'end_status', None),
                      'death': getattr(m, 'cause_of_death', None),
                      'bot_cpu_s': round(bot_cpu, 3), 'py_cpu_s': round(cpu, 2), 'wall_s': round(wall, 2),
                      'env_s': round(env_s, 2), 'us_per_step': round(1e6 * bot_cpu / max(steps, 1), 1),
                      'match': None if ref is None else mismatch is None, 'mismatch': mismatch}), flush=True)
