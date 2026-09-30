package term

import (
	. "aago/internal/pyrt"
	. "aago/nle"
	"aago/world"
)

// ---- dialogs: step(action, generator)
//
// A Python additional_action_iterator generator reads the screen after every key and yields the next one.
// As a frame it is an ordinary routine that sends each yielded key with rawKey (the key and the reading of the
// observation, nothing else); dialogF wraps it: first key, the generator frame, then the rest of the step.
// A raw key belongs to the innermost dialog on the stack (the generator runs inside it).

// dialogF is step(First, Gen()): Gen answers the screens after First; its result is the dialog's result.
type dialogF struct {
	First Action
	Gen   Frame
	Res   any
	Obs   *Obs // the last observation of the dialog and update's `done` for it (see stepF)
	Done  bool
	PC    int
}

func dialog(first Action, gen Frame) Frame { return &dialogF{First: first, Gen: gen} }

func (s *dialogF) Resume(c *Ctx) Cmd {
	switch s.PC {
	case 0:
		s.PC = 1
		return call(rawKey(s.First))
	case 1:
		s.PC = 2
		return call(s.Gen)
	case 2:
		s.Res = c.Ret
		s.PC = 3
		return call(finishStep(s.Obs, s.Done))
	}
	return ret(s.Res)
}

// innermostDialog is the dialog the running generator belongs to.
func innermostDialog(c *Ctx) *dialogF {
	for i := len(c.M.Stack) - 1; i >= 0; i-- {
		if d, ok := c.M.Stack[i].(*dialogF); ok {
			return d
		}
	}
	panic("raw key outside a dialog")
}

// rawKeyF sends one key of the innermost dialog.
type rawKeyF struct {
	Action Action
	PC     int
}

func rawKey(x Action) Frame { return &rawKeyF{Action: x} }

func (s *rawKeyF) Resume(c *Ctx) Cmd {
	a := c.W
	if s.PC == 0 {
		if a.NoStepCalls {
			Raise("ValueError", "Shouldn't call step now")
		}
		s.PC = 1
		return send(s.Action)
	}
	d := innermostDialog(c)
	d.Obs = c.Obs
	d.Done = readObservation(a, c.Obs)
	return ret(nil)
}

// rawTextF sends the characters of Text in the innermost dialog (`yield from text`).
type rawTextF struct {
	Text string
	I    int
}

func rawText(t string) Frame { return &rawTextF{Text: t} }

func (s *rawTextF) Resume(c *Ctx) Cmd {
	if s.I < len(s.Text) {
		s.I++
		return call(rawKey(world.Ch(s.Text[s.I-1])))
	}
	return ret(nil)
}

// ioTerm is the io layer behind Agent.Term: the frames above ask it for the frames that press keys.
type Terminal struct{}

func (Terminal) Key(x Action) Frame { return &stepF{Action: x} }

func (Terminal) Keys(x Action, it world.ActIter) Frame { return &stepF{Action: x, It: it} }

func (Terminal) Dialog(first Action, gen Frame) Frame { return dialog(first, gen) }

func (Terminal) RawKey(x Action) Frame { return rawKey(x) }

func (Terminal) RawText(t string) Frame { return rawText(t) }
