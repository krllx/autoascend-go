package tactics

import (
	"aago/flow"
	. "aago/internal/pyrt"
	"aago/skills"
	"aago/world"
)

type arrangeStepper struct{ St *arrangeState }

func (s arrangeStepper) Step(a *world.Agent) Frame { return arrangeStep(a.Inventory, s.St) }

func (s arrangeStepper) Finish(a *world.Agent) { arrangeFinalAsserts(a.Inventory, s.St) }

// ---- pickup_and_drop_items / arrange_items

type pickupAndDropItemsN struct{}

func (pickupAndDropItemsN) Check(a *world.Agent) (bool, Frame) {
	inv := a.Inventory
	// TODO: free (no charge) items
	inv.ItemManager.PriceIdentification()
	if a.CurrentLevel().ShopInterior[a.Blstats.Y][a.Blstats.X] {
		return false, nil
	}
	if len(belowMeLen(inv)) == 0 {
		return false, nil
	}
	return arrangeItemsN{}.Check(a)
}

type arrangeItemsN struct{}

func (arrangeItemsN) Check(a *world.Agent) (bool, Frame) {
	inv := a.Inventory
	if a.Character.Prop.Polymorph() {
		// TODO: only handless
		return false, nil
	}
	st := &arrangeState{}
	act := arrangeStep(inv, st)
	if act == nil {
		arrangeFinalAsserts(inv, st)
		return false, nil
	}
	return true, flow.LoopFrom(arrangeStepper{St: st}, act)
}

// arrangePutF puts AllItems[Indices] into Container.
type arrangePutF struct {
	Container *world.Item
	AllItems  []*world.Item
	Indices   []int
	Counts    []int
	PC        int
}

func (s *arrangePutF) Resume(c *Ctx) Cmd {
	if s.PC == 1 {
		return ret(nil)
	}
	var put []*world.Item
	var putCounts []int
	for _, i := range s.Indices {
		put = append(put, s.AllItems[i])
		putCounts = append(putCounts, s.Counts[i])
	}
	s.PC = 1
	return call(skills.UseContainer(s.Container, put, []*world.Item{}, putCounts, nil))
}

// arrangeDropF drops the surplus of FreeItems[Indices].
type arrangeDropF struct {
	FreeItems []*world.Item
	Indices   []int
	Counts    []int
	PC        int
}

func (s *arrangeDropF) Resume(c *Ctx) Cmd {
	if s.PC == 1 {
		Assert(c.Ret.(bool))
		return ret(nil)
	}
	var its []*world.Item
	var cs []int
	for _, i := range s.Indices {
		its = append(its, s.FreeItems[i])
	}
	for _, i := range s.Indices {
		cs = append(cs, s.FreeItems[i].Count-s.Counts[i])
	}
	s.PC = 1
	return call(skills.Drop(its, cs, false))
}

// arrangeTakeF takes AllItems[Indices] out of Container, as many as there are free slots.
type arrangeTakeF struct {
	Container  *world.Item
	AllItems   []*world.Item
	Indices    []int
	TakeCounts []int
	PC         int
}

func (s *arrangeTakeF) Resume(c *Ctx) Cmd {
	if s.PC == 1 {
		return ret(nil)
	}
	inv := c.W.Inventory
	Assert(inv.Items.FreeSlots() > 0)
	indices := s.Indices[:min(len(s.Indices), inv.Items.FreeSlots())]
	var take []*world.Item
	for _, i := range indices {
		take = append(take, s.AllItems[i])
	}
	s.PC = 1
	return call(skills.UseContainer(s.Container, []*world.Item{}, take, nil, s.TakeCounts))
}

// arrangePickupF picks up ItemsBelowMe[Indices].
type arrangePickupF struct {
	ItemsBelowMe []*world.Item
	Indices      []int
	ToPickup     []int
	PC           int
}

func (s *arrangePickupF) Resume(c *Ctx) Cmd {
	if s.PC == 1 {
		return ret(nil)
	}
	var its []*world.Item
	var cs []int
	for _, i := range s.Indices {
		its = append(its, s.ItemsBelowMe[i])
	}
	for _, i := range s.Indices {
		cs = append(cs, s.ToPickup[i])
	}
	s.PC = 1
	return call(skills.Pickup(its, cs))
}
