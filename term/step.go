package term

import (
	"strings"

	. "aago/grid"
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/perception"
	"aago/sense"
	"aago/world"
)

// stepF is agent.step(action, additional_action_iterator) together with agent.update(observation): send a
// key, then finish whatever the game shows before the step counts as done (--More--, a menu, a [yn]
// question, a wish) by sending more keys, and finally update the agent state. In Python step and update call
// each other recursively and always return right after the inner call; here the tail call is a loop.
type stepF struct {
	Action Action
	It     world.ActIter // nil: no iterator
	// the last observation of this step and update's `done` for it: Python keeps them in update()'s locals,
	// so steps nested inside the iterator (a generator may call type_text) do not change them
	Obs  *Obs
	Done bool
	PC   int // 0: send Action, 1: an observation arrived, 2: [yn] answer or state update done, 3: keys are over
}

// finishStep continues update() after the keys of a dialog are over (see dialogF): it starts at the checks
// for --More--, menus and questions, with the dialog's last observation.
func finishStep(obs *Obs, done bool) Frame { return &stepF{Obs: obs, Done: done, PC: 3} }

func (s *stepF) Resume(c *Ctx) Cmd {
	a := c.W
	if s.PC == 2 {
		return ret(nil)
	}
	for {
		switch s.PC {
		case 0:
			if a.NoStepCalls {
				Raise("ValueError", "Shouldn't call step now")
			}
			s.PC = 1
			return send(s.Action)
		case 1:
			s.Obs = c.Obs
			s.Done = readObservation(a, c.Obs)
			s.PC = 3
			if s.It != nil {
				if next, ok := s.It.Next(a); ok {
					s.Action, s.PC = next, 0
					continue
				}
			}
		}

		// update(obs, it) after the iterator: every `self.step(...); return` is the next loop iteration
		obs := s.Obs
		s.PC = 0
		if !s.Done || obs.Misc[2] != 0 {
			s.Action, s.It = A_TextCharacters_SPACE, nil
			continue
		}
		if obs.Misc[1] != 0 {
			if strings.Contains(a.Message, "You may wish for an object.") {
				s.Action, s.It = world.Ch('b'), world.IterStr("lessed greased +2 gray dragon scale mail\r")
				continue
			}
			s.Action, s.It = A_Command_ESC, nil
			continue
		}
		if strings.Contains(a.Message, "Where do you want to be teleported?") {
			s.Action, s.It = A_Command_ESC, nil
			continue
		}
		if perception.TtyContains(obs, "[yn]") {
			s.PC = 2
			return call(sense.TypeText("y"))
		}
		a.IsReadingMessageOrPopup = false
		a.MessageHistory = append(a.MessageHistory, a.Message)
		if len(a.MessageHistory) > 400 {
			a.MessageHistory = append([]string(nil), a.MessageHistory[len(a.MessageHistory)-100:]...)
		}
		if a.LastObservation == nil {
			a.LastObservation = obs
			a.PreviousObs = a.LastObservation
		} else {
			a.PreviousObs = a.LastObservation
			a.LastObservation = obs
		}
		b := &obs.Blstats
		a.Blstats = world.BLStats{int(b[0]), int(b[1]), int(b[2]), int(b[3]), int(b[4]), int(b[5]), int(b[6]), int(b[7]), int(b[8]),
			int(b[9]), int(b[10]), int(b[11]), int(b[12]), int(b[13]), int(b[14]), int(b[15]), int(b[16]), int(b[17]),
			int(b[18]), int(b[19]), int(b[20]), int(b[21]), int(b[22]), int(b[23]), int(b[24]), int(b[25]), int(b[26])}
		a.CurrentLevel() // stats_logger.log_cumulative_value touches current_level()

		a.InactivityCounter++
		if a.LastTurn != a.Blstats.Time {
			a.LastTurn = a.Blstats.Time
			a.InactivityCounter = 0
		}
		Assert(a.InactivityCounter < 200, "turn inactivity")
		allowUpdate := a.AtomOperationAllowUpdate == 1 || !a.InAtomOperation()
		s.PC = 2
		return call(sense.UpdateState(allowUpdate, !a.InAtomOperation()))
	}
}

// readObservation is the part of step + update that runs for every observation, before the iterator is
// asked for the next key: count the step and accumulate the message and popup. Returns update's `done`.
func readObservation(a *world.Agent, obs *Obs) bool {
	a.StepCount++
	a.CursorPos = Pos{int(obs.TtyCursor[0]) - 1, int(obs.TtyCursor[1])}
	a.Observation = obs
	done := a.UpdateMessageAndPopup(obs)
	a.IsReadingMessageOrPopup = true
	return done
}
