# gobot architecture

gobot is a Go port of AutoAscend, a NetHack 3.6.6 bot for NLE. The port presses exactly the same keys as the
Python original (checked action for action, see "Checks"). Every piece of bot state is plain data: the call
stack of the bot's routines is an explicit slice of frames, so a snapshot of the bot is a copy of one structure.
It can be saved to a file and restored in another process.

## Packages

```
cmd/aabot     pipe to harness/bot.py: R/A/C messages, S/L snapshots, action timeout watchdog
brain         Driver (Act: observation -> action), the main frame (restart after AgentPanic)
term          the io layer: the step frame (--More--, menus, [yn], wishes), dialogs, raw keys; implements world.Terminal
strategy      milestones and preempting layers as tables, level-to-level travel, Sokoban solver
tactics       nodes: Check proposes a body frame (fight, emergency, eating, altars, traps, doors, items, exploring)
skills        multi-step actions as frames: move, go to, search, kick, doors, untrap, engrave, wield/wear/eat, pickup/drop, containers
sense         what the bot learns by asking the game: the update phase, ':' look, '#terrain', ';' monsters, '\' known, ^X, #enhance, containers
flow          Node, Pred, combinators (Preempt, Repeat, Every, If, Until, Before, Loop), position guard
assess        pure evaluations: best weapon/armor/ranged sets, item priority split, fight priorities, wand simulation
world         all state as data (Agent, Level, Item, ItemManager, Inventory, Character, MonsterTracker, Exploration, GlobalLogic) + pure updates and queries
perception    pure parsing: item text and names, --More-- / (end) / (n of m) markers, tty text
engine        the frame machine: Frame/Cmd/Machine, Call/Return/Send, exception unwinding, operation limit
snapshot      codec of the state: Save/Load, type and static-table registries
nle, grid, internal/pyrt   NLE tables and types; 21x79 grids and BFS; Python emulation (exceptions, dict/set order, numpy RNG)
internal/botsim            tests only: a scripted game (a small map) whose terminal plays the keys of a frame
```

Each package imports only the ones below it. `internal/archtest` enforces the exact table (`allowed` in
`arch_test.go`). The key rules:

- `skills`, `tactics` and `strategy` never import `term` or `brain`. They press keys through the interface
  `world.Terminal` (`a.Term`), which `term.Terminal{}` implements. The driver sets it.
- `world`, `perception` and `assess` import nothing above themselves. `engine` knows nothing about NetHack
  beyond actions and observations, because it is generic in the state type `W`.
- `snapshot` is a leaf: every package registers its own frame and node types with it (`snapshot_types_gen.go`).

## One step

```
bot.py --obs--> cmd/aabot --> brain.Driver.Act(obs)
                                 engine.Machine.Feed(agent, obs)
                                   top frame = the term frame that sent the last key (stepF or rawKeyF)
                                     reads the observation: message and popup accumulation, step count
                                     more keys of this step? (iterator, --More--, menu, [yn], wish) -> Send, done
                                     else sense.UpdateState: character, inventory, monsters, #terrain,
                                          level, global logic (may press keys itself: nested steps),
                                          then the on_update watches (Agent.OnUpdate)
                                   Return to the caller frames: skill -> tactic body -> combinators -> main
                                   ... until some frame asks a.Term.Key(x) -> stepF -> Send(x)
                                 <-- action x
```

Decisions are made in `Node.Check` (plain Go, no keys pressed): the global strategy is a tree of nodes
(`strategy.GlobalStrategy`). Each `Preempt` layer checks its preempting nodes after every step, through a
watch in `Agent.OnUpdate`. If one applies, it interrupts everything below it with an `AgentChangeStrategy`
exception, which the machine unwinds frame by frame the way Python unwinds its stack.

## Where the state lives

- `world.Agent`: everything the bot knows. Levels, items, character, the last observations, message history,
  counters, flags of the atom operation and of the update phase, the numpy `RandomState`, the on_update watch
  list, and `M` (the machine).
- `engine.Machine.Stack`: the unfinished routines, outermost first. A frame is a struct holding the routine's
  locals and a `PC` (the place it stopped at). Between two actions the top frame is always the term frame that
  sent the last key.
- `brain.Driver`: `started`, and `dead` (after a fatal error the bot answers ESC until the game ends, like the
  Python adapter).
- Not state: `engine.Ctx` (what a frame sees while it runs: `W`, `M`, `Ret`, `Obs`), and the memo caches of
  pure functions (`sync.Map`: parsed item texts, glyph set unions).

A snapshot is `snapshot.Save(driver)`: all of the above, with shared pointers kept shared (one `*Item` in the
inventory and in a frame stays one object). Pointers into static tables (`*Obj`, `*Permonst`, `*GSet`) are
written as table indices. Typical size is 130-170 KB (max about 570 KB). Save takes about 2 ms and Load about
1 ms under Rosetta.

## Frames, nodes, conditions

A Python generator strategy (`@Strategy.wrap`, it yields once) becomes a **node**:

