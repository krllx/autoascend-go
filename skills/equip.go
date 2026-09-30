package skills

import (
	"regexp"
	"strings"

	"aago/assess"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/perception"
	"aago/sense"
	"aago/world"
)

// ####### ACTIONS

var wieldRe = regexp.MustCompile(`(You secure the tether\.  )?([a-zA-z] - |welds?( itself| themselves| ) to|` +
	`You are already wielding that|You are empty handed|You are already empty handed)`)

// Wield, wear, take off, eat and quaff (inventory.py actions) as frames.

// wieldF is wield(item, smart); item nil means fists. Returns whether the item is (now) wielded.
type wieldF struct {
	Item  *world.Item
	Smart bool
	PC    int
}

func Wield(item *world.Item, smart bool) Frame { return &wieldF{Item: item, Smart: smart} }

func (s *wieldF) Resume(c *Ctx) Cmd {
	a := c.W
	inv := a.Inventory
	switch s.PC {
	case 0:
		s.PC = 1
		if s.Smart && s.Item != nil {
			return call(MoveToInventory(s.Item))
		}
	case 1:
		s.Item = c.Ret.(*world.Item)
	case 2:
		return ret(c.Ret)
	}
	item := s.Item
	var letter string
	if item == nil { // fists
		letter = "-"
	} else {
		letter = inv.Items.GetLetter(item)
	}
	if item != nil && item.Equipped {
		return ret(true)
	}
	if a.Character.Prop.Polymorph() {
		// TODO: depends on kind of a monster
		return ret(false)
	}
	if (inv.Items.MainHand != nil && inv.Items.MainHand.Status == perception.Item_CURSED) ||
		(item != nil && world.ObjBi(item.Objs[0]) != 0 && inv.Items.OffHand != nil) {
		return ret(false)
	}
	s.PC = 2
	return call(sense.Atom(&wieldBodyF{Letter: letter}))
}

// wieldBodyF is the atom operation of wield: returns false if the game refused.
type wieldBodyF struct {
	Letter string
	PC     int
}

func (s *wieldBodyF) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 0:
		s.PC = 1
		return call(a.Term.Key(A_Command_WIELD))
	case 1:
		if strings.Contains(a.Message, "Don't be ridiculous") {
			return ret(false)
		}
		Assert(strings.Contains(a.Message, "What do you want to wield"), a.Message)
		s.PC = 2
		return call(sense.TypeText(s.Letter))
	}
	if strings.Contains(a.Message, "You cannot wield a two-handed sword while wearing a shield.") ||
		strings.Contains(a.Message, "You cannot wield a two-handed weapon while wearing a shield.") ||
		strings.Contains(a.Message, " welded to your hand") {
		return ret(false)
	}
	Assert(Search(wieldRe, a.Message) != nil, a.Message, a.Popup)
	return ret(true)
}

// wieldBestMeleeWeapon returns whether it wielded something.
type wieldBestF struct{ PC int }

func WieldBest() Frame { return &wieldBestF{} }

func (s *wieldBestF) Resume(c *Ctx) Cmd {
	a := c.W
	if s.PC == 0 {
		item := assess.GetBestMeleeWeapon(a.Inventory, nil, false)
		if item != a.Inventory.Items.MainHand {
			s.PC = 1
			return call(Wield(item, true))
		}
		return ret(false)
	}
	return ret(c.Ret)
}

// wearF is wear(item, smart).
type wearF struct {
	Item  *world.Item
	Smart bool
	PC    int
}

func Wear(item *world.Item, smart bool) Frame { return &wearF{Item: item, Smart: smart} }

