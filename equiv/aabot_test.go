package equiv

import (
	"bufio"
	"encoding/binary"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"aago/internal/corpus"
	"aago/nle"
)

// aabotProc is cmd/aabot behind its pipe protocol (the one harness/bot.py speaks).
type aabotProc struct {
	cmd *exec.Cmd
	in  io.WriteCloser
	out *bufio.Reader
}

func startAabot(t *testing.T, bin string, env ...string) *aabotProc {
	cmd := exec.Command(bin)
	cmd.Env = append(append(os.Environ(), "AABOT_QUIET=1"), env...)
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return &aabotProc{cmd, in, bufio.NewReader(out)}
}

func (p *aabotProc) send(t *testing.T, b ...[]byte) {
	for _, x := range b {
		if _, err := p.in.Write(x); err != nil {
			t.Fatal(err)
		}
	}
}

func (p *aabotProc) read(t *testing.T, n int) []byte {
	b := make([]byte, n)
	if _, err := io.ReadFull(p.out, b); err != nil {
		t.Fatal(err)
	}
	return b
}

func (p *aabotProc) act(t *testing.T, obs []byte) int {
	p.send(t, []byte{'A'}, obs)
	return int(binary.LittleEndian.Uint16(p.read(t, 2)))
}

func (p *aabotProc) close() {
	p.in.Write([]byte{'C'})
	p.in.Close()
	p.cmd.Wait()
}

// TestAabotSnapshotPipe drives the real binary: one aabot plays to the middle of a game and answers 'S', a
// second aabot is started, gets 'L' with that state and must continue with the recorded actions.
func TestAabotSnapshotPipe(t *testing.T) {
	snapTests(t)
	dir := corpus.Dir()
	seeds := corpus.Seeds(dir)
	if len(seeds) == 0 {
		t.Skip("no corpus")
	}
	bin := buildAabot(t)
	for _, seed := range seeds[:min(3, len(seeds))] {
		path := corpus.Path(dir, seed)
		k := countSteps(t, path) / 2
		r, err := corpus.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		a := startAabot(t, bin)
		var b *aabotProc
		checked := 0
		for checked < 1000 {
			obs, want, ok, err := r.Next()
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				break
			}
			if r.Step == 0 {
				a.send(t, []byte{'R'}, obs)
			}
			if r.Step == k {
				a.send(t, []byte{'S'})
				n := binary.LittleEndian.Uint32(a.read(t, 4))
				if n == 0 {
					t.Fatalf("seed %d: aabot could not save its state", seed)
				}
				data := a.read(t, int(n))
				a.close()
				b = startAabot(t, bin)
				var hdr [5]byte
				hdr[0] = 'L'
				binary.LittleEndian.PutUint32(hdr[1:], n)
				b.send(t, hdr[:], data)
				if st := b.read(t, 1); st[0] != 1 {
					t.Fatalf("seed %d: restore failed", seed)
				}
			}
			p := a
			if b != nil {
				p = b
				checked++
			}
			if got := p.act(t, obs); got != want {
				t.Fatalf("seed %d, step %d (snapshot at %d): action %d, recorded %d", seed, r.Step, k, got, want)
			}
		}
		if b != nil {
			b.close()
		}
		r.Close()
		t.Logf("seed %d: restored at step %d in a new aabot, %d actions match", seed, k, checked)
	}
}

func buildAabot(t *testing.T) string {
	bin := filepath.Join(t.TempDir(), "aabot")
	build := exec.Command("go", "build", "-trimpath", "-o", bin, "aago/cmd/aabot")
	build.Env = append(os.Environ(), "GOARCH=amd64")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

// TestAabotWatchdog: with a timeout of ~0 the watchdog takes over the first action it sees running (it checks
// once a second). The protocol must stay in step: recorded actions up to that point, then only ESC, and a
// reset starts a fresh bot that plays the recorded game again.
func TestAabotWatchdog(t *testing.T) {
	snapTests(t)
	dir := corpus.Dir()
	seeds := corpus.Seeds(dir)
	if len(seeds) == 0 {
		t.Skip("no corpus")
	}
	p := startAabot(t, buildAabot(t), "AABOT_ACTION_TIMEOUT=1e-9")
	defer p.close()
	esc := nle.ActionToIndex[nle.A_Command_ESC]
	for round := 0; round < 2; round++ {
		r, err := corpus.Open(corpus.Path(dir, seeds[0]))
		if err != nil {
			t.Fatal(err)
		}
		hungAt, start := -1, time.Now()
		for time.Since(start) < 10*time.Second && (hungAt < 0 || r.Step < hungAt+100) {
			obs, want, ok, err := r.Next()
			if err != nil || !ok {
				t.Fatal("game over before the watchdog fired", err)
			}
			if r.Step == 0 {
				p.send(t, []byte{'R'}, obs)
			}
			got := p.act(t, obs)
			if hungAt < 0 && got != want {
				if got != esc {
					t.Fatalf("round %d, step %d: action %d, recorded %d", round, r.Step, got, want)
				}
				hungAt = r.Step
			} else if hungAt >= 0 && got != esc {
				t.Fatalf("round %d, step %d: action %d after the watchdog fired at %d", round, r.Step, got, hungAt)
			}
		}
		r.Close()
		if hungAt < 0 {
			t.Fatalf("round %d: the watchdog never fired", round)
		}
		t.Logf("round %d: the watchdog fired at step %d; ESC after that", round, hungAt)
	}
}
