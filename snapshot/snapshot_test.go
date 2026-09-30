package snapshot

import (
	"strings"
	"sync"
	"testing"
)

type item struct {
	name  string
	count int
	next  *item // cycles allowed
}

type table struct{ v int }

var statics = [3]table{{1}, {2}, {3}}

func init() {
	for i := range statics {
		RegisterStatic(&statics[i])
	}
	Register(&frameA{}, frameB{}, 0)
}

type frame interface{ Name() string }

type frameA struct {
	PC     int
	Target *item
}

func (f *frameA) Name() string { return "a" }

type frameB struct{ Steps [3]int16 }

func (f frameB) Name() string { return "b" }

type state struct {
	items   []*item
	hand    *item // alias of items[1]
	grid    [4][5]bool
	byPos   map[[2]int]*item
	counts  map[string]int
	same    map[string]int // alias of counts
	stack   []frame
	ret     any
	tab     *table // static
	below   []*item
	belowAl []*item // same slice as below
	empty   []int
	nilS    []int
	skipped func() `snapshot:"-"`
}

func build() *state {
	a := &item{name: "dagger", count: 2}
	b := &item{name: "apple", count: 1}
	a.next = b
	b.next = a
	s := &state{items: []*item{a, b}, hand: b, counts: map[string]int{"x": 1, "y": 2},
		byPos: map[[2]int]*item{{1, 2}: a}, tab: &statics[2], empty: []int{}, skipped: func() {}}
	s.same = s.counts
	s.grid[3][4] = true
	s.below = []*item{b}
	s.belowAl = s.below
	s.stack = []frame{&frameA{PC: 3, Target: a}, frameB{Steps: [3]int16{1, 2, 3}}}
	s.ret = 7
	return s
}

func TestRoundTrip(t *testing.T) {
	s := build()
	b, err := Save(s)
	if err != nil {
		t.Fatal(err)
	}
	var r state
	if err := Load(b, &r); err != nil {
		t.Fatal(err)
	}
	if r.hand != r.items[1] || r.items[0].next != r.items[1] || r.items[1].next != r.items[0] {
		t.Fatal("pointer aliasing / cycles lost")
	}
	if r.items[0] == s.items[0] {
		t.Fatal("not a copy")
	}
	r.counts["z"] = 3
	if r.same["z"] != 3 {
		t.Fatal("map aliasing lost")
	}
	if &r.below[0] != &r.belowAl[0] {
		t.Fatal("slice aliasing lost")
	}
	if r.tab != &statics[2] {
		t.Fatal("static pointer not restored")
	}
	if !r.grid[3][4] || r.byPos[[2]int{1, 2}] != r.items[0] {
		t.Fatal("arrays / map keys")
	}
	fa, ok := r.stack[0].(*frameA)
	if !ok || fa.PC != 3 || fa.Target != r.items[0] || r.stack[1].(frameB).Steps != [3]int16{1, 2, 3} {
		t.Fatal("interfaces")
	}
	if r.ret != 7 || r.empty == nil || len(r.empty) != 0 || r.nilS != nil || r.skipped != nil {
		t.Fatal("scalars in interfaces / empty vs nil / skipped")
	}
	// equal states give equal bytes
	b2, err := Save(&r)
	if err != nil {
		t.Fatal(err)
	}
	r.counts["z"] = 0
	delete(r.counts, "z")
	b3, _ := Save(&r)
	if string(b3) != string(b) || len(b2) <= len(b) {
		t.Fatalf("bytes differ after a round trip (%d vs %d)", len(b3), len(b))
	}
}

type bad struct {
	f    func()
	ch   chan int
	mu   sync.Mutex
	arr  *[3]int
	elem *int // points into arr
	u    unregistered
}

type unregistered struct{ x any }

func TestRejects(t *testing.T) {
	arr := &[3]int{1, 2, 3}
	v := &bad{f: func() {}, ch: make(chan int), arr: arr, elem: &arr[1], u: unregistered{x: &item{}}}
	_, err := Save(v)
	if err == nil {
		t.Fatal("no error")
	}
	for _, want := range []string{".f: func()", ".ch: chan int", ".mu: struct sync.Mutex", ".elem: points into",
		".u.x: type *aago/snapshot.item is not registered"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q:\n%v", want, err)
		}
	}
}

func TestClone(t *testing.T) {
	s := build()
	c, err := Clone(s)
	if err != nil {
		t.Fatal(err)
	}
	c.items[0].count = 99
	if s.items[0].count != 2 || c.items[0].next.next.count != 99 {
		t.Fatal("clone shares or loses objects")
	}
}