```go
type fooN struct{}                                   // stateless node: a value
func (fooN) Check(a *world.Agent) (bool, Frame) {    // the code up to the yield; must not press keys
    target, ok := ...
    if !ok { return false, nil }                     // yield False
    return true, &fooF{Target: target}               // yield True; the rest of the generator is a frame
}

type fooF struct { Target grid.Pos; PC int }         // the generator's locals after the yield + where it stopped
func (s *fooF) Resume(c *Ctx) Cmd {
    a := c.W
    switch s.PC {
    case 0:
        s.PC = 1
        return call(skills.GoTo(s.Target.Y, s.Target.X, skills.GoToOpts{}))   // a sub-routine
    case 1:
        if c.Ret ... { }                             // its result
        s.PC = 2
        return call(a.Term.Key(A_Command_SEARCH))    // one key (plus --More--, menus and the update)
    }
    return ret(nil)                                  // the generator's return value
}
```

- A frame is data only: no funcs, channels or pointers into the middle of other objects (the codec refuses
  them and names the path). Frame types stay unexported; the package exports a constructor (`skills.Pray()`).
- `Pred` is a condition as data (`xpAtLeast{7}`, `notHungry{}`), used by `flow.If` and `flow.Until`.
- Python `with atom_operation():` becomes `sense.Atom(body)`. `with panic_if_position_changes():` becomes
  `flow.PosGuard(body)`. `try/except` in a frame is `Catch(c, exc) bool` and `finally` is `Unwind(c)`; see
  `engine`.
- A generator passed to `step(action, gen())` becomes `a.Term.Dialog(first, genFrame)`. The generator frame
  answers each screen with `a.Term.RawKey(x)` / `a.Term.RawText(s)`. A plain key iterator is
  `a.Term.Keys(x, world.IterStr("..."))`.
- Stateful nodes (`Every` keeps a counter) are pointers and are built at the same moments as the Python
  objects (the strategy tables are functions, not variables).

## How to add

- **A skill** (a multi-step action): a frame type and a constructor in a new file in `skills/`. It presses keys
  only through `a.Term` and calls other skills with `call(...)`. It reads the state through `world`. Asserts go
  first, like the Python preconditions.
- **A tactic**: a node type in its own file in `tactics/`. `Check` decides without pressing keys and returns
  the body frame. Add it to a layer or an exploration table in `strategy/global.go`.
- **A milestone**: a row in `milestones()` in `strategy/global.go`, with a `Done` condition (`Pred`, in
  `strategy/conditions.go`) and a target level. **A preempting layer**: a row in `globalLayers()`. Lower rows are
  interrupted by higher ones.
- After adding a type that goes into an interface field: `go run ./tools/genreg <package dir>`. A test fails
  if you forget. A new kind of routine result (`ret(v)` of a new type) needs a line in `<package>/snapshot.go`.
  The snapshot tests find what is missing.
