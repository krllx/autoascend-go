package flow

import (
	. "aago/internal/pyrt"
	"aago/world"
)

// posWatch is panic_if_position_changes' callback: AgentPanic if the agent is no longer at (Y, X).
type posWatch struct{ Y, X int }

func (w *posWatch) Fire(a *world.Agent) {
	if w.Y != a.Blstats.Y || w.X != a.Blstats.X {
		Panic("position changed")
	}
}

// posGuardF is `with agent.panic_if_position_changes(): body`.
type posGuardF struct {
	Body  Frame
	Watch world.Watch
	PC    int
}

func PosGuard(body Frame) Frame { return &posGuardF{Body: body} }

func (s *posGuardF) Resume(c *Ctx) Cmd {
	a := c.W
	if s.PC == 0 {
		s.Watch = &posWatch{a.Blstats.Y, a.Blstats.X}
		a.OnUpdate = append(a.OnUpdate, s.Watch)
		s.PC = 1
		return call(s.Body)
	}
	s.exit(a)
	return ret(c.Ret)
}

func (s *posGuardF) Unwind(c *Ctx) {
	if s.PC == 1 {
		s.exit(c.W)
	}
}

func (s *posGuardF) exit(a *world.Agent) {
	Assert(Contains(a.OnUpdate, s.Watch))
	a.RemoveOnUpdate(s.Watch)
}
