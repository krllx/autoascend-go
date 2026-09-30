package sense

import (
	. "aago/internal/pyrt"
)

// Python context managers of the agent as frames: each wraps a body frame and does the enter / exit work
// around it; Unwind is the exit path taken when an exception passes through.

// atomF is `with agent.atom_operation(allow_update): body`. Inside an atom operation steps do not update
// the state fully and on_update callbacks do not run; the outermost one updates the state on normal exit.
type atomF struct {
	AllowUpdate bool
	Body        Frame
	Old         int8 // allow_update of the enclosing atom operation (nested case)
	Ret         any
	PC          int // 0: enter, 1: nested body, 2: outermost body, 3: final update_state
}

func Atom(body Frame) Frame { return &atomF{Body: body} }

func (s *atomF) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 0:
		Assert(!a.NoStepCalls)
		if a.TurnsInAtomOperation >= 0 {
			s.Old = a.AtomOperationAllowUpdate
			if s.Old < 0 {
				a.AtomOperationAllowUpdate = int8(B2i(s.AllowUpdate))
			} else {
				Assert(s.Old == 1 || !s.AllowUpdate)
				a.AtomOperationAllowUpdate = int8(B2i(s.Old == 1 && s.AllowUpdate))
			}
			s.PC = 1
			return call(s.Body)
		}
		a.TurnsInAtomOperation = 0
		a.AtomOperationAllowUpdate = int8(B2i(s.AllowUpdate))
		s.PC = 2
		return call(s.Body)
	case 1:
		a.AtomOperationAllowUpdate = s.Old
		return ret(c.Ret)
	case 2:
		s.Ret = c.Ret
		a.TurnsInAtomOperation = -1
		a.AtomOperationAllowUpdate = -1
		s.PC = 3
		return call(UpdateState(true, true))
	case 3:
		return ret(s.Ret)
	}
	panic("atomF: bad PC")
}

func (s *atomF) Unwind(c *Ctx) {
	a := c.W
	switch s.PC {
	case 1:
		a.AtomOperationAllowUpdate = s.Old
	case 2:
		a.TurnsInAtomOperation = -1
		a.AtomOperationAllowUpdate = -1
	}
}
