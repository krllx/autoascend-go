package skills

import (
	"strings"

	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/sense"
)

// untrapF is the atom body of untrap(y, x): returns whether the trap is handled.
type untrapF struct {
	Y, X, PC int
}

func (s *untrapF) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 0:
		s.PC = 1
		return call(sense.TypeText("#u"))
	case 1:
		s.PC = 2
		return call(a.Term.Key(A_MiscAction_MORE))
	case 2:
		Assert(a.SingleMessage == "In what direction?", a.SingleMessage)
		s.PC = 3
		return call(directionTo(a, s.Y, s.X))
	}
	if a.SingleMessage == "You know of no traps there." {
		return ret(true)
	}
	if a.SingleMessage == "You disarm the trap." {
		return ret(true)
	}
	return ret(false)
}

func Untrap(y, x int) Frame { return sense.Atom(&untrapF{Y: y, X: x}) }

// untrapContainerF is the atom body of untrap_container_below_me: returns untrapResult.
type untrapContainerF struct{ PC int }

func UntrapContainer() Frame { return &untrapContainerF{} }

type UntrapResult struct {
	Msg  string
	Fail bool
}

func (s *untrapContainerF) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 0:
		s.PC = 1
		return call(sense.TypeText("#u"))
	case 1:
		s.PC = 2
		return call(a.Term.Key(A_MiscAction_MORE))
	case 2:
		Assert(a.SingleMessage == "In what direction?", a.SingleMessage)
		s.PC = 3
		return call(sense.TypeText("."))
	case 3:
		s.PC = 4
		if strings.Contains(a.Message, "There is a container and a ") {
			return call(sense.TypeText("n"))
		}
		fallthrough
	case 4:
		if strings.Contains(a.Message, "You know of no traps there.") {
			Panic("no container below me to untrap")
		}
		s.PC = 5
		if strings.Contains(a.Message, "There is a container and ") &&
			(strings.Contains(a.Message, " trap here.") || strings.Contains(a.Message, " field here.")) &&
			(strings.Contains(a.Message, "trap?") || strings.Contains(a.Message, "field?")) {
			return call(sense.TypeText("n"))
		}
		fallthrough
	case 5:
		if strings.Contains(a.SingleMessage, "You cannot disable this trap.") {
			return ret(UntrapResult{})
		}
		Assert(strings.Contains(a.SingleMessage, "Check it for traps?"), a.SingleMessage)
		s.PC = 6
		return call(sense.TypeText("y"))
	case 6:
		if strings.HasPrefix(a.Message, "You find no traps on the") {
			return ret(UntrapResult{})
		}
		Assert(strings.Contains(a.Message, "Disarm it?"), a.Message)
		s.PC = 7
		return call(sense.TypeText("y"))
	}
	if strings.Contains(a.Message, "You disarm it!") {
		return ret(UntrapResult{})
	}
	return ret(UntrapResult{a.Message, true})
}
