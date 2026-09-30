package engine

import (
	"reflect"
	"testing"

	"aago/internal/pyrt"
	"aago/nle"
)

// toy world: a log of events
type world struct{ log []string }

type C = Ctx[*world]

// sendN sends actions 1..N and returns the sum of the Misc[0] values of the observations.
type sendN struct {
	N, I, Sum int
}

func (s *sendN) Resume(c *C) Cmd[*world] {
	if c.Obs != nil {
		s.Sum += int(c.Obs.Misc[0])
	}
	if s.I < s.N {
		s.I++
		return Send[*world](nle.Action(s.I))
	}
	return Return[*world](s.Sum)
}

// seq calls its children one by one and records their results in the world log.
type seq struct {
	Kids []Frame[*world]
	I    int
}

func (s *seq) Resume(c *C) Cmd[*world] {
	if s.I > 0 {
		c.W.log = append(c.W.log, "ret", string(rune('0'+c.Ret.(int)/10)))
	}
	if s.I < len(s.Kids) {
		s.I++
		return Call(s.Kids[s.I-1])
	}
	return Return[*world](nil)
}

// drive plays m like a game that answers action a with Misc[0] = 10*a.
func drive(m *Machine[*world], w *world, root Frame[*world]) (sent []nle.Action) {
	a, ok := m.Start(w, root)
	for ok {
		sent = append(sent, a)
		o := new(nle.Obs)
		o.Misc[0] = int32(10 * a)
		a, ok = m.Feed(w, o)
	}
	return sent
}

func TestCallReturnSend(t *testing.T) {
	var m Machine[*world]
	w := &world{}
	sent := drive(&m, w, &seq{Kids: []Frame[*world]{&sendN{N: 2}, &sendN{N: 3}}})
	if !reflect.DeepEqual(sent, []nle.Action{1, 2, 1, 2, 3}) {
		t.Fatalf("sent %v", sent)
	}
	if !reflect.DeepEqual(w.log, []string{"ret", "3", "ret", "6"}) {
		t.Fatalf("log %v", w.log)
	}
	if len(m.Stack) != 0 {
		t.Fatalf("stack not empty: %d", len(m.Stack))
	}
}

// guard logs its unwinding; raiser panics with an AgentPanic after one Send.
type guard struct {
	Name string
	Kid  Frame[*world]
	PC   int
}

func (g *guard) Resume(c *C) Cmd[*world] {
	if g.PC == 0 {
		g.PC = 1
		return Call(g.Kid)
	}
	c.W.log = append(c.W.log, "done "+g.Name)
	return Return[*world](c.Ret)
}

func (g *guard) Unwind(c *C) { c.W.log = append(c.W.log, "unwind "+g.Name) }

type raiser struct{ PC int }

func (r *raiser) Resume(c *C) Cmd[*world] {
	if r.PC == 0 {
		r.PC = 1
		return Send[*world](7)
	}
	pyrt.Panic("boom")
	return Cmd[*world]{}
}

// catcher handles AgentPanic from its child once and then returns "caught".
type catcher struct {
	Kid Frame[*world]
	PC  int
}

func (h *catcher) Resume(c *C) Cmd[*world] {
	switch h.PC {
	case 0:
		h.PC = 1
		return Call(h.Kid)
	case 2:
		c.W.log = append(c.W.log, "after catch")
		return Return[*world]("caught")
	}
	return Return[*world](c.Ret)
}

func (h *catcher) Catch(c *C, exc any) bool {
	if _, ok := exc.(*pyrt.AgentPanic); ok && h.PC == 1 {
		c.W.log = append(c.W.log, "catch")
		h.PC = 2
		return true
	}
	return false
}

func TestUnwindCatch(t *testing.T) {
	var m Machine[*world]
	w := &world{}
	drive(&m, w, &guard{Name: "outer", Kid: &catcher{Kid: &guard{Name: "a", Kid: &guard{Name: "b", Kid: &raiser{}}}}})
	want := []string{"unwind b", "unwind a", "catch", "after catch", "done outer"}
	if !reflect.DeepEqual(w.log, want) {
		t.Fatalf("log %v, want %v", w.log, want)
	}
}

func TestUncaughtAndFatal(t *testing.T) {
	var m Machine[*world]
	w := &world{}
	func() {
		defer func() {
			if _, ok := recover().(*pyrt.AgentPanic); !ok {
				t.Fatal("AgentPanic did not reach the caller")
			}
		}()
		drive(&m, w, &guard{Name: "g", Kid: &raiser{}})
	}()
	if !reflect.DeepEqual(w.log, []string{"unwind g"}) || len(m.Stack) != 0 {
		t.Fatalf("log %v stack %d", w.log, len(m.Stack))
	}
	// a non-control error is re-raised as is, without unwinding
	w.log = nil
	func() {
		defer func() {
			if e, ok := recover().(*pyrt.PyExc); !ok || e.Type != "AssertionError" {
				t.Fatal("assertion error lost")
			}
		}()
		drive(&m, w, &guard{Name: "g", Kid: &fails{}})
	}()
	if len(w.log) != 0 {
		t.Fatalf("fatal errors must not unwind: %v", w.log)
	}
}

type fails struct{}

func (fails) Resume(c *C) Cmd[*world] { pyrt.Assert(false, "x"); return Cmd[*world]{} }

type spin struct{}

func (spin) Resume(c *C) Cmd[*world] { return Call[*world](noop{}) }

type noop struct{}

func (noop) Resume(c *C) Cmd[*world] { return Return[*world](nil) }

func TestHang(t *testing.T) {
	m := Machine[*world]{OpLimit: 1000}
	defer func() {
		if _, ok := recover().(*Hang); !ok {
			t.Fatal("no Hang")
		}
	}()
	m.Start(&world{}, spin{})
}
