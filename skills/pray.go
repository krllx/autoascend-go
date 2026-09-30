package skills

import (
	. "aago/nle"
)

// prayF is pray(): returns true.
type prayF struct{ PC int }

func Pray() Frame { return &prayF{} }

func (s *prayF) Resume(c *Ctx) Cmd {
	if s.PC == 0 {
		s.PC = 1
		return call(c.W.Term.Key(A_Command_PRAY))
	}
	c.W.LastPrayerTurn = c.W.Blstats.Time
	return ret(true)
}
