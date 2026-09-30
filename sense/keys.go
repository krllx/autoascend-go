package sense

import (
	"aago/world"
)

// Primitive actions of agent.py as frames. Each Python method keeps its Go method, which runs the frame.

// keysF sends the characters of Text one by one (the body of type_text).
type keysF struct {
	Text string
	I    int
}

func (s *keysF) Resume(c *Ctx) Cmd {
	if s.I < len(s.Text) {
		s.I++
		return call(c.W.Term.Key(world.Ch(s.Text[s.I-1])))
	}
	return ret(nil)
}

// typeText is type_text(text): the keys inside one atom operation.
func TypeText(text string) Frame { return Atom(&keysF{Text: text}) }
