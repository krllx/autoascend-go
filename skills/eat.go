package skills

import (
	"regexp"
	"strings"

	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/sense"
	"aago/world"
)

// thereIsHereEatRe: a floor item offered before the inventory. The Python pattern allowed only letters, digits,
// '-' and ' ' in the item, so a corpse the bot had named ("... named #old") was not recognized: the inventory
// letter went to the [ynq] question and the bot looped without the turn passing (seed 3011).
var thereIsHereEatRe = regexp.MustCompile(`There (is|are)[^;]* here; eat (it|one)\?`)

// eatF is eat(item, quaff, smart); quaff(item, smart) is eat with quaff = true.
type eatF struct {
	Item         *world.Item
	Quaff, Smart bool
	PC           int
}

func Eat(item *world.Item, quaff, smart bool) Frame {
	return &eatF{Item: item, Quaff: quaff, Smart: smart}
}

func (s *eatF) Resume(c *Ctx) Cmd {
	inv := c.W.Inventory
	switch s.PC {
	case 0:
		if s.Smart {
			if !s.Quaff && inv.InBelowMe(s.Item) {
				s.PC = 3
				return call(sense.Atom(&eatFloorF{Item: s.Item}))
			}
			// TODO: eat directly from ground if possible
			s.PC = 1
			return call(MoveToInventory(s.Item))
		}
	case 1:
		s.Item = c.Ret.(*world.Item)
	case 2:
		if c.Ret.(bool) { // done
			return ret(true)
		}
		Assert(false)
	case 3:
		return ret(c.Ret)
	}
	Assert(inv.InAllItems(s.Item), s.Item)
	letter := inv.Items.GetLetter(s.Item)
	s.PC = 2
	return call(sense.Atom(&eatBodyF{Item: s.Item, Quaff: s.Quaff, Letter: letter}))
}

// eatFloorF is the atom operation of smart eat for an item on the floor: answer the "eat it?" questions.
type eatFloorF struct {
	Item *world.Item
	PC   int // 0: start, 1: a question may be shown, 2: 'y' sent
}

func (s *eatFloorF) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 0:
		s.PC = 1
		return call(a.Term.Key(A_Command_EAT))
	case 2:
		return ret(true)
	}
	if strings.Contains(a.Message, "; eat it? [ynq]") || strings.Contains(a.Message, "; eat one? [ynq]") {
		if strings.Contains(a.Message, world.ItemTextStr(s.Item)+" here; eat it? [ynq]") ||
			strings.Contains(a.Message, world.ItemTextStr(s.Item)+" here; eat one? [ynq]") {
			s.PC = 2
			return call(sense.TypeText("y"))
		}
		return call(sense.TypeText("n")) // and look at the next question
	}
	// if "What do you want to eat?" in self.agent.message or \
	//         "You don't have anything to eat." in self.agent.message:
	Panic("no such food is lying here")
	return Cmd{}
}

// eatBodyF is the atom operation of eat / quaff for an item in the inventory: returns `done`.
type eatBodyF struct {
	Item   *world.Item
	Quaff  bool
	Letter string
	PC     int // 0: start, 1: command sent, 2: "There is ... here; eat it?" answered, 3: letter sent, 4: floor item questions, 5: 'y' sent
}

func (s *eatBodyF) Resume(c *Ctx) Cmd {
	a := c.W
	inv := a.Inventory
	switch s.PC {
	case 0:
		s.PC = 1
		if s.Quaff {
			d := a.Term.Dialog(A_Command_QUAFF, &quaffFountainGen{})
			return call(d)
		}
		return call(a.Term.Key(A_Command_EAT))
	case 1:
		if inv.InAllItems(s.Item) {
			s.PC = 2
		} else if inv.InBelowMe(s.Item) {
			s.PC = 4
			return s.floorQuestion(a)
		} else {
			return ret(false)
		}
		fallthrough
	case 2:
		if Search(thereIsHereEatRe, a.Message) != nil {
			return call(sense.TypeText("n"))
		}
		s.PC = 3
		return call(sense.TypeText(s.Letter))
	case 4:
		return s.floorQuestion(a)
	}
	return ret(true) // 3, 5
}

// floorQuestion answers the "eat it?" questions about the floor items: 'y' for the item, 'n' for the others.
// Not in the Python bot: its loop sent no key for another item and never ended (the aabot watchdog stopped it),
// and for the item it called a missing Inventory.type_text (AttributeError).
func (s *eatBodyF) floorQuestion(a *world.Agent) Cmd {
	if strings.Contains(a.Message, " eat it? [ynq]") || strings.Contains(a.Message, " eat one? [ynq]") {
		if strings.Contains(a.Message, world.ItemTextStr(s.Item)) {
			s.PC = 5
			return call(sense.TypeText("y"))
		}
		return call(sense.TypeText("n"))
	}
	if strings.Contains(a.Message, "What do you want to eat?") ||
		strings.Contains(a.Message, "You don't have anything to eat.") {
		Panic("no food is lying here")
	}
	Assert(false, a.Message)
	return Cmd{}
}

// quaffFountainGen declines drinking from a fountain.
type quaffFountainGen struct {
	PC int
}

func (g *quaffFountainGen) Resume(c *Ctx) Cmd {
	if g.PC == 0 && strings.HasPrefix(c.W.Message, "Drink from the fountain?") {
		g.PC = 1
		return call(c.W.Term.RawKey(world.Ch('n')))
	}
	return ret(nil)
}
