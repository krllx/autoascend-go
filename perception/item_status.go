package perception

import (
	. "aago/nle"
)

// Port of autoascend/item/inventory.py (actions part). Strategies are in inventory_strategy.go.

var InventoryNameToCategory = map[string]int{
	"Amulets":          NH_AMULET_CLASS,
	"Armor":            NH_ARMOR_CLASS,
	"Comestibles":      NH_FOOD_CLASS,
	"Coins":            NH_COIN_CLASS,
	"Gems/Stones":      NH_GEM_CLASS,
	"Potions":          NH_POTION_CLASS,
	"Rings":            NH_RING_CLASS,
	"Scrolls":          NH_SCROLL_CLASS,
	"Spellbooks":       NH_SPBOOK_CLASS,
	"Tools":            NH_TOOL_CLASS,
	"Weapons":          NH_WEAPON_CLASS,
	"Wands":            NH_WAND_CLASS,
	"Boulders/Statues": NH_ROCK_CLASS,
	"Chains":           NH_CHAIN_CLASS,
	"Iron balls":       NH_BALL_CLASS,
}

// Port of autoascend/item/item.py and item/utils.py.  OWNER: item agent (replace TODO stubs).

const (
	Item_UNKNOWN  = 0
	Item_CURSED   = 1
	Item_UNCURSED = 2
	Item_BLESSED  = 3

	Item_NOT_SHOP = 0
	Item_FOR_SALE = 1
	Item_UNPAID   = 2
)

func ObjsEqual(a, b []*Obj) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
