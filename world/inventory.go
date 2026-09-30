package world

import (
	. "aago/internal/pyrt"
	. "aago/nle"
)

type Inventory struct {
	Agent       *Agent
	ItemManager *ItemManager
	Items       *InventoryItems

	PreviousBlstats     BLStats
	PreviousBlstatsNone bool

	ItemsBelowMe     []*Item // None -> nil (check ItemsBelowMeNone)
	ItemsBelowMeNone bool
	// LettersBelowMe: "" is a None letter (the 'You see here' case); the list itself may be None.
	LettersBelowMe       []string
	LettersBelowMeNone   bool
	EngravingBelowMe     string // None -> "" (check EngravingBelowMeNone)
	EngravingBelowMeNone bool

	SkipEngraveCounter int
}

func NewInventory(a *Agent) *Inventory {
	inv := &Inventory{Agent: a}
	inv.ItemManager = NewItemManager(a)
	inv.Items = NewInventoryItems(a)

	inv.PreviousBlstatsNone = true
	inv.ItemsBelowMe, inv.ItemsBelowMeNone = nil, true
	inv.LettersBelowMe, inv.LettersBelowMeNone = nil, true
	inv.EngravingBelowMe, inv.EngravingBelowMeNone = "", true

	inv.SkipEngraveCounter = 0
	return inv
}

// ---- small Python-semantics helpers

// inBelowMe mirrors `item in self.items_below_me` (TypeError when it is None).
func (inv *Inventory) InBelowMe(item *Item) bool {
	if inv.ItemsBelowMeNone {
		Raise("TypeError", "argument of type 'NoneType' is not iterable")
	}
	return Contains(inv.ItemsBelowMe, item)
}

func (inv *Inventory) InAllItems(item *Item) bool { return Contains(inv.Items.AllItems, item) }

func StrPtrEq(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// pyStrOfID mirrors f'{container_id}' (None -> 'None').
func PyStrOfID(s *string) string {
	if s == nil {
		return "None"
	}
	return *s
}

// itemTextStr mirrors f'{item.text}'.
func ItemTextStr(it *Item) string {
	if it.TextNone {
		return "None"
	}
	return it.Text
}

// objBi mirrors obj.bi (only Weapon and WepTool namedtuples have it).
func ObjBi(o *Obj) int {
	if o.Kind != KWeapon && o.Kind != KWepTool {
		Raise("AttributeError", "object has no attribute 'bi'")
	}
	return o.Bi
}

func chAction(c byte) Action { return Ch(c) }

// yieldStr yields each char of s as an action (Python `yield from s`).
func yieldStr(s string, yield func(Action) bool) bool {
	for i := 0; i < len(s); i++ {
		if !yield(Ch(s[i])) {
			return false
		}
	}
	return true
}

func (inv *Inventory) CurrentPositionID() *string {
	a := inv.Agent
	k := a.CurrentLevel().Key()
	return PositionID(k.Dungeon, k.Level, a.Blstats.Y, a.Blstats.X)
}

// Port of autoascend/item/inventory_items.py.

type InventoryItems struct {
	agent                                                      *Agent
	PreviousInvStrs                                            [55][80]uint8
	PreviousInvStrsNone                                        bool
	MainHand, OffHand, Suit, Helm, Gloves, Boots, Cloak, Shirt *Item
	TotalWeight                                                float64
	AllItems                                                   []*Item
	AllLetters                                                 []byte
	RecheckContainers                                          bool
}

func NewInventoryItems(a *Agent) *InventoryItems {
	ii := &InventoryItems{agent: a, PreviousInvStrsNone: true}
	ii.Clear()
	return ii
}

func (ii *InventoryItems) Clear() {
	ii.MainHand = nil
	ii.OffHand = nil
	ii.Suit = nil
	ii.Helm = nil
	ii.Gloves = nil
	ii.Boots = nil
	ii.Cloak = nil
	ii.Shirt = nil

	ii.TotalWeight = 0

	ii.AllItems = []*Item{}
	ii.AllLetters = []byte{}

	ii.RecheckContainers = true
}

func (ii *InventoryItems) TotalNutrition() int {
	ret := 0
	for _, item := range ii.AllItems {
		if item.IsFood() {
			ret += item.Object().Nutrition * item.Count
		}
	}
	return ret
}

func (ii *InventoryItems) FreeSlots() int {
	// isinstance(item, O.Coin) is always False for Item instances
	isCoin := false
	return 52 + B2i(isCoin) - len(ii.AllItems)
}

func (ii *InventoryItems) OnPanic() {
	ii.PreviousInvStrsNone = true
	ii.Clear()
}

type InvEntry struct {
	Name     string
	Category int
	Glyph    int
	Letter   byte
}

func (ii *InventoryItems) GetLetter(item *Item) string {
	Assert(Contains(ii.AllItems, item), item)
	return string(ii.AllLetters[Index(ii.AllItems, item)])
}
