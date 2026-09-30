package world

import (
	. "aago/internal/pyrt"
)

// NewItemPriority mirrors ItemPriority.__init__.
func NewItemPriority(a *Agent) *ItemPriority {
	return &ItemPriority{Agent: a, TakeSacrificialCorpses: false, DropGoldTillTurn: negInf()}
}

// Skeletons for global_logic.py, exploration_logic.py, combat/*, soko_solver. OWNER: logic agent.

type ItemPriority struct {
	Agent                  *Agent
	TakeSacrificialCorpses bool    // _take_sacrificial_corpses
	DropGoldTillTurn       float64 // _drop_gold_till_turn
}

// SplitResult mirrors the dict returned by ItemPriorityBase.split: container (nil = None key) -> counts,
// in insertion order.
type SplitResult = *OMap[*Item, []int]
