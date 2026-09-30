package flow

import (
	"aago/world"
)

// Strategies as data (strategy.py).
//
// A Python strategy is a generator that yields exactly once. As data it is a Node: Check runs the generator
// up to its yield and must not act; ok=false means the strategy does not apply now; otherwise body is the
// rest of the generator as a frame, with everything the check computed (the target, the item) in its fields.
// A node itself may hold state (the counter of Every): nodes are created at the same moments as the Python
// strategy objects, so that this state lives exactly as long.
type Node interface {
	Check(a *world.Agent) (ok bool, body Frame)
}

// runNodeF is Strategy.run(): nil if the check fails, else the body's result.
type runNodeF struct {
	N  Node
	PC int
}

func RunNode(n Node) Frame { return &runNodeF{N: n} }

func (s *runNodeF) Resume(c *Ctx) Cmd {
	if s.PC == 0 {
		ok, body := s.N.Check(c.W)
		if !ok {
			return ret(nil)
		}
		s.PC = 1
		return call(body)
	}
	return ret(c.Ret)
}

// runCondF is Strategy.run(return_condition=True): whether the body ran.
type runCondF struct {
	N  Node
	PC int
}

func (s *runCondF) Resume(c *Ctx) Cmd {
	if s.PC == 0 {
		ok, body := s.N.Check(c.W)
		if !ok {
			return ret(false)
		}
		s.PC = 1
		return call(body)
	}
	return ret(true)
}

// seqF runs frames one after another and returns nil (a body like `do_a(); do_b()`). The frames are built
// before the first one runs, so their constructors must not read the state (directionTo and moveTo do).
type seqF struct {
	Fs []Frame
	I  int
}

func seq(fs ...Frame) Frame { return &seqF{Fs: fs} }

func (s *seqF) Resume(c *Ctx) Cmd {
	if s.I < len(s.Fs) {
		s.I++
		return call(s.Fs[s.I-1])
	}
	return ret(nil)
}

// Stepper is one iteration of a generator loop (the yieldLoop shape of explore.go):
//
//	while 1:
//	    <pre>                              # may end the generator: step returns nil
//	    if not yielded: yielded = True; yield True
//	    <post>                             # the action: the frame step returns
//
// A Stepper may also implement Finisher: code after the loop.
type Stepper interface{ Step(a *world.Agent) Frame }

type Finisher interface{ Finish(a *world.Agent) }

// loopN is a strategy made of a Stepper loop: it applies if the first iteration has an action.
type loopN struct{ S Stepper }

func Loop(s Stepper) Node { return &loopN{S: s} }

// LoopFrom is the loop body of s after a check that computed its first action.
func LoopFrom(s Stepper, first Frame) Frame { return &loopF{S: s, First: first} }

func (n *loopN) Check(a *world.Agent) (bool, Frame) {
	act := n.S.Step(a)
	if act == nil {
		return false, nil
	}
	return true, LoopFrom(n.S, act)
}

// loopF runs First, then the actions of the next iterations while there are any.
type loopF struct {
	S     Stepper
	First Frame
	PC    int
}

func (s *loopF) Resume(c *Ctx) Cmd {
	if s.PC == 0 {
		s.PC = 1
		return call(s.First)
	}
	if act := s.S.Step(c.W); act != nil {
		return call(act)
	}
	if f, ok := s.S.(Finisher); ok {
		f.Finish(c.W)
	}
	return ret(nil)
}

// thenNilF runs F and returns nil (a body `do_x()` of a generator that returns None).
type thenNilF struct {
	F  Frame
	PC int
}

func ThenNil(f Frame) Frame { return &thenNilF{F: f} }

func (s *thenNilF) Resume(c *Ctx) Cmd {
	if s.PC == 0 {
		s.PC = 1
		return call(s.F)
	}
	return ret(nil)
}
