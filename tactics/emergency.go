package tactics

import (
	"aago/flow"
	. "aago/nle"
	"aago/skills"
	"aago/world"
)

// ---- emergency_strategy: heal with a potion or pray when HP is low, drink juice or pray when fainting

type Emergency struct{}

func (Emergency) Check(a *world.Agent) (bool, Frame) {
	b := &a.Blstats
	items := a.HealingPotions("healing", "extra healing", "full healing")
	if (float64(b.Hitpoints) < 1.0/3*float64(b.MaxHitpoints) || b.Hitpoints < 8) && len(items) > 0 {
		return true, flow.ThenNil(skills.Eat(items[0], true, true))
	}
	items = a.HealingPotions("fruit juice")
	if len(items) > 0 && b.HungerState >= Hunger_FAINTING {
		return true, flow.ThenNil(skills.Eat(items[0], true, true))
	}
	div := 5.0
	if b.ExperienceLevel >= 6 {
		div = 6
	}
	if (a.IsSafeToPray(500) && (float64(b.Hitpoints) < 1/div*float64(b.MaxHitpoints) || b.Hitpoints < 6)) ||
		(a.IsSafeToPray(400) && b.HungerState >= Hunger_FAINTING) {
		return true, flow.ThenNil(skills.Pray())
	}
	return false, nil
}
