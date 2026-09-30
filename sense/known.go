package sense

import (
	"strings"

	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/perception"
	"aago/world"
)

// ---- item manager: '\' discoveries

type knownF struct{ PC int }

func UpdateObjectGlyphMapping() Frame { return Atom(&knownF{}) }

func (s *knownF) Resume(c *Ctx) Cmd {
	a := c.W
	m := a.Inventory.ItemManager
	if s.PC == 0 {
		s.PC = 1
		return call(a.Term.Key(A_Command_KNOWN))
	}
	for _, line := range a.Popup {
		if strings.HasPrefix(line, "*") {
			Assert(line[PyIdx(1, len(line))] == ' ' && line[len(line)-1] == ')' &&
				strings.Count(line, "(") == 1 && strings.Count(line, ")") == 1, line)
			name := PyStrip(world.PySlice(line, 1, strings.Index(line, "(")))
			desc := PyStrip(world.PySlice(line, strings.Index(line, "(")+1, len(line)-1))

			nObjs, _ := perception.ParseName(name)
			dGlyphs := O_DescToGlyphs(desc, O_GetCategory(nObjs[0]))
			Assert(len(dGlyphs) > 0)
			if len(nObjs) == 1 && len(dGlyphs) == 1 {
				obj, glyph := nObjs[0], dGlyphs[0]

				g2o, ok := m.GlyphToObject[glyph]
				Assert(!ok || g2o == obj)
				m.GlyphToObject[glyph] = obj
				o2g, ok := m.ObjectToGlyph[obj]
				Assert(!ok || o2g == glyph)
				m.ObjectToGlyph[obj] = glyph
			}
		}
	}
	m.LastObjectGlyphMappingUpdateStep = a.StepCount
	m.LastObjectGlyphMappingUpdateNone = false
	return ret(nil)
}
