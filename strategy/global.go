package strategy

import (
	"aago/flow"
	"aago/tactics"
	"aago/world"
)

// ---- milestones

// milestone is one row of current_strategy's plan: the level to go to until Done holds.
type milestone struct {
	Done  flow.Pred
	Level world.LevelKey
}

func milestones(m int) milestone {
	switch m {
	case world.Milestone_BE_ON_FIRST_LEVEL:
		return milestone{xpAtLeast{8}, world.LevelKey{world.Level_DUNGEONS_OF_DOOM, 1}}
	case world.Milestone_FIND_SOKOBAN:
		return milestone{inDungeon{world.Level_SOKOBAN}, world.LevelKey{world.Level_SOKOBAN, 4}}
	case world.Milestone_FIND_GNOMISH_MINES:
		return milestone{inDungeon{world.Level_GNOMISH_MINES}, world.LevelKey{world.Level_GNOMISH_MINES, 1}}
	case world.Milestone_FIND_MINETOWN:
		return milestone{minetownFound{}, world.LevelKey{world.Level_GNOMISH_MINES, 4}} // TODO
	case world.Milestone_SOLVE_SOKOBAN:
		// TODO: fix the condition, monster can destroy doors
		return milestone{sokobanTopSolved{}, world.LevelKey{world.Level_SOKOBAN, 1}}
	case world.Milestone_FIND_MINES_END:
		return milestone{atLevel{world.LevelKey{world.Level_GNOMISH_MINES, 9}}, world.LevelKey{world.Level_GNOMISH_MINES, 9}} // TODO
	}
	return milestone{never{}, world.LevelKey{world.Level_DUNGEONS_OF_DOOM, 100}} // TODO
}

// ---- strategies

// explorationStrategy is current_strategy's exploration_strategy(level).
func explorationStrategy(g *world.GlobalLogic, level *int) flow.Node {
	return flow.Preempt(&explorationBaseN{Level: level}, []flow.Node{
		flow.Every(100, tactics.IdentifyOnAltar{}),
		flow.If(onAltar{}, tactics.IdentifyOnAltar{}),
		flow.Every(10, flow.If(xpAtLeast{7}, tactics.DipForExcalibur{})),
	}, true)
}

// explorationBaseN builds explore1 anew at every check, like the Python lambda.
type explorationBaseN struct{ Level *int }

func (n *explorationBaseN) Check(a *world.Agent) (bool, Frame) {
	o := world.DefaultExploreOpts()
	o.SearchPrioLimit = n.Level
	o.TrapSearchOffset = 1
	o.KickDoors = a.CurrentLevel().DungeonNumber != world.Level_GNOMISH_MINES
	return tactics.Explore1(a, o).Check(a)
}

// goToStrategy is current_strategy's go_to_strategy(y, x): explore, and walk to (y, x) (gathering items on
// the way) unless there is an altar to sacrifice at, until there.
func goToStrategy(g *world.GlobalLogic, y, x int) flow.Node {
	a := g.Agent
	return flow.Until(flow.Preempt(explorationStrategy(g, nil), []flow.Node{
		flow.If(noCoalignedAltarOrArtifact{}, flow.Preempt(goToStrategyN{Y: y, X: x}, []flow.Node{
			tactics.GatherItems(a.Inventory),
			tactics.IdentifyOnAltar{},
			flow.If(xpAtLeast{7}, tactics.DipForExcalibur{}),
		}, true)),
	}, true), atPos{y, x})
}

// currentStrategyN is current_strategy: walk the milestones.
type currentStrategyN struct{}

func (currentStrategyN) Check(a *world.Agent) (bool, Frame) { return true, &currentStrategyF{} }

type currentStrategyF struct{ PC int }

func (s *currentStrategyF) Resume(c *Ctx) Cmd {
	a := c.W
	g := a.GlobalLogic
	for {
		m := milestones(g.Milestone)
		if m.Done.Eval(a) {
			g.Milestone = world.MilestoneNext(g.Milestone)
			continue
		}
		zero := 0
		plan := flow.Until(flow.Preempt(flow.Preempt(
			flow.Before(&goToLevelN{DN: m.Level.Dungeon, LN: m.Level.Level, F: globalGoTo{}, Explore: explorationStrategy(g, nil)},
				explorationStrategy(g, nil)), //.before(self.agent.exploration.patrol())
			[]flow.Node{
				explorationStrategy(g, &zero),
				flow.Until(explorationStrategy(g, nil), hpRecovered{}),
			}, true),
			[]flow.Node{flow.If(never{}, exploreStairsN{F: globalGoTo{}, All: true})}, true),
			m.Done)
		return call(flow.RunNode(plan))
	}
}

// globalLayers are the preempting layers of global_strategy, lowest priority first: each layer may interrupt
// everything below it (emergency beats fighting beats eating beats exploring).
func globalLayers(g *world.GlobalLogic) [][]flow.Node {
	return [][]flow.Node{
		{flow.If(sokobanActive{}, solveSokobanN{})},
		{flow.Preempt(tactics.OfferCorpses{}, []flow.Node{flow.If(notHungry{}, tactics.EatCorpses{OnlyBelowMe: true})}, true)},
		{tactics.WaitOut{}},
		{flow.Every(5, tactics.CureDisease{})},
		{
			flow.If(notHungry{}, tactics.EatCorpses{OnlyBelowMe: true}),
			flow.If(notHungry{}, flow.Every(5, tactics.EatCorpses{OnlyBelowMe: true})),
			flow.Every(5, tactics.EatFromInventory{}),
		},
		{tactics.FollowGuard{}},
		{tactics.Fight2{}},
		{tactics.EngulfedFight{}},
		{tactics.Emergency{}},
	}
}

// globalStrategy is global_strategy: the milestones, under every layer.
func GlobalStrategy(g *world.GlobalLogic) flow.Node {
	n := flow.Repeat(currentStrategyN{})
	for _, layer := range globalLayers(g) {
		n = flow.Preempt(n, layer, true)
	}
	return n
}
