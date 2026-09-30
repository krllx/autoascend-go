package equiv

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"aago/brain"
	"aago/internal/corpus"
	"aago/nle"
	"aago/snapshot"
)

// snapStats collects snapshot sizes and times over a test.
type snapStats struct {
	sync.Mutex
	n          int
	bytes      []int
	save, load time.Duration
}

func (s *snapStats) add(size int, save, load time.Duration) {
	s.Lock()
	defer s.Unlock()
	s.n++
	s.bytes = append(s.bytes, size)
	s.save += save
	s.load += load
}

func (s *snapStats) String() string {
	if s.n == 0 {
		return "no snapshots"
	}
	b := append([]int(nil), s.bytes...)
	sort.Ints(b)
	sum := 0
	for _, x := range b {
		sum += x
	}
	return fmt.Sprintf("%d snapshots: size median %d KB, max %d KB, mean %d KB; save %.2f ms, load %.2f ms (mean)",
		s.n, b[len(b)/2]>>10, b[len(b)-1]>>10, sum/s.n>>10,
		float64(s.save.Microseconds())/float64(s.n)/1e3, float64(s.load.Microseconds())/float64(s.n)/1e3)
}

// roundTrip replaces the bot with a copy restored from its snapshot; the copy must save to the same bytes.
func roundTrip(t *testing.T, d *brain.Driver, st *snapStats) *brain.Driver {
	t0 := time.Now()
	data, err := d.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	t1 := time.Now()
	n := brain.NewDriver()
	if err := n.Restore(data); err != nil {
		t.Fatal(err)
	}
	t2 := time.Now()
	st.add(len(data), t1.Sub(t0), t2.Sub(t1))
	again, err := n.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, again) {
		i := 0
		for i < min(len(data), len(again)) && data[i] == again[i] {
			i++
		}
		t.Fatalf("the restored bot saves to different bytes (%d vs %d), first at %d:\n  saved:    %s\n  restored: %s",
			len(again), len(data), i, snapshot.Explain(d, i), snapshot.Explain(n, i))
	}
	return n
}

// snapTests gates the long snapshot tests (several minutes): GOBOT_SNAP=1.
func snapTests(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("run with GOARCH=amd64")
	}
	if os.Getenv("GOBOT_SNAP") == "" {
		t.Skip("set GOBOT_SNAP=1")
	}
}

func snapEvery() int {
	if v, err := strconv.Atoi(os.Getenv("GOBOT_SNAP_EVERY")); err == nil && v > 0 {
		return v
	}
	return 1000
}

// TestSnapshotRoundTrip replays the corpus and every GOBOT_SNAP_EVERY steps (default 1000, at a different
// phase for every seed) swaps the bot for its restored snapshot. The actions must stay the recorded ones.
func TestSnapshotRoundTrip(t *testing.T) {
	snapTests(t)
	dir := corpus.Dir()
	seeds := corpus.Seeds(dir)
	if len(seeds) == 0 {
		t.Skip("no corpus")
	}
	every := snapEvery()
	var st snapStats
	t.Run("seed", func(t *testing.T) {
		for _, seed := range seeds {
			t.Run(strconv.Itoa(seed), func(t *testing.T) {
				t.Parallel()
				phase := seed % every
				replayWith(t, corpus.Path(dir, seed), func(step int, d *brain.Driver) *brain.Driver {
					if step%every == phase && !d.Dead() {
						return roundTrip(t, d, &st)
					}
					return d
				})
			})
		}
	})
	t.Log(st.String())
}

// replayWith is replay with a hook called after every action (step = index of the observation answered).
func replayWith(t *testing.T, path string, after func(step int, d *brain.Driver) *brain.Driver) int {
	r, err := corpus.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	d := brain.NewDriver()
	first := true
	for {
		b, want, ok, err := r.Next()
		if err != nil {
			t.Fatalf("step %d: %v", r.Step, err)
		}
		if !ok {
			return r.Step + 1
		}
		if first {
			d.Reset()
			first = false
		}
		if got := d.Act(nle.ObsFromWire(b)); got != want {
			t.Fatalf("step %d: action %d, recorded %d\n%s", r.Step, got, want, d.Agent.DebugString())
		}
		d = after(r.Step, d)
	}
}

// countSteps is the number of answered observations in a corpus file.
func countSteps(t *testing.T, path string) int {
	r, err := corpus.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for {
		_, _, ok, err := r.Next()
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			return r.Step
		}
	}
}