- **A test of a frame** (a skill, a tactic body) without NLE: `internal/botsim`. `botsim.Parse` draws a map
  (walls, doors, stairs, pits and holes, boulders, mimics, fountains, altars, objects, the hero), `g.NewAgent`
  gives an agent standing in it whose `a.Term` plays keys in the game, `g.Run(a, frame, maxKeys)` runs the frame
  and returns its result and exception. The game answers moves, Sokoban pushes, doors, kick, search and wait with
  a count, travel with its cursor, `#terrain`; a test scripts other commands with `g.Respond` (it sees the
  prompt the key answers). The update phase is off: the game writes glyphs, position, turn, cursor, message and
  the level memory itself, and runs the on_update watches. Examples: `skills/goto_test.go`,
  `strategy/soko_sim_test.go` (all 8 Sokoban levels of NetHack's `sokoban.des`), `tactics/altar_test.go`.

## Fast code

Speed-ups live in the utility packages, behind ordinary functions:
- `grid/vec.go`: numpy-style operations over whole grids: `InRanges` (behind `nle.Isin`/`AnyIn`: a glyph set
  is kept as up to 8 runs of ids), `I32Grid.Eq/Ne/Gt/Le/Lt/Ge/Mul/MulScalar/SetWhere/AppendPositive`,
  `I16Grid.Eq/SetWhere/CopyWhere`, `F32Grid` `FullF32/SubI32/AddWhere/SetWhere/GreaterEqual/Less/Eq/IsFinite/
  MaxWhere`. Each has a plain loop there and an AVX2 version in `vec_simd.go` (`simd/archsimd`, compiled with
  `GOEXPERIMENT=simd` on amd64, used when the CPU has AVX2: `grid.SIMDOn()`, also printed in the aabot stats
  line). Only element-wise vector operations, no FMA and no reordering, so the bits are those of the plain loop;
  `vec_test.go` compares the two on random grids, NaN and infinities included. Mixed float32/int arithmetic
  follows numpy: `SubI32` subtracts in float64 and rounds once.
- `grid/bits.go`: BoolGrid operations eight cells per word (32 per vector with SIMD). A Go bool is a 0/1 byte,
  so `&`/`|` on words are the cell-wise operations. `NeighborCount8/4` and `NeighborAny8/4` work the same way.
- `grid.BFS`: a padded flat queue; the same distances as the plain `BFSFlat`.
- `world.Level.SearchMasks()` is a derived cache: it is recomputed when the level maps it copied differ.
  It is tagged `snapshot:"-"`, so a restored bot starts with it empty.
- No PGO profile: with Go 1.27.1 every profile tried made the bot slower (`REFACTOR_PLAN.md` 6.1). To try one
  again, run games with `AABOT_CPUPROFILE=/out/cpu harness/run_remote.sh ...` and put
  `go tool pprof -proto <profiles>` into `cmd/aabot/default.pgo`.

The layers above them (tactics, strategy, the fight and item heuristics) stay plain Go.

Keep floating-point results bit for bit: every float32 step is rounded exactly like numpy does it.
Measure each change on blackbox against the previous commit, alternating the runs, because the machine's
speed drifts. `REFACTOR_PLAN.md` section 6 has the table of what each change gave.

## Determinism rules

- Never iterate a Go map where the order can matter: use `OMap` (insertion order) or `PySet` (CPython set
  order) from `internal/pyrt`, or sorted slices. `internal/archtest` rejects any `range` over a map.
- Randomness comes only from `a.Rng` (numpy `RandomState`, part of the state). The bot never reads the clock.
- No goroutines, channels or select outside `cmd/`. No writes to package-level variables. Pure memo caches
  (`sync.Map`) are the exception, and they must be safe for several bots in one process.
- Build the arena binary for linux/amd64 only (`harness/build.sh`): on arm64 Go fuses `a*b+c` into FMA, which
  changes rounding. On the mac, run tests as `GOARCH=amd64` (Rosetta).

## Checks

- Unit and architecture tests: `GOEXPERIMENT=simd GOARCH=amd64 go vet -composites=false ./... && GOEXPERIMENT=simd GOARCH=amd64 go test ./...`, and once without `GOEXPERIMENT` (the plain build).
- Equivalence on the recorded corpus (59 seeds, 2.2M steps, 2-4 min):
  - Command: `GOEXPERIMENT=simd GOARCH=amd64 go test ./equiv` (`-run 'Corpus/seed/1239$' -v` for one seed).
  - On a mismatch it prints the frame stack and the last messages.
  - Every 20000 steps the bot is also swapped for its restored snapshot.
  - The reference bot is tag `v2` (`harness/build.sh ref/v2 v2`): the literal port `v0-literal-port` plus the
    behavior fixes so far (the seed 1061 hang, `v1`; the Sokoban solver, `v2`). 47 seeds were recorded with
    `v0-literal-port`, 1061 with `v1`, 1139 and the 10 new seeds with `v2`; all play the same under `v2`.
  - 49 seeds are the `baseA` ones; 10 of the new seeds 3000-3299 (3019, 3031, 3046, 3066, 3070, 3103, 3164,
    3210, 3216, 3293) were added because they reach code the others do not (containers, untrap, follow the
    guard, wand engraving).
  - Scenarios (`equiv/scenario_test.go`): what a recorded game must show besides its keys (1139: three
    Sokoban levels solved, the top one given up).
  - The corpus is recorded with `harness/run_local.sh ... --record /corpus` (format in `internal/corpus`).
- Snapshots (about 6 min): `GOBOT_SNAP=1 GOEXPERIMENT=simd GOARCH=amd64 go test ./equiv -run 'Snapshot|Aabot' -v`. It runs:
  - a swap for the restored copy every 1000 steps, which must save back to the same bytes;
  - 24 seeds x 3 points, each restored in a new process, 2000 steps checked after each restore;
  - the real binary over the pipe (S in one aabot, L in another);
  - the watchdog.
- Speed: `harness/speed.sh [base-rev]` compares CPU per step with an older commit on the mac. In the arena
  environment, use `harness/run_remote.sh` / `harness/goinproc.py` on blackbox.
- Full games on blackbox: `run_remote.sh <label> <bot-dir> 16 <seeds> -- --ref /eo/v2` (the 49 `baseA`
  seeds against the traces of `v2`; `/eo/seeds_v2` has the new seeds 3000-3299). The AutoAscend crashes found on
  these seeds are fixed, one commit each (`REFACTOR_PLAN.md` 7, 7.1).
- Coverage: by the tests, `GOEXPERIMENT=simd GOARCH=amd64 go test -coverpkg=./... -coverprofile=<file> ./...`;
  by live games, a bot built with `GOFLAGS=-cover sh harness/build.sh <dir>` and run with
  `GOCOVERDIR=/out harness/run_remote.sh ...`, then `go tool covdata percent|textfmt -i=<dirs>`.

## Loops that never act

- Loops through frames: the machine counts operations between two actions. `brain.OpLimit` (2^16; real
  decisions take at most 64) turns such a loop into a `*engine.Hang`, and the bot then answers ESC.
- A loop inside one frame (plain Go) is invisible to the machine. For it, `cmd/aabot` keeps the Python
  adapter's action timeout (118 s, `AABOT_ACTION_TIMEOUT`): it answers ESC and abandons the stuck goroutine.
  A reset starts a fresh bot.
