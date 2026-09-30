package tactics

import (
	"aago/flow"
	"aago/perception"
	"aago/skills"
	"aago/world"
)

// ---- cure_disease: lycanthropy

type CureDisease struct{}

func (CureDisease) Check(a *world.Agent) (bool, Frame) {
	if a.Character.IsLycanthrope {
		for it := range world.EachItem(a.Inventory.Items.AllItems) {
			if it.Objs[0].Name == "sprig of wolfsbane" {
				return true, flow.ThenNil(skills.Eat(it, false, true))
			}
		}
		for it := range world.EachItem(a.Inventory.Items.AllItems) {
			if it.Objs[0].Name == "water" && it.Status == perception.Item_BLESSED {
				return true, flow.ThenNil(skills.Eat(it, true, true))
			}
		}
		if a.IsSafeToPray(500) {
			return true, flow.ThenNil(skills.Pray())
		}
	}
	return false, nil
}
