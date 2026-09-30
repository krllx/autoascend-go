package sense

import (
	"strings"

	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/world"
)

// Routines that read game views to update the agent's knowledge (several run inside update_state).

// ---- character: ^X attributes and #enhance

type attributesF struct{ PC int }

// checkAttributes reads ^X into the character.
func CheckAttributes() Frame { return &attributesF{} }

func (s *attributesF) Resume(c *Ctx) Cmd {
	a := c.W
	if s.PC == 0 {
		s.PC = 1
		return call(a.Term.Key(A_Command_ATTRIBUTES))
	}
	text := strings.Join(a.Popup, " ")
	a.Character.Parse(text)
	a.Character.SelfGlyph = int(a.Glyphs()[a.Blstats.Y][a.Blstats.X])
	return ret(nil)
}

// enhanceF reads #enhance and advances every skill that can be advanced.
type enhanceF struct {
	Old []int
	PC  int
}

func ParseEnhanceView() Frame { return Atom(&enhanceF{}) }

func (s *enhanceF) Resume(c *Ctx) Cmd {
	a := c.W
	ch := a.Character
	switch s.PC {
	case 0:
		s.PC = 1
		return call(a.Term.Key(A_Command_ENHANCE))
	case 1:
		ch.ParseEnhanceView()
	case 2: // the skill is picked
		s.PC = 3
		return call(a.Term.Key(A_Command_ENHANCE))
	case 3:
		ch.ParseEnhanceView()
		changed := false
		for i := range s.Old {
			if s.Old[i] != ch.SkillLevels[i] {
				changed = true
			}
		}
		Assert(changed, s.Old, ch.SkillLevels)
	}
	if ch.UpgradableSkills.Len() > 0 {
		toUpgrade := ch.UpgradableSkills.Keys()[0]
		s.Old = append([]int(nil), ch.SkillLevels...)
		letter := ch.UpgradableSkills.MustGet(toUpgrade)
		s.PC = 2
		d := a.Term.Dialog(A_Command_ENHANCE, &enhancePickF{Letter: letter})
		return call(d)
	}
	return ret(nil)
}

// enhancePickF pages the #enhance menu until the skill's letter shows, then picks it.
type enhancePickF struct {
	Letter string
	PC     int
}

func (s *enhancePickF) Resume(c *Ctx) Cmd {
	a := c.W
	if s.PC == 1 {
		return ret(nil)
	}
	if !strings.Contains(strings.Join(a.SinglePopup, "\n"), s.Letter+" - ") {
		return call(a.Term.RawKey(A_TextCharacters_SPACE))
	}
	s.PC = 1
	return call(a.Term.RawKey(world.Ch(s.Letter[0])))
}