// TestSnapshotProcess saves the bot at several points k of a game and restores every snapshot in a new process
// (this test binary, TestSnapshotChild), which must answer the next M recorded observations with the recorded
// actions. GOBOT_SNAP_SEEDS limits the number of seeds (default 24), GOBOT_SNAP_M the steps checked (2000).
func TestSnapshotProcess(t *testing.T) {
	snapTests(t)
	if os.Getenv("GOBOT_SNAP_CHILD") != "" {
		t.Skip("child")
	}
	dir := corpus.Dir()
	seeds := corpus.Seeds(dir)
	if len(seeds) == 0 {
		t.Skip("no corpus")
	}
	nseeds := 24
	if v, err := strconv.Atoi(os.Getenv("GOBOT_SNAP_SEEDS")); err == nil && v > 0 {
		nseeds = v
	}
	seeds = seeds[:min(nseeds, len(seeds))]
	m := 2000
	if v, err := strconv.Atoi(os.Getenv("GOBOT_SNAP_M")); err == nil && v > 0 {
		m = v
	}
	tmp := t.TempDir()
	var st snapStats
	var mu sync.Mutex
	checked := 0
	t.Run("seed", func(t *testing.T) {
		for _, seed := range seeds {
			t.Run(strconv.Itoa(seed), func(t *testing.T) {
				t.Parallel()
				path := corpus.Path(dir, seed)
				n := countSteps(t, path)
				// k = number of actions sent before the snapshot: early, middle and late in the game
				ks := map[int]bool{}
				for _, f := range []float64{0.1, 0.5, 0.9} {
					if k := int(float64(n) * f); k > 0 {
						ks[k] = true
					}
				}
				files := map[int]string{}
				replayWith(t, path, func(step int, d *brain.Driver) *brain.Driver {
					if ks[step+1] {
						t0 := time.Now()
						data, err := d.Snapshot()
						if err != nil {
							t.Fatal(err)
						}
						st.add(len(data), time.Since(t0), 0)
						f := filepath.Join(tmp, fmt.Sprintf("%d-%d.snap", seed, step+1))
						if err := os.WriteFile(f, data, 0o644); err != nil {
							t.Fatal(err)
						}
						files[step+1] = f
					}
					return d
				})
				for _, k := range sortedKeys(files) {
					cmd := exec.Command(os.Args[0], "-test.run=^TestSnapshotChild$", "-test.v")
					cmd.Env = append(os.Environ(), fmt.Sprintf("GOBOT_SNAP_CHILD=%s;%d;%d;%d", files[k], seed, k, m))
					out, err := cmd.CombinedOutput()
					if err != nil || !strings.Contains(string(out), "CHILD OK") {
						t.Fatalf("k=%d: child failed: %v\n%s", k, err, out)
					}
					mu.Lock()
					checked++
					mu.Unlock()
				}
			})
		}
	})
	t.Logf("%d restores in a new process, %d steps each; %s", checked, m, st.String())
}

func sortedKeys(m map[int]string) []int {
	var ks []int
	for k := range m {
		ks = append(ks, k)
	}
	sort.Ints(ks)
	return ks
}

// TestSnapshotChild is the new process of TestSnapshotProcess: it skips the first k observations, restores
// the snapshot and checks the next M actions.
func TestSnapshotChild(t *testing.T) {
	spec := os.Getenv("GOBOT_SNAP_CHILD")
	if spec == "" {
		t.Skip("run by TestSnapshotProcess")
	}
	p := strings.Split(spec, ";")
	file := p[0]
	seed, _ := strconv.Atoi(p[1])
	k, _ := strconv.Atoi(p[2])
	m, _ := strconv.Atoi(p[3])
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	d := brain.NewDriver()
	t0 := time.Now()
	if err := d.Restore(data); err != nil {
		t.Fatal(err)
	}
	load := time.Since(t0)
	r, err := corpus.Open(corpus.Path(corpus.Dir(), seed))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	done := 0
	for {
		b, want, ok, err := r.Next()
		if err != nil {
			t.Fatal(err)
		}
		if !ok || done == m {
			break
		}
		if r.Step < k {
			continue
		}
		if got := d.Act(nle.ObsFromWire(b)); got != want {
			t.Fatalf("seed %d, k %d, step %d: action %d, recorded %d\n%s", seed, k, r.Step, got, want, d.Agent.DebugString())
		}
		done++
	}
	t.Logf("CHILD OK: %d steps after restore (load %.2f ms, %d bytes)", done, float64(load.Microseconds())/1e3, len(data))
}
