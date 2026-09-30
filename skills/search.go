package skills

import (
	"strconv"
	"strings"

	"aago/flow"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/sense"
)

// searchF is the atom body of search(max_count).
type searchF struct {
	MaxCount, PC int
}

func (s *searchF) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 0:
		s.PC = 1
		if s.MaxCount > 1 {
			return call(sense.TypeText(strconv.Itoa(s.MaxCount)))
		}
		fallthrough
	case 1:
		s.PC = 2
		return call(a.Term.Key(A_Command_SEARCH))
	case 2:
		a.CurrentLevel().SearchCount[a.Blstats.Y][a.Blstats.X] += int32(s.MaxCount)
		if strings.Contains(a.Message, "You find ") {
			s.PC = 3
			return call(sense.CheckTerrain(true))
		}
	}
	return ret(nil)
}

// searchTop is search(max_count): returns true.
type searchTopF struct {
	MaxCount, PC int
}

func (s *searchTopF) Resume(c *Ctx) Cmd {
	if s.PC == 0 {
		Assert(s.MaxCount >= 1)
		s.PC = 1
		return call(flow.PosGuard(sense.Atom(&searchF{MaxCount: s.MaxCount})))
	}
	return ret(true)
}

func SearchTimes(maxCount int) Frame { return &searchTopF{MaxCount: maxCount} }
