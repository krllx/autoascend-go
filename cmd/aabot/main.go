// aabot: AutoAscend Go port behind a binary pipe protocol (see harness/bot.py).
//
// stdin messages: 'R' + obs (reset), 'A' + obs (act -> 2 bytes LE action index on stdout), 'C' (close),
// 'S' (snapshot -> uint32 LE length + the bot state; length 0 if it cannot be saved), 'L' + uint32 LE length +
// state (restore a snapshot of the same build -> 1 byte: 1 ok, 0 failed).
package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"aago/brain"
	"aago/grid"
	"aago/nle"
)

// readObs decodes the fixed little-endian layout written by harness/bot.py straight into the arrays.
func readObs(r io.Reader, o *nle.Obs) error {
	for _, p := range o.WireParts() {
		if _, err := io.ReadFull(r, p); err != nil {
			return err
		}
	}
	return nil
}

// watchdog is the action timeout of the Python adapter (118 s, AABOT_ACTION_TIMEOUT). The machine's operation
// limit already stops routines that cycle through frames without acting; a loop inside one frame is plain Go
// code it cannot see. When an action takes longer than the timeout, the watchdog answers ESC for it, the stuck
// goroutine is abandoned (like the Python agent thread) and a new server goroutine answers ESC until the next
// reset.
type watchdog struct {
	timeout time.Duration
	acting  atomic.Int64 // generation of the server that is inside Act, 0 when none
	started atomic.Int64 // UnixNano when that Act started
}

// server answers the protocol messages; gen tells a live server from an abandoned one.
type server struct {
	in   *bufio.Reader
	out  *bufio.Writer
	d    *brain.Driver
	hung bool // the current game hung: ESC until the next reset
	gen  int64
	w    *watchdog
	done chan struct{}
}

var escIndex = nle.ActionToIndex[nle.A_Command_ESC]

func (w *watchdog) run(s *server) {
	for range time.Tick(time.Second) {
		g := w.acting.Load()
		if g == 0 || time.Since(time.Unix(0, w.started.Load())) < w.timeout || !w.acting.CompareAndSwap(g, 0) {
			continue
		}
		fmt.Fprintf(os.Stderr, "[aabot] no action for %v: answering ESC until the next reset\n", w.timeout)
		s = &server{in: s.in, out: s.out, hung: true, gen: g + 1, w: w, done: s.done}
		s.reply(escIndex)
		go s.serve()
	}
}

func (s *server) reply(action int) {
	var buf [2]byte
	binary.LittleEndian.PutUint16(buf[:], uint16(action))
	s.out.Write(buf[:])
	s.out.Flush()
}

func (s *server) serve() {
	for {
		tag, err := s.in.ReadByte()
		if err != nil {
			break
		}
		switch tag {
		case 'R', 'A':
			// a fresh buffer per step: update() frames suspended up the recursion (nested steps inside action
			// generators) keep reading their own observation, like Python's per-step dicts. Obs has no pointers,
			// so the GC never scans it.
			obs := new(nle.Obs)
			if err := readObs(s.in, obs); err != nil {
				fmt.Fprintln(os.Stderr, "read obs:", err)
				os.Exit(1)
			}
			if tag == 'R' {
				if s.hung {
					s.d, s.hung = brain.NewDriver(), false
				}
				s.d.Reset()
				continue
			}
			if s.hung {
				s.reply(escIndex)
				continue
			}
			s.w.started.Store(time.Now().UnixNano())
			s.w.acting.Store(s.gen)
			action := s.d.Act(obs)
			if !s.w.acting.CompareAndSwap(s.gen, 0) {
				select {} // the watchdog has answered for us and handed the pipe to a new server
			}
			s.reply(action)
		case 'S':
			var data []byte
			if !s.hung {
				if data, err = s.d.Snapshot(); err != nil {
					fmt.Fprintln(os.Stderr, "[aabot] snapshot:", err)
					data = nil
				}
			}
			var n [4]byte
			binary.LittleEndian.PutUint32(n[:], uint32(len(data)))
			s.out.Write(n[:])
			s.out.Write(data)
			s.out.Flush()
		case 'L':
			var n [4]byte
			if _, err := io.ReadFull(s.in, n[:]); err != nil {
				fmt.Fprintln(os.Stderr, "read snapshot:", err)
				os.Exit(1)
			}
			data := make([]byte, binary.LittleEndian.Uint32(n[:]))
			if _, err := io.ReadFull(s.in, data); err != nil {
				fmt.Fprintln(os.Stderr, "read snapshot:", err)
				os.Exit(1)
			}
			d := brain.NewDriver()
			ok := byte(1)
			if err := d.Restore(data); err != nil {
				fmt.Fprintln(os.Stderr, "[aabot] restore:", err)
				ok = 0
			} else {
				s.d, s.hung = d, false
			}
			s.out.WriteByte(ok)
			s.out.Flush()
		case 'C':
			dead := s.hung || s.d.Dead()
			if !s.hung {
				s.d.Close()
			}
			var ru syscall.Rusage
			syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
			cpu := float64(ru.Utime.Sec+ru.Stime.Sec) + float64(ru.Utime.Usec+ru.Stime.Usec)/1e6
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			fmt.Fprintf(os.Stderr, "[aabot] cpu_s=%.3f dead=%v simd=%v total_alloc_mb=%d mallocs=%d num_gc=%d gc_cpu_frac=%.4f heap_mb=%d\n",
				cpu, dead, grid.SIMDOn(), ms.TotalAlloc>>20, ms.Mallocs, ms.NumGC, ms.GCCPUFraction, ms.HeapAlloc>>20)
			if path := os.Getenv("AABOT_MEMPROFILE"); path != "" {
				if f, err := os.Create(fmt.Sprintf("%s.%d", path, time.Now().UnixNano())); err == nil {
					pprof.Lookup("allocs").WriteTo(f, 0)
					f.Close()
				}
			}
			close(s.done)
			return
		default:
			fmt.Fprintln(os.Stderr, "bad tag", tag)
			os.Exit(1)
		}
	}
	close(s.done)
}

func main() {
	if path := os.Getenv("AABOT_CPUPROFILE"); path != "" {
		f, err := os.Create(fmt.Sprintf("%s.%d", path, time.Now().UnixNano()))
		if err == nil {
			pprof.StartCPUProfile(f)
			defer pprof.StopCPUProfile()
		}
	}
	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(400)
	}
	w := &watchdog{timeout: 118 * time.Second}
	if v, err := strconv.ParseFloat(os.Getenv("AABOT_ACTION_TIMEOUT"), 64); err == nil {
		w.timeout = time.Duration(v * float64(time.Second))
	}
	s := &server{in: bufio.NewReaderSize(os.Stdin, 1<<16), out: bufio.NewWriter(os.Stdout), d: brain.NewDriver(),
		gen: 1, w: w, done: make(chan struct{})}
	go w.run(s)
	go s.serve()
	<-s.done
}
