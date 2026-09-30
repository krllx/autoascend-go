package tactics

import (
	"aago/skills"
	"aago/world"
)

// Tactics of global_logic.py as nodes.

// ---- wait_out_unexpected_state_strategy: wait while blind, confused, stunned, hallucinating or polymorphed

func unexpectedState(a *world.Agent) bool {
	p := a.Character.Prop
	return p.Blind() || p.Confusion() || p.Stun() || p.Hallu() || p.Polymorph()
}

type WaitOut struct{}

func (WaitOut) Check(a *world.Agent) (bool, Frame) {
	if !unexpectedState(a) {
		return false, nil
	}
	return true, &waitOutF{}
}

type waitOutF struct{ PC int }

func (s *waitOutF) Resume(c *Ctx) Cmd {
	if s.PC == 0 || unexpectedState(c.W) {
		s.PC = 1
		return call(skills.Direction("."))
	}
	return ret(nil)
}
