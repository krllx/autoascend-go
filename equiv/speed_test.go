package equiv

import (
	"os"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"aago/brain"
	"aago/internal/corpus"
	"aago/nle"
)

// TestSpeed replays a few seeds one after another and reports CPU time per step (bot + corpus decoding).
// Enabled by GOBOT_SPEED=seed,seed,... (e.g. GOBOT_SPEED=0,1001,1047); harness/speed.sh compares two commits.
// Self-contained on purpose: speed.sh copies it into older commits (renaming the driver package if needed).
func TestSpeed(t *testing.T) {
	spec := os.Getenv("GOBOT_SPEED")
	if spec == "" {
		t.Skip("set GOBOT_SPEED=<seeds>")
	}
	dir := corpus.Dir()
	cpu := func() float64 {
		var ru syscall.Rusage
		syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
		return float64(ru.Utime.Sec+ru.Stime.Sec) + float64(ru.Utime.Usec+ru.Stime.Usec)/1e6
	}
	c0 := cpu()
	steps := 0
	for _, s := range strings.Split(spec, ",") {
		seed, err := strconv.Atoi(s)
		if err != nil {
			t.Fatal(err)
		}
		r, err := corpus.Open(corpus.Path(dir, seed))
		if err != nil {
			t.Fatal(err)
		}
		d := brain.NewDriver()
		for {
			b, want, ok, err := r.Next()
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				break
			}
			if r.Step == 0 {
				d.Reset()
			}
			if got := d.Act(nle.ObsFromWire(b)); got != want {
				t.Fatalf("seed %d step %d: action %d, recorded %d", seed, r.Step, got, want)
			}
			steps++
		}
		r.Close()
	}
	t.Logf("SPEED %d steps %.1f us/step", steps, 1e6*(cpu()-c0)/float64(steps))
}