func (s *wearF) Resume(c *Ctx) Cmd {
	inv := c.W.Inventory
	switch s.PC {
	case 0:
		Assert(s.Item != nil)
		s.PC = 1
		if s.Smart {
			return call(MoveToInventory(s.Item))
			// TODO: smart should be more than that (taking off the armor for shirts, etc)
		}
	case 1:
		s.Item = c.Ret.(*world.Item)
	case 2:
		return ret(c.Ret)
	}
	letter := inv.Items.GetLetter(s.Item)
	if s.Item.Equipped {
		return ret(true)
	}
	// for i in self.items: assert not isinstance(i, O.Armor) or ... -- always true for Item instances
	s.PC = 2
	return call(sense.Atom(&wearBodyF{Letter: letter}))
}

type wearBodyF struct {
	Letter string
	PC     int
}

func (s *wearBodyF) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 0:
		s.PC = 1
		return call(a.Term.Key(A_Command_WEAR))
	case 1:
		if strings.Contains(a.Message, "Don't even bother.") {
			return ret(false)
		}
		Assert(strings.Contains(a.Message, "What do you want to wear?"), a.Message)
		s.PC = 2
		return call(sense.TypeText(s.Letter))
	}
	if feetStuck(a) {
		return ret(false)
	}
	Assert(strings.Contains(a.Message, "You finish your dressing maneuver.") ||
		strings.Contains(a.Message, "You are now wearing "), a.Message)
	return ret(true)
}

// feetStuckRe: the game refuses boots on or off while the feet are held (do_wear.c canwearobj, select_off).
var feetStuckRe = regexp.MustCompile(`Your [a-z]+ (is|are) trapped!|Your [a-z]+ are stuck in the |` +
	`is attached to the buried ball!|prevents you from pulling your |and cannot pull your `)

// feetStuck notes the refusal in the agent (see Agent.FeetStuckTurn).
func feetStuck(a *world.Agent) bool {
	if feetStuckRe.MatchString(a.Message) {
		a.FeetStuckTurn = a.Blstats.Time
		return true
	}
	return false
}

// takeoffF is takeoff(item).
type takeoffF struct {
	Item *world.Item
	PC   int
}

func Takeoff(item *world.Item) Frame { return &takeoffF{Item: item} }

func (s *takeoffF) Resume(c *Ctx) Cmd {
	inv := c.W.Inventory
	if s.PC == 1 {
		return ret(c.Ret)
	}
	item := s.Item
	// TODO: smart
	Assert(item != nil && item.Equipped, item)
	letter := inv.Items.GetLetter(item)
	Assert(item.Status != perception.Item_CURSED, item)

	var equippedArmors []*world.Item
	for _, i := range inv.Items.AllItems {
		if i.IsArmor() && i.Equipped {
			equippedArmors = append(equippedArmors, i)
		}
	}
	Assert(Contains(equippedArmors, item))
	s.PC = 1
	return call(sense.Atom(&takeoffBodyF{Letter: letter, Several: len(equippedArmors) > 1}))
}

type takeoffBodyF struct {
	Letter  string
	Several bool // more than one armor piece is worn: the game asks which one
	PC      int
}

func isTakeOffMessage(a *world.Agent) bool {
	return strings.Contains(a.Message, "You finish taking off ") ||
		strings.Contains(a.Message, "You were wearing ") ||
		strings.Contains(a.Message, "You feel that monsters no longer have difficulty pinpointing your location.")
}

func (s *takeoffBodyF) Resume(c *Ctx) Cmd {
	a := c.W
	switch s.PC {
	case 0:
		s.PC = 1
		return call(a.Term.Key(A_Command_TAKEOFF))
	case 1:
		if s.Several {
			if isTakeOffMessage(a) {
				Panic("env did not ask for the item to takeoff")
			}
			Assert(strings.Contains(a.Message, "What do you want to take off?"), a.Message)
			s.PC = 2
			return call(sense.TypeText(s.Letter))
		}
	}
	if strings.Contains(a.Message, "It is cursed.") || strings.Contains(a.Message, "They are cursed.") {
		return ret(false)
	}
	if feetStuck(a) {
		return ret(false)
	}
	Assert(isTakeOffMessage(a), a.Message)
	return ret(true)
}
