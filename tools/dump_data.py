"""Dump NLE + AutoAscend static tables to JSON for the Go port. Run inside the arena image with the bot at /sol."""
import json, sys, functools
sys.path.insert(0, '/sol')
import numpy as np
import nle.nethack as nh
from nle.nethack import actions as A
from autoascend import objects as O
from autoascend.glyph import G, SS, MON
from autoascend.glyph import monflag
from autoascend import soko_solver
import arena_adapter

out = {}
consts = {}
for k in dir(nh):
    v = getattr(nh, k)
    if k.isupper() and isinstance(v, (int, np.integer)) and not isinstance(v, bool):
        consts[k] = int(v)
out['nh'] = consts

MAXG = nh.MAX_GLYPH
out['glyph'] = {
    'is_monster': [bool(nh.glyph_is_monster(i)) for i in range(MAXG)],
    'is_pet': [bool(nh.glyph_is_pet(i)) for i in range(MAXG)],
    'is_body': [bool(nh.glyph_is_body(i)) for i in range(MAXG)],
    'is_statue': [bool(nh.glyph_is_statue(i)) for i in range(MAXG)],
    'is_object': [bool(nh.glyph_is_object(i)) for i in range(MAXG)],
    'is_normal_object': [bool(nh.glyph_is_normal_object(i)) for i in range(MAXG)],
    'to_mon': [int(nh.glyph_to_mon(i)) for i in range(MAXG)],
    'to_obj': [int(nh.glyph_to_obj(i)) for i in range(MAXG)],
}
# glyph_to_pet may not exist in all versions
try:
    out['glyph']['to_pet'] = [int(nh.glyph_to_pet(i)) if nh.glyph_is_pet(i) else -1 for i in range(MAXG)]
except AttributeError:
    out['glyph']['to_pet'] = None

mons = []
for i in range(nh.NUMMONS):
    p = nh.permonst(i)
    d = {}
    for f in ['mname', 'mlet', 'mlevel', 'mmove', 'ac', 'mr', 'maligntyp', 'geno', 'cwt', 'cnutrit', 'msound',
              'msize', 'mresists', 'mconveys', 'mflags1', 'mflags2', 'mflags3', 'difficulty', 'mcolor']:
        if not hasattr(p, f):
            continue
        v = getattr(p, f)
        d[f] = v if isinstance(v, str) else int(v)
    mons.append(d)
out['permonst'] = mons

objcls = []
for i in range(nh.NUM_OBJECTS):
    c = nh.objclass(i)
    dsc = nh.objdescr.from_idx(i)
    objcls.append({'oc_class': ord(c.oc_class), 'oc_name': dsc.oc_name, 'oc_descr': dsc.oc_descr})
out['objclass'] = objcls

out['actions'] = [[a.name if hasattr(a, 'name') else str(a), int(a)] for a in nh.ACTIONS]
out['action_to_index'] = {str(k): v for k, v in arena_adapter._ACTION_TO_INDEX.items()}
out['action_enums'] = {}
for cls in ['CompassDirection', 'CompassDirectionLonger', 'MiscDirection', 'MiscAction', 'TextCharacters', 'Command', 'UnsafeActions']:
    if hasattr(A, cls):
        out['action_enums'][cls] = {m.name: int(m.value) for m in getattr(A, cls)}

# AutoAscend objects
objs = []
ids = {}
for i, o in enumerate(O.objects):
    if o is None:
        objs.append(None)
        continue
    ids[id(o)] = i
    fields = {}
    for f in o._fields:
        v = getattr(o, f)
        fields[f] = v
    objs.append({'type': type(o).__name__, 'fields': fields})
out['objects'] = objs
nonnull = [o for o in O.objects if o is not None]
out['objects_distinct'] = len(set(nonnull)) == len(nonnull)

def idx(o):
    return ids[id(o)]

# python tuple-order rank among objects (comparisons that raise are resolved by index)
def cmp(a, b):
    oa, ob = O.objects[a], O.objects[b]
    try:
        if oa < ob: return -1
        if ob < oa: return 1
        return 0
    except TypeError:
        return (a > b) - (a < b)
order = sorted([i for i, o in enumerate(O.objects) if o is not None], key=functools.cmp_to_key(cmp))
out['object_sort_order'] = order

poss = {}
for g in range(nh.GLYPH_OBJ_OFF, nh.GLYPH_OBJ_OFF + nh.NUM_OBJECTS):
    try:
        r = O.possibilities_from_glyph(g)
        poss[g] = [idx(o) for o in r]
    except AssertionError as e:
        poss[g] = {'error': 'AssertionError', 'msg': repr(e)}
    except Exception as e:
        poss[g] = {'error': type(e).__name__, 'msg': repr(e)}
out['possibilities_from_glyph'] = poss
pg = {}
for i, o in enumerate(O.objects):
    if o is None: continue
    try:
        pg[i] = O.possible_glyphs_from_object(o)
    except Exception as e:
        pg[i] = {'error': type(e).__name__}
out['possible_glyphs_from_object'] = pg
out['get_category'] = {i: O.get_category(o) for i, o in enumerate(O.objects) if o is not None}
# objects.index(obj) uses value equality: record first equal index
out['objects_index'] = {i: O.objects.index(o) for i, o in enumerate(O.objects) if o is not None}

oconsts = {}
for k in dir(O):
    v = getattr(O, k)
    if k.isupper() and isinstance(v, int) and not isinstance(v, bool):
        oconsts[k] = v
out['O_consts'] = oconsts

mf = {}
for k in dir(monflag):
    v = getattr(monflag, k)
    if k.isupper() and isinstance(v, int) and not isinstance(v, bool):
        mf[k] = v
out['monflag'] = mf

ss = {}
for k in dir(SS):
    v = getattr(SS, k)
    if k.startswith('S_') and isinstance(v, int):
        ss[k] = v
out['SS'] = ss

gsets = {}
for k, v in G.DICT.items():
    if isinstance(v, (set, frozenset)):
        gsets[k] = sorted(int(x) for x in v)
out['G'] = gsets

soko = []
for smap, answer in soko_solver.maps.items():
    try:
        sm = soko_solver.convert_map(smap)
        conv = {'grid': sm.sokomap.astype(int).tolist(), 'pos': [int(sm.pos[0]), int(sm.pos[1])]}
    except Exception as e:
        conv = {'error': type(e).__name__, 'msg': str(e)}
    soko.append({'map': smap, 'conv': conv, 'answer': [[list(map(int, p)), list(map(int, d))] for p, d in answer]})
out['soko_maps'] = soko
out['soko_consts'] = {k: getattr(soko_solver, k) for k in dir(soko_solver) if k.isupper() and isinstance(getattr(soko_solver, k), int)}

json.dump(out, open('/out/nhdata.json', 'w'))
print('ok', len(objs), len(mons), MAXG, out['objects_distinct'])
