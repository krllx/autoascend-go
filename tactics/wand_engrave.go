package tactics

import (
	"regexp"
	"strings"

	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/sense"
	"aago/world"
)

// Port of autoascend/item/inventory.py: wand engrave identification and the remaining low-level strategies.

type wandMsg struct {
	re    *regexp.Regexp
	types []string
}

// _determine_possible_wands mapping (dict literal order; keys are used as regexes).
var wandMapping = func() []wandMsg {
	wandRegex := "[a-zA-Z ]+"
	floorRegex := "[a-zA-Z]+"
	m := func(s string, types ...string) wandMsg { return wandMsg{regexp.MustCompile(s), types} }
	return []wandMsg{
		m("The engraving on the "+floorRegex+" vanishes!", "cancellation", "teleportation", "make invisible"),
		// TODO?: cold,  # (if the existing engraving is a burned one)

		m("A few ice cubes drop from the wand.", "cold"),
		m("The bugs on the "+floorRegex+" stop moving", "death", "sleep"),
		m("This "+wandRegex+" is a wand of digging!", "digging"),
		m("Gravel flies up from the floor!", "digging"),
		m("This "+wandRegex+" is a wand of fire!", "fire"),
		// "lighting" in the Python bot: O.from_name failed on it (no such wand)
		m("Lightning arcs from the wand. You are blinded by the flash!", "lightning"),
		m("This "+wandRegex+" is a wand of lightning!", "lightning"),
		m("The "+floorRegex+" is riddled by bullet holes!", "magic missile"),
		m("The engraving now reads:", "polymorph"),
		m("The bugs on the "+floorRegex+" slow down!", "slow monster"),
		m("The bugs on the "+floorRegex+" speed up!", "speed monster"),
		m("The wand unsuccessfully fights your attempt to write!", "striking"),

		// activated effects:
		m("A lit field surrounds you!", "light"),
		m("You may wish for an object.", "wishing"),
		m("You feel self-knowledgeable...", "enlightenment"), // TODO: parse the effect
		// TODO: "The wand is too worn out to engrave.": [None],  # wand is exhausted
	}
}()

var (
	wandExplodesRe = regexp.MustCompile("Your [a-zA-Z ]+ suddenly explodes!")
	wandWornOutRe  = regexp.MustCompile("The wand is too worn out to engrave.")
	wandGlowsRe    = regexp.MustCompile("[a-zA-Z ]+ glows, then fades.")
)

// Item tactics of inventory.py (low-level strategies) as nodes.

// ---- wand_engrave_identify: engrave with unknown wands to learn what they are

type wandEngraveIdentifyN struct{}

func wandQualifies(a *world.Agent, item *world.Item) bool {
	inv := a.Inventory
	if item.Objs[0].Kind != KWand {
		return false
	}
	if item.IsUnambiguous() {
		return false
	}
	lvl := a.CurrentLevel()
	if !G_FLOOR.Has(int(lvl.Objects[a.Blstats.Y][a.Blstats.X])) {
		return false
	}
	if inv.ItemManager.AlreadyEngravedGlyphs[item.Glyphs[0]] {
		return false
	}
	if len(item.Glyphs) > 1 {
		return false
	}
	if item.Comment == "EMPT" {
		return false
	}
	return true
}

func (wandEngraveIdentifyN) Check(a *world.Agent) (bool, Frame) {
	inv := a.Inventory
	if a.Character.Prop.Polymorph() {
		return false, nil // TODO: only for handless monsters (which cannot write)
	}
	inv.SkipEngraveCounter--
	if a.Character.Prop.Blind() || inv.SkipEngraveCounter > 0 {
		return false, nil
	}
	items := a.Inventory.Items.AllItems
	for i, item := range items {
		if wandQualifies(a, item) {
			return true, &wandEngraveIdentifyF{Items: items, I: i}
		}
	}
	return false, nil
}

// wandEngraveIdentifyF engraves with Items[I] and then with every later item that qualifies.
type wandEngraveIdentifyF struct {
	Items []*world.Item
	I     int
	PC    int // 0: process Items[I], 1: next item, 2: final update
}

func (s *wandEngraveIdentifyF) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 0:
		s.PC = 1
		a.Inventory.SkipEngraveCounter = 8
		return call(sense.Atom(&engraveWandF{Item: s.Items[s.I]}))
	case 1:
		for s.I++; s.I < len(s.Items); s.I++ {
			if wandQualifies(a, s.Items[s.I]) {
				a.Inventory.SkipEngraveCounter = 8
				return call(sense.Atom(&engraveWandF{Item: s.Items[s.I]}))
			}
		}
		s.PC = 2
		return call(sense.UpdateItems(true))
	}
	return ret(nil)
}

// engraveWandF is the atom operation of process(item): _engrave_single_wand and the bookkeeping.
type engraveWandF struct {
	Item *world.Item
	Skip bool // skip_engraving
	Try  int  // the `for _ in range(5)` loop
	Poss []*Obj
	PC   int
}

