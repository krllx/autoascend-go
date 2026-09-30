package flow

import (
	. "aago/internal/pyrt"
	"aago/world"
)

// Strategy combinators of strategy.py as nodes (see nodes.go). Stateful nodes (Every) are pointers.

// Pred is a condition as data (the lambdas of .condition() / .until()). It must not act.
type Pred interface{ Eval(a *world.Agent) bool }

type notPred struct{ P Pred }

func (p notPred) Eval(a *world.Agent) bool { return !p.P.Eval(a) }

// condN is s.condition(pred).
type condN struct {
	P     Pred
	Inner Node
}

func (n *condN) Check(a *world.Agent) (bool, Frame) {
	if !n.P.Eval(a) {
		return false, nil
	}
	return n.Inner.Check(a)
}

func If(p Pred, inner Node) Node { return &condN{P: p, Inner: inner} }

// everyN is s.every(n): the check passes every n-th time it is made; the counter restarts when the body runs.
type everyN struct {
	N, Cur int
	Inner  Node
}

func Every(n int, inner Node) Node { return &everyN{N: n, Cur: -1, Inner: inner} }

func (n *everyN) Check(a *world.Agent) (bool, Frame) {
	n.Cur++
	if FloorMod(n.Cur, n.N) != 0 {
		return false, nil
	}
	ok, body := n.Inner.Check(a)
	if !ok {
		return false, nil
	}
	return true, &everyBodyF{E: n, Body: body}
}

type everyBodyF struct {
	E    *everyN
	Body Frame
	PC   int
}

func (s *everyBodyF) Resume(c *Ctx) Cmd {
	if s.PC == 0 {
		s.E.Cur = -1
		s.PC = 1
		return call(s.Body)
	}
	return ret(c.Ret)
}

// repeatN is s.repeat(): run the body again while the check passes.
type repeatN struct{ Inner Node }

func Repeat(inner Node) Node { return &repeatN{Inner: inner} }

func (n *repeatN) Check(a *world.Agent) (bool, Frame) {
	ok, body := n.Inner.Check(a)
	if !ok {
		return false, nil
	}
	return true, &repeatF{N: n, First: body}
}

type repeatF struct {
	N     *repeatN
	First Frame
	PC    int
}

func (s *repeatF) Resume(c *Ctx) Cmd {
	if s.PC == 0 {
		s.PC = 1
		return call(s.First)
	}
	ok, body := s.N.Inner.Check(c.W)
	if !ok {
		return ret(nil)
	}
	return call(body)
}

// beforeN is s.before(o): s then o, or o alone if s does not apply; returns [s's result, o's result].
type beforeN struct{ A, B Node }

func Before(a, b Node) Node { return &beforeN{A: a, B: b} }

func (n *beforeN) Check(a *world.Agent) (bool, Frame) {
	if ok1, b1 := n.A.Check(a); ok1 {
		return true, &beforeF{N: n, First: b1}
	}
	if ok2, b2 := n.B.Check(a); ok2 {
		return true, &beforeF{N: n, Second: b2, PC: 2}
	}
	return false, nil
}

type beforeF struct {
	N             *beforeN
	First, Second Frame
	R1            any
	PC            int // 0: run First, 1: First done, 2: run Second, 3: Second done
}

func (s *beforeF) Resume(c *Ctx) Cmd {
	switch s.PC {
	case 0:
		s.PC = 1
		return call(s.First)
	case 1:
		s.R1 = c.Ret
		ok2, b2 := s.N.B.Check(c.W)
		if !ok2 {
			return ret([2]any{s.R1, nil})
		}
		s.PC = 3
		return call(b2)
	case 2:
		s.PC = 3
		return call(s.Second)
	}
	return ret([2]any{s.R1, c.Ret})
}

// untilN is s.until(pred) = s.condition(not pred).preempt([stop when pred], continue=False).
func Until(inner Node, p Pred) Node {
	return Preempt(If(notPred{p}, inner), []Node{&stopN{P: p}}, false)
}

type stopN struct{ P Pred }

