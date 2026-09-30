package world

import (
	"math"

	"aago/engine"
	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
)

// Port of autoascend/agent.py. OWNER: lead (do not edit from other files).

type LevelKey struct{ Dungeon, Level int }

type BLStats struct {
	X, Y, StrengthPercentage, Strength, Dexterity, Constitution, Intelligence, Wisdom, Charisma int
	Score, Hitpoints, MaxHitpoints, Depth, Gold, Energy, MaxEnergy, ArmorClass, MonsterLevel    int
	ExperienceLevel, ExperiencePoints, Time, HungerState, CarryingCapacity, DungeonNumber       int
	LevelNumber, PropMask, Alignment                                                            int
}

// Monster mirrors the (distance, y, x, permonst, glyph) tuples of get_visible_monsters.
type Monster struct {
	Dis   int
	Y, X  int
	Mon   *Permonst
	Glyph int
}

// watch is one on_update callback (Python agent.on_update list), identified by its pointer. fire runs the
// check after each step; it may raise AgentPanic / AgentChangeStrategy but must not act.
type Watch interface{ Fire(a *Agent) }

// ActIter mirrors an additional_action_iterator passed to step(). Next may read the agent state, like the
// lazy Python generators it replaces.
type ActIter interface{ Next(a *Agent) (Action, bool) }

// Terminal is the io layer as the rest of the bot sees it: the frames that press keys. Key and Keys are
// agent.step (with an additional action iterator): the key, then whatever the game shows before the step
// counts as done (--More--, menus, [yn]) and the state update. Dialog is step(first, generator): gen answers
// the screens after first with RawKey / RawText, which send a key and read the screen, nothing else.
type Terminal interface {
	Key(x Action) Frame
	Keys(x Action, it ActIter) Frame
	Dialog(first Action, gen Frame) Frame
	RawKey(x Action) Frame
	RawText(t string) Frame
}

type Agent struct {
	M               *engine.Machine[*Agent]
	Term            Terminal
	Rng             *RandomState
	AllPanics       []any
	OnUpdate        []Watch
	Levels          *OMap[LevelKey, *Level]
	StepCount       int
	Observation     *Obs
	Message         string
	SingleMessage   string
	Popup           []string
	SinglePopup     []string
	MessageHistory  []string
	CursorPos       Pos
	LastObservation *Obs
	lastPetSeen     int

	Inventory      *Inventory
	Character      *Character
	Exploration    *ExplorationLogic
	GlobalLogic    *GlobalLogic
	MonsterTracker *MonsterTracker
	StatsLogger    *StatsLogger

	Blstats BLStats

	lastBfsDis               I32Grid
	lastBfsStep              int // None -> -1
	LastPrayerTurn           int // None -> -1
	PreviousObs              *Obs
	LastTurn                 int // _last_turn
	InactivityCounter        int
	IsUpdatingState          bool
	NoStepCalls              bool
	TurnsInAtomOperation     int  // None -> -1
	AtomOperationAllowUpdate int8 // None -1, False 0, True 1
	IsReadingMessageOrPopup  bool
	LastTerrainCheck         int // None -> -1
	ForbiddenEngravePosition Pos

	AllowWalkingThroughTrapsTurn float64 // _allow_walking_through_traps_turn
	AllowAttackAllTurn           float64 // _allow_attack_all_turn
	LastCastFailTurn             map[string]float64

	// FeetStuckTurn: the turn the game refused to put boots on or take them off because the feet are held (a
	// bear trap, stuck in the floor); wear_best_stuff leaves the boots alone for the rest of that turn. Not in
	// the Python bot, which retried forever without the turn passing ("turn inactivity").
	FeetStuckTurn int
}

func (a *Agent) HasPet() bool { return (a.Blstats.Time - a.lastPetSeen) <= 16 }

func (a *Agent) InAtomOperation() bool { return a.TurnsInAtomOperation >= 0 }

func (a *Agent) DisallowStepCalling(f func()) {
	if a.NoStepCalls {
		f()
		return
	}
	a.NoStepCalls = true
	defer func() { a.NoStepCalls = false }()
	f()
}

func (a *Agent) RemoveOnUpdate(fn Watch) {
	for i, g := range a.OnUpdate {
		if g == fn {
			a.OnUpdate = append(a.OnUpdate[:i:i], a.OnUpdate[i+1:]...)
			return
		}
	}
	Raise("ValueError", "list.index(x): x not in list")
}

// CallUpdateFunctions mirrors call_update_functions(funcs) with an explicit list.
func (a *Agent) CallUpdateFunctions(funcs []Watch) {
	for _, f := range funcs {
		Assert(Contains(a.OnUpdate, f))
	}
	a.DisallowStepCalling(func() {
		for _, f := range funcs {
			f.Fire(a)
		}
	})
}

// callAllUpdateFunctions mirrors call_update_functions() (funcs=None: iterate self.on_update live).
func (a *Agent) CallAllUpdateFunctions() {
	a.DisallowStepCalling(func() {
		for i := 0; i < len(a.OnUpdate); i++ {
			a.OnUpdate[i].Fire(a)
		}
	})
}

func (a *Agent) CurrentLevel() *Level {
	key := LevelKey{a.Blstats.DungeonNumber, a.Blstats.LevelNumber}
	l, ok := a.Levels.Get(key)
	if !ok {
		l = NewLevel(key.Dungeon, key.Level)
		a.Levels.Set(key, l)
	}
	return l
}

func NewAgent() *Agent {
	a := &Agent{M: &engine.Machine[*Agent]{}, Rng: NewRandomState(0), Levels: NewOMap[LevelKey, *Level](),
		Popup: []string{}, SinglePopup: []string{},
		lastBfsStep: -1, LastPrayerTurn: -1, LastTurn: -1, TurnsInAtomOperation: -1, AtomOperationAllowUpdate: -1,
		LastTerrainCheck: -1, ForbiddenEngravePosition: Pos{-1, -1},
		AllowWalkingThroughTrapsTurn: math.Inf(-1), AllowAttackAllTurn: math.Inf(-1),
		LastCastFailTurn: map[string]float64{}, StatsLogger: &StatsLogger{}, FeetStuckTurn: -1}
	a.Inventory = NewInventory(a)
	a.Character = NewCharacter(a)
	a.Exploration = &ExplorationLogic{agent: a}
	a.GlobalLogic = NewGlobalLogic(a)
	a.MonsterTracker = NewMonsterTracker(a)
	return a
}

// Ch mirrors step(<one-char str>): A.ACTIONS[A.ACTIONS.index(ord(c))] (ValueError if not an action).
func Ch(c byte) Action {
	for _, v := range ActionsList {
		if v == Action(c) {
			return v
		}
	}
	Raise("ValueError", "%d is not in list", c)
	return 0
}

// Glyphs is the glyph layer of the last observation (nil before the first one).
func (a *Agent) Glyphs() *I16Grid {
	if a.LastObservation == nil {
		return nil
	}
	return &a.LastObservation.Glyphs
}