func (s *engraveWandF) Resume(c *Ctx) Cmd {
	a := c.W
	inv := a.Inventory
	for {
		switch s.PC {
		case 0:
			s.PC = 1
			return call(a.Term.Key(A_Command_LOOK))
		case 1:
			if a.Message != "You see no objects here." {
				return ret(nil) // (None, False): there is a problem with engraving on this tile
			}
			s.PC = 2
		case 2: // write 'x' with finger in the dust
			if s.Try == 5 {
				Assert(false, a.Message)
			}
			s.Try++
			d := a.Term.Dialog(A_Command_ENGRAVE, &engraveFingerGen{W: s})
			s.PC = 3
			return call(d)
		case 3:
			if s.Skip {
				Assert(strings.HasSuffix(PyStrip(a.Message), "Never mind.") ||
					strings.Contains(a.Message, "You cannot wipe out the message that is burned into the floor here."), a.Message)
				return ret(nil)
			}
			// check if the written 'x' is visible when looking
			s.PC = 4
			return call(a.Term.Key(A_Command_LOOK))
		case 4:
			if !(strings.Contains(a.Message, "Something is written here in the dust.") &&
				strings.Contains(a.Message, `You read: "x"`)) {
				return ret(nil)
			}
			// try engraving with the wand
			letter := inv.Items.GetLetter(s.Item)
			s.Poss = []*Obj{}
			d := a.Term.Dialog(A_Command_ENGRAVE, &engraveWandGen{W: s, Letter: letter})
			s.PC = 5
			return call(d)
		case 5:
			if s.Skip {
				return ret(nil)
			}
			if strings.Contains(a.SingleMessage, "Do you want to add to the current engraving") {
				s.PC = 6
				return call(sense.TypeText("q"))
			}
			return s.done(a)
		case 6:
			Assert(PyStrip(a.SingleMessage) == "Never mind.", a.SingleMessage)
			return s.done(a)
		}
	}
}

// done records the wand types learned (possible_wand_types, True).
func (s *engraveWandF) done(a *world.Agent) Cmd {
	m := a.Inventory.ItemManager
	m.GlyphToPossibleWandTypes[s.Item.Glyphs[0]] = s.Poss
	m.AlreadyEngravedGlyphs[s.Item.Glyphs[0]] = true
	m.PossibleObjectsFromGlyph(s.Item.Glyphs[0])
	return ret(nil)
}

// engraveFingerGen writes 'x' in the dust with a finger.
type engraveFingerGen struct {
	W  *engraveWandF
	PC int
}

func (g *engraveFingerGen) Resume(c *Ctx) Cmd {
	smsg := c.W.SingleMessage
	switch g.PC {
	case 0:
		Assert(strings.HasPrefix(smsg, "What do you want to write with?"), smsg)
		g.PC = 1
		return call(c.W.Term.RawKey(world.Ch('-')))
	case 1:
		if strings.HasPrefix(smsg, "You wipe out the message that was written") {
			g.PC = 5
			return call(c.W.Term.RawKey(world.Ch(' ')))
		}
		if strings.HasPrefix(smsg, "You cannot wipe out the message that is burned into the floor here.") {
			g.W.Skip = true
			return ret(nil)
		}
		Assert(strings.HasPrefix(smsg, "You write in the dust with your fingertip."), smsg)
		g.PC = 2
		return call(c.W.Term.RawKey(world.Ch(' ')))
	case 2:
		Assert(strings.HasPrefix(smsg, "What do you want to write in the dust here?"), smsg)
		g.PC = 3
		return call(c.W.Term.RawKey(world.Ch('x')))
	case 3:
		Assert(strings.HasPrefix(smsg, "What do you want to write in the dust here?"), smsg)
		g.PC = 4
		return call(c.W.Term.RawKey(world.Ch('\r')))
	case 5:
		g.W.Skip = true
	}
	return ret(nil)
}

// engraveWandGen engraves with the wand and reads what happened.
type engraveWandGen struct {
	W      *engraveWandF
	Letter string
	PC     int
}

func (g *engraveWandGen) Resume(c *Ctx) Cmd {
	a := c.W
	switch g.PC {
	case 0:
		Assert(strings.HasPrefix(a.SingleMessage, "What do you want to write with?"), a.SingleMessage)
		g.PC = 1
		return call(a.Term.RawKey(world.Ch(g.Letter[0])))
	case 1:
		g.PC = 2
		if strings.Contains(a.SingleMessage, "Do you want to add to the current engraving") {
			return call(sense.TypeText("y"))
		}
		fallthrough
	case 2:
		r, ok, empty := classifyWandEngraving(a.Inventory, a.SingleMessage, g.W.Item)
		if empty {
			g.PC = 3
			return call(sense.CallItem(g.W.Item, "EMPT"))
		}
		if ok {
			g.W.Poss = append(g.W.Poss, r...)
		} else {
			g.W.Skip = true // wand exploded
		}
		return ret(nil)
	case 3:
		g.W.Skip = true // (None) after calling the empty wand
	}
	return ret(nil)
}

// classifyWandEngraving is _determine_possible_wands without its one action: callEmpty means "call the
// wand EMPT, then return None".
func classifyWandEngraving(inv *world.Inventory, message string, item *world.Item) (objs []*Obj, ok, callEmpty bool) {
	for _, mm := range wandMapping {
		res := mm.re.FindAllString(message, -1)
		if len(res) > 0 {
			Assert(len(res) == 1)
			r := make([]*Obj, len(mm.types))
			for i, w := range mm.types {
				r[i] = O_FromName(w, NH_WAND_CLASS)
			}
			return r, true, false
		}
	}

	// TODO: "wand is cancelled (x:-1)" ?
	// TODO: "secret door detection self-identifies if secrets are detected" ?

	if res := wandExplodesRe.FindAllString(inv.Agent.Message, -1); len(res) > 0 {
		Assert(len(res) == 1)
		return nil, false, false
	}
	if res := wandWornOutRe.FindAllString(inv.Agent.Message, -1); len(res) > 0 {
		Assert(len(res) == 1)
		return nil, false, true
	}
	if res := wandGlowsRe.FindAllString(inv.Agent.Message, -1); len(res) > 0 {
		Assert(len(res) == 1)
		r := []*Obj{}
		for _, p := range O_PossibilitiesFromGlyph(item.Glyphs[0]) {
			if p.Name != "light" && p.Name != "wishing" {
				r = append(r, p)
			}
		}
		return r, true, false
	}
	Assert(false, message)
	return nil, false, false
}