func (n *stopN) Check(a *world.Agent) (bool, Frame) {
	if !n.P.Eval(a) {
		return false, nil
	}
	return true, retNil{}
}

// retNil is an empty body.
type retNil struct{}

func (retNil) Resume(c *Ctx) Cmd { return ret(nil) }

// ---- preempt

// preemptN is s.preempt(agent, strategies, continue_after_preemption): run s; after every step each of the
// strategies may interrupt it, and then s starts over (with continue) or the whole thing ends.
type preemptN struct {
	Base     Node
	Preempts []Node
	Cont     bool
}

func Preempt(base Node, preempts []Node, cont bool) Node {
	return &preemptN{Base: base, Preempts: preempts, Cont: cont}
}

func (n *preemptN) Check(a *world.Agent) (bool, Frame) {
	var ok bool
	var body Frame
	a.DisallowStepCalling(func() { ok, body = n.Base.Check(a) })
	if !ok {
		return false, nil
	}
	return true, &preemptF{N: n, First: body}
}

// preemptF is agent.preempt(strategies, fn, first_func, continue): Watches are the on_update callbacks
// (one per strategy, identified by pointer), registered only while fn runs (the `with add_on_update` inside
// the try), so a running preemptor cannot be interrupted by its own siblings.
type preemptF struct {
	N                    *preemptN
	First                Frame
	Watches              []world.Watch
	Started              bool
	CallUpdate, IsFirst  bool
	Val                  any
	LastStep, Inactivity int
	Registered           bool
	Iterator             Frame
	PC                   int // 0: loop head, 1: fn running, 2: preemptor body running, 3: start the preemptor body
}

// preemptWatch is the on_update callback of preemptor I: if its check passes, interrupt.
type preemptWatch struct {
	P *preemptF
	I int
}

func (w *preemptWatch) Fire(a *world.Agent) {
	if ok, body := w.P.N.Preempts[w.I].Check(a); ok {
		panic(&AgentChangeStrategy{ID: w, Body: body})
	}
}

func (s *preemptF) Resume(c *Ctx) Cmd {
	a := c.W
	if !s.Started {
		s.Started = true
		Assert(!a.NoStepCalls)
		s.Watches = make([]world.Watch, len(s.N.Preempts))
		for i := range s.Watches {
			s.Watches[i] = &preemptWatch{P: s, I: i}
		}
		s.CallUpdate, s.IsFirst = true, true
		s.LastStep = a.StepCount
	}
	for {
		switch s.PC {
		case 0:
			s.Inactivity++
			if a.StepCount != s.LastStep {
				s.LastStep = a.StepCount
				s.Inactivity = 0
			}
			Assert(s.Inactivity < 5, "cyclic preempt")
			a.OnUpdate = append(a.OnUpdate, s.Watches...)
			s.Registered = true
			if s.CallUpdate {
				s.CallUpdate = false
				a.CallUpdateFunctions(s.Watches)
			}
			s.PC = 1
			if s.IsFirst && s.First != nil {
				return call(s.First)
			}
			return call(RunNode(s.N.Base))
		case 1:
			s.Val = c.Ret
			s.unregister(a)
			return ret(s.Val)
		case 3:
			s.PC = 2
			return call(s.Iterator)
		case 2:
			if !s.N.Cont {
				return ret(s.Val)
			}
			s.IsFirst = false
			s.PC = 0
		}
	}
}

func (s *preemptF) unregister(a *world.Agent) {
	s.Registered = false
	for _, w := range s.Watches {
		a.RemoveOnUpdate(w)
	}
}

func (s *preemptF) Catch(c *Ctx, exc any) bool {
	if !s.Registered { // the preemptor body runs outside the try
		return false
	}
	s.unregister(c.W)
	e, ok := exc.(*AgentChangeStrategy)
	if !ok {
		return false
	}
	if w, isWatch := e.ID.(world.Watch); !isWatch || !Contains(s.Watches, w) {
		return false
	}
	s.Iterator = e.Body.(Frame)
	s.PC = 3
	return true
}

func (s *preemptF) Unwind(c *Ctx) {
	if s.Registered {
		s.unregister(c.W)
	}
}
