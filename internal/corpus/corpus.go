// Package corpus reads recorded games: the observations a reference bot saw and the actions it sent.
//
// File format (written by harness/goinproc.py --record): b"NHC1", uint32 LE observation size, then one zlib
// stream of obs0, action0, obs1, action1, ..., obs_n. obs0 is raw, every later observation is XOR the previous
// one; observations are packed exactly like harness/bot.py sends them; actions are uint16 LE action indices.
package corpus

import (
	"bufio"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type Reader struct {
	f       *os.File
	z       io.ReadCloser
	cur     []byte
	started bool
	Step    int // index of the observation returned by the last Next
}

func Open(path string) (*Reader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	var hdr [8]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil || string(hdr[:4]) != "NHC1" {
		f.Close()
		return nil, fmt.Errorf("%s: not a corpus file", path)
	}
	n := binary.LittleEndian.Uint32(hdr[4:])
	z, err := zlib.NewReader(bufio.NewReaderSize(f, 1<<16))
	if err != nil {
		f.Close()
		return nil, err
	}
	return &Reader{f: f, z: z, cur: make([]byte, n), Step: -1}, nil
}

// ObsSize is the size of one observation in bytes.
func (r *Reader) ObsSize() int { return len(r.cur) }

// Next returns the next observation and the action the reference bot answered it with. ok is false after the
// last answered observation (the final, terminal observation has no action). obs is valid until the next call.
func (r *Reader) Next() (obs []byte, action int, ok bool, err error) {
	if !r.started {
		r.started = true
		if _, err := io.ReadFull(r.z, r.cur); err != nil {
			return nil, 0, false, err
		}
	} else {
		d := make([]byte, len(r.cur))
		if _, err := io.ReadFull(r.z, d); err != nil {
			return nil, 0, false, err
		}
		for i, b := range d {
			r.cur[i] ^= b
		}
	}
	var a [2]byte
	if _, err := io.ReadFull(r.z, a[:]); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, 0, false, nil
		}
		return nil, 0, false, err
	}
	r.Step++
	return r.cur, int(binary.LittleEndian.Uint16(a[:])), true, nil
}

func (r *Reader) Close() error {
	r.z.Close()
	return r.f.Close()
}

// Dir returns the corpus directory: $GOBOT_CORPUS, else <module root>/corpus found upwards from the working
// directory ("" if there is none).
func Dir() string {
	if d := os.Getenv("GOBOT_CORPUS"); d != "" {
		return d
	}
	wd, _ := os.Getwd()
	for d := wd; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			c := filepath.Join(d, "corpus")
			if _, err := os.Stat(c); err == nil {
				return c
			}
			return ""
		}
		if filepath.Dir(d) == d {
			return ""
		}
	}
}

// Seeds lists the seeds recorded in dir, in increasing order.
func Seeds(dir string) []int {
	ms, _ := filepath.Glob(filepath.Join(dir, "*.nhc"))
	var seeds []int
	for _, m := range ms {
		if s, err := strconv.Atoi(strings.TrimSuffix(filepath.Base(m), ".nhc")); err == nil {
			seeds = append(seeds, s)
		}
	}
	sort.Ints(seeds)
	return seeds
}

func Path(dir string, seed int) string { return filepath.Join(dir, fmt.Sprintf("%d.nhc", seed)) }
