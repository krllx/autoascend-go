package tactics

import (
	"sort"

	"aago/assess"
	. "aago/internal/pyrt"
	"aago/perception"
	"aago/world"
)

// ######## LOW-LEVEL STRATEGIES

// belowMeIter mirrors iterating self.items_below_me (TypeError when None).
func belowMeIter(inv *world.Inventory) []*world.Item {
	if inv.ItemsBelowMeNone {
		Raise("TypeError", "'NoneType' object is not iterable")
	}
	return inv.ItemsBelowMe
}

type arrangeState struct {
	itemSplit world.SplitResult
	allItems  []*world.Item
}

// arrangeStep runs one iteration of arrange_items' `while 1` loop up to the action decision. It returns
// the action to run after `yield True` (nil means `break`).
func arrangeStep(inv *world.Inventory, st *arrangeState) Frame {
	a := inv.Agent
	var itemsBelowMe []*world.Item
	for i := range world.EachItem(belowMeIter(inv)) {
		if i.ShopStatus == perception.Item_NOT_SHOP {
			itemsBelowMe = append(itemsBelowMe, i)
		}
	}
	forcedItems := []*world.Item{}
	for i := range world.EachItem(inv.Items.AllItems) {
		if !i.CanBeDroppedFromInventory() {
			forcedItems = append(forcedItems, i)
		}
	}
	for _, item := range forcedItems {
		Assert(inv.InAllItems(item))
	}
	sortedInv := append([]*world.Item{}, inv.Items.AllItems...)
	for _, x := range sortedInv {
		if x.TextNone {
			Raise("TypeError", "'<' not supported between instances of 'NoneType' and 'str'")
		}
	}
	sort.SliceStable(sortedInv, func(i, j int) bool { return sortedInv[i].Text < sortedInv[j].Text })
	freeItems := []*world.Item{}
	for i := range world.EachItem(sortedInv) {
		if i.CanBeDroppedFromInventory() {
			freeItems = append(freeItems, i)
		}
	}
	allItems := append(append([]*world.Item{}, freeItems...), itemsBelowMe...)

	itemSplit := assess.Split(a.GlobalLogic.ItemPriority, allItems, forcedItems, float64(a.Character.CarryingCapacity()))
	st.itemSplit, st.allItems = itemSplit, allItems

	for _, container := range itemSplit.Keys() {
		ok := container == nil || inv.InBelowMe(container) || inv.InAllItems(container)
		if !ok {
			s := 0
			for _, c := range itemSplit.MustGet(container) {
				s += c
			}
			ok = s == 0 && len(container.Content.Items) == 0
		}
		Assert(ok, "TODO: nested containers")
	}

	// put into containers
	for _, container := range itemSplit.Keys() {
		if container != nil {
			counts := itemSplit.MustGet(container)
			var indices []int
			for i, item := range allItems {
				if inv.InAllItems(item) && counts[i] > 0 {
					indices = append(indices, i)
				}
			}
			if len(indices) == 0 {
				continue
			}
			return &arrangePutF{Container: container, AllItems: allItems, Indices: indices, Counts: counts}
		}
	}

	// drop on ground
	{
		counts := itemSplit.MustGet(nil)
		var indices []int
		for i, item := range freeItems {
			if inv.InAllItems(item) && counts[i] != item.Count {
				indices = append(indices, i)
			}
		}
		if len(indices) > 0 {
			return &arrangeDropF{FreeItems: freeItems, Indices: indices, Counts: counts}
		}
	}

	// take from container
	for _, container := range allItems {
		if !container.IsContainer() {
			continue
		}

		var indices []int
		var itemsToTakeCounts []int
		if itemSplit.Has(container) {
			counts := itemSplit.MustGet(container)
			for i, item := range allItems {
				if Contains(container.Content.Items, item) && counts[i] != item.Count {
					indices = append(indices, i)
				}
			}
			for _, i := range indices {
				itemsToTakeCounts = append(itemsToTakeCounts, allItems[i].Count-counts[i])
			}
		} else {
			counts := assess.SumCols(itemSplit.Values())
			for i, item := range allItems {
				if Contains(container.Content.Items, item) && counts[i] != 0 {
					indices = append(indices, i)
				}
			}
			for _, i := range indices {
				itemsToTakeCounts = append(itemsToTakeCounts, counts[i])
			}
		}

		if len(indices) == 0 {
			continue
		}
		return &arrangeTakeF{Container: container, AllItems: allItems, Indices: indices, TakeCounts: itemsToTakeCounts}
	}

	// pick up from ground
	var rows [][]int
	for _, counts := range itemSplit.Values() {
		rows = append(rows, counts[min(len(freeItems), len(counts)):])
	}
	toPickup := assess.SumCols(rows)
	Assert(len(toPickup) == len(itemsBelowMe))
	var indices []int
	for i, item := range itemsBelowMe {
		if toPickup[i] > 0 && inv.InBelowMe(item) {
			indices = append(indices, i)
		}
	}
	if len(indices) > 0 {
		Assert(inv.Items.FreeSlots() > 0)
		indices = indices[:min(len(indices), inv.Items.FreeSlots())]
		return &arrangePickupF{ItemsBelowMe: itemsBelowMe, Indices: indices, ToPickup: toPickup}
	}

	return nil
}

func arrangeFinalAsserts(inv *world.Inventory, st *arrangeState) {
	for _, container := range st.itemSplit.Keys() {
		counts := st.itemSplit.MustGet(container)
		for j := 0; j < min(len(st.allItems), len(counts)); j++ {
			item, count := st.allItems[j], counts[j]
			Assert(count == 0 || count == item.Count)
			if count != 0 {
				if container != nil {
					Assert(Contains(container.Content.Items, item))
				} else {
					Assert(inv.InAllItems(item))
				}
			}
		}
	}
}
