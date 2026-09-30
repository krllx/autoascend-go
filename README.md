# gobot

A Go port of [AutoAscend](https://github.com/maciej-sypetkowski/autoascend), the NetHack 3.6.6 bot that won the
NeurIPS 2021 NetHack Challenge, for the [NetHack Learning Environment](https://github.com/NetHack-LE/nle).

- **Same play as the original.** The port was checked action for action against the Python bot on recorded games:
  for the same seed it presses the same keys. Since then AutoAscend bugs are fixed on purpose, one commit each
  with the list of games it changes: a hang, the Sokoban solver and every crash found on 349 test seeds, see
  `REFACTOR_PLAN.md`, sections 7 and 7.1.
- **All state is plain data.** The bot's routines are explicit frames on a slice instead of a call stack, so the
  whole bot is one structure: it can be saved to a file mid-game and restored in another process.
- **Faster.** Several times less CPU per step than the Python bot; the grid kernels use AVX2 through
  `simd/archsimd` when available, with plain loops giving the same results bit for bit.

## Documentation

- `ARCHITECTURE.md` - packages and layers, the frame machine, how state and snapshots work, checks, how to add code.
- `PORTING.md` - rules used to port the Python code (Python semantics that had to be kept).
- `REFACTOR_PLAN.md` - the rework log (in Russian): from the literal port to explicit frames, speed-ups, fixes.

## Build and test

Go 1.27.1 (pinned by `toolchain` in `go.mod`, the go command fetches it). The bot is built for linux/amd64 only:
on arm64 Go fuses `a*b+c` into FMA and rounding no longer matches the Python bot.

```bash
GOEXPERIMENT=simd GOARCH=amd64 go vet -composites=false ./...
GOEXPERIMENT=simd GOARCH=amd64 go test ./...
```

Run the tests once without `GOEXPERIMENT` too: the plain build compiles other files. On an Apple Silicon Mac the
amd64 tests run under Rosetta.

- `harness/build.sh <dir>` builds the arena bot: `<dir>/aabot` plus the `bot.py` shim that talks to it over a pipe.
- `harness/run_local.sh <label> <bot-dir> <parallel> <seeds...>` plays seeds in the
  [NetHackers](https://github.com/dunnolab/nethackers) arena image under Docker. It pins the game clock with an
  `LD_PRELOAD` shim, `localcheck/faketime.so` (`time()` returns `$FAKE_EPOCH`), which is not in the repository.
- Equivalence tests (`./equiv`) replay a corpus of recorded observations (`corpus/<seed>.nhc`, not in the
  repository, a few hundred MB). Record it with `harness/run_local.sh <label> <bot-dir> 6 <seeds> -- --record
  /corpus`; without a corpus these tests are skipped.
- `internal/botsim` is a small scripted map game for unit tests of bot routines without NLE (movement, doors,
  boulders, Sokoban). The long random Sokoban sweep runs with `GOBOT_SOKO_TRIALS=<n>`.

This repository is a snapshot of the development history. Commits and tags named in the documents
(`v0-literal-port`, `v1`, `v2`) and the reference traces refer to that history and are not included; the code here is
`v2` plus the fixes of section 7.1.

## License

gobot contains data derived from NetHack (object and monster tables, the Sokoban levels), so it is distributed
under the NetHack General Public License, see `LICENSE`. AutoAscend is MIT-licensed; its notice and the list of
NetHack-derived files are in `NOTICE`.
