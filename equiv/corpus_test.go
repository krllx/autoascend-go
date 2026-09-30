// Package equiv checks that the bot sends exactly the recorded actions on the observation corpus.
//
// Run as amd64 (on the mac under Rosetta: arm64 fuses a*b+c into FMA and changes rounding):
//
//	GOARCH=amd64 go test ./equiv
//	GOARCH=amd64 go test ./equiv -run 'Corpus/1239$' -v   # one seed
//
// Corpus: gobot/corpus/<seed>.nhc (or $GOBOT_CORPUS), recorded by harness/goinproc.py --record.
package equiv

import (
	"os"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"aago/brain"
	"aago/internal/corpus"
	"aago/nle"
)

func TestMain(m *testing.M) {
	os.Setenv("AABOT_QUIET", "1") // known AutoAscend crashes (asserts) would print long stacks
	os.Exit(m.Run())
}

func TestCorpus(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("run with GOARCH=amd64: the reference was recorded on amd64 (no FMA)")
	}
	dir := corpus.Dir()
	seeds := corpus.Seeds(dir)
	if len(seeds) == 0 {
		t.Skip("no corpus (gobot/corpus or $GOBOT_CORPUS)")
	}
	var steps, maxOps atomic.Int64
	start := time.Now()
	t.Run("seed", func(t *testing.T) {
		for _, seed := range seeds {
			t.Run(strconv.Itoa(seed), func(t *testing.T) {
				t.Parallel()
				n, ops := replay(t, corpus.Path(dir, seed), seed)
				steps.Add(int64(n))
				for m := maxOps.Load(); int64(ops) > m && !maxOps.CompareAndSwap(m, int64(ops)); m = maxOps.Load() {
				}
			})
		}
	})
	t.Logf("%d seeds, %d steps, %.1f s; at most %d frame operations between two actions (limit %d)",
		len(seeds), steps.Load(), time.Since(start).Seconds(), maxOps.Load(), brain.OpLimit)
}

// replay feeds the recorded observations to a fresh bot and fails at the first action that differs. Every
// 20000 steps (at a seed-dependent phase) the bot is also swapped for its restored snapshot, a cheap guard for
// state that snapshots would lose (the full snapshot tests: GOBOT_SNAP=1, see snapshot_test.go).
func replay(t *testing.T, path string, seed int) (steps, maxOps int) {
	r, err := corpus.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if r.ObsSize() != nle.ObsWireSize() {
		t.Fatalf("observation size %d, want %d", r.ObsSize(), nle.ObsWireSize())
	}
	r.Close()
	const every = 20000
	phase := seed * 7919 % every
	var st snapStats
	steps = replayWith(t, path, func(step int, d *brain.Driver) *brain.Driver {
		if d.Agent != nil {
			maxOps = max(maxOps, d.Agent.M.MaxOps)
		}
		if step%every == phase && !d.Dead() {
			return roundTrip(t, d, &st)
		}
		return d
	})
	return steps, maxOps
}
