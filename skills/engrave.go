package skills

import (
	"strings"

	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/sense"
	"aago/world"
)

// engraveFeed is the additional_action_iterator of engrave(text): answers the prompts of the engrave
// command, reading the screen before each key like the Python generator. Ret is the generator's result.
type engraveFeed struct {
	Text  string
	PC, I int
	Ret   bool
}

func (f *engraveFeed) Next(a *world.Agent) (Action, bool) {
	switch f.PC {
	case 0:
		if !strings.Contains(a.SingleMessage, "What do you want to write with?") {
			a.ForbiddenEngravePosition = Pos{a.Blstats.Y, a.Blstats.X}
			f.PC = 5
			return A_Command_ESC, true
		}
		f.PC = 1
		return world.Ch('-'), true
	case 1:
		f.PC = 2
		if strings.Contains(a.SingleMessage, "Do you want to add to the current engraving?") {
			return world.Ch('n'), true
		}
		fallthrough
	case 2:
		if a.Observation.Misc[2] != 0 {
			return world.Ch(' '), true
		}
		if !strings.Contains(a.SingleMessage, "What do you want to write in the dust here?") {
			a.ForbiddenEngravePosition = Pos{a.Blstats.Y, a.Blstats.X}
			f.PC = 5
			return A_Command_ESC, true
		}
		f.PC = 3
		fallthrough
	case 3:
		if f.I < len(f.Text) {
			f.I++
			return world.Ch(f.Text[f.I-1]), true
		}
		f.PC = 4
		return world.Ch('\r'), true
	case 4:
		f.Ret = true
		f.PC = 5
	}
	return 0, false
}

// engraveF is engrave(text): returns whether the text was written.
type engraveF struct {
	Text string
	Feed *engraveFeed
	PC   int
}

func Engrave(text string) Frame { return &engraveF{Text: text} }

func (s *engraveF) Resume(c *Ctx) Cmd {
	if s.PC == 0 {
		Assert(!strings.Contains(s.Text, "\r"))
		s.Feed = &engraveFeed{Text: s.Text}
		s.PC = 1
		return call(sense.Atom(&engraveBodyF{Feed: s.Feed}))
	}
	return ret(s.Feed.Ret)
}

type engraveBodyF struct {
	Feed *engraveFeed
	PC   int
}

func (s *engraveBodyF) Resume(c *Ctx) Cmd {
	switch s.PC {
	case 0:
		s.PC = 1
		return call(c.W.Term.Keys(A_Command_ENGRAVE, s.Feed))
	case 1:
		s.PC = 2
		return call(sense.GetItemsBelowMe(false))
	}
	return ret(nil)
}
