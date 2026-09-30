// Package engine runs the bot's multi-step routines as explicit frames.
//
// A frame is an unfinished routine: its local variables and the place where it stopped (usually a PC field),
// as a plain struct. Frames sit on an explicit stack (Machine.Stack) instead of a goroutine stack or closures,
// so the whole state of the bot is data that can be copied and saved. A frame never waits: Resume runs it up
// to its next command - start a child routine (Call), finish (Return) or hand a key to the game (Send). On
// Send the machine stops; Feed resumes it with the next observation.
//
// Exceptions stay Go panics inside a single Resume (pyrt.Assert, pyrt.Panic, ...). The machine catches them at
// the frame boundary and unwinds the explicit stack the way Python unwinds its call stack: every frame gets
// Catch (try/except) and Unwind (finally) in turn from the top. Errors that are not control exceptions (see
// pyrt.IsControl) kill the bot: they are re-raised at once, and so is a control exception no frame catches.
//
// The package knows nothing about NetHack beyond actions and observations; W is the bot state the frames
// work on.
package engine

import (
	"fmt"

	"aago/internal/pyrt"
	"aago/nle"
)

// Frame is one unfinished multi-step routine. Frames are plain data: no funcs, channels or pointers into the
// middle of other structs, so that a snapshot of the stack can be taken at any Send.
type Frame[W any] interface {
	// Resume continues the routine up to its next command. c.Ret is the result of the child that has just
	// returned; after a Send, c.Obs is the observation that answered it.
	Resume(c *Ctx[W]) Cmd[W]
}

// Unwinder is a frame that must clean up when an exception unwinds it (Python finally / Go defer).
type Unwinder[W any] interface{ Unwind(c *Ctx[W]) }

// Catcher is a frame with a try/except around its children (or around its own code). Catch is offered every
// control exception that reaches the frame; it returns true to handle it, and the frame is resumed next
// (it sets its own PC). On false the exception goes on up and Unwind is called as usual.
type Catcher[W any] interface{ Catch(c *Ctx[W], exc any) bool }

// Ctx is what a frame sees while it runs. It is not part of the state.
type Ctx[W any] struct {
	W   W
	M   *Machine[W]
	Ret any      // result of the child that has just returned
	Obs *nle.Obs // observation after a Send (nil otherwise)
}

type op uint8

const (
	opCall op = iota + 1
	opReturn
	opSend
)

// Cmd is what a frame asks the machine to do next.
type Cmd[W any] struct {
	op    op
	child Frame[W]
	val   any
	act   nle.Action
}

// Call starts the child routine f; the caller is resumed with c.Ret = f's result.
func Call[W any](f Frame[W]) Cmd[W] { return Cmd[W]{op: opCall, child: f} }

// Return finishes the routine with result v.
func Return[W any](v any) Cmd[W] { return Cmd[W]{op: opReturn, val: v} }

// Send hands action a to the game; the frame is resumed with the observation in c.Obs.
func Send[W any](a nle.Action) Cmd[W] { return Cmd[W]{op: opSend, act: a} }

// Machine is the explicit call stack of the bot. Between two observations the top frame is the one that
// sent the last action (it resumes with the next observation), so a copy of the machine at that moment
// holds everything about where the bot is.
type Machine[W any] struct {
	Stack []Frame[W]
	// Ops counts frame operations since the last Send; OpLimit > 0 turns a routine that loops forever without
	// acting into a Hang error (the goroutine-based agent relied on the arena's 118 s action timeout instead).
	Ops, OpLimit int
	// MaxOps is the largest Ops seen before a Send (diagnostics; not part of the state).
	MaxOps int `snapshot:"-"`
}

// Hang is raised when OpLimit frame operations pass without an action.
type Hang struct{ Ops int }

func (h *Hang) Error() string { return fmt.Sprintf("no action after %d frame operations", h.Ops) }

// Start pushes the root frame and runs until its first action. ok is false if the root returned instead.
func (m *Machine[W]) Start(w W, root Frame[W]) (a nle.Action, ok bool) {
	m.Stack = append(m.Stack[:0], root)
	return m.run(&Ctx[W]{W: w, M: m})
}

// Feed answers the pending action with obs and runs until the next action.
func (m *Machine[W]) Feed(w W, obs *nle.Obs) (a nle.Action, ok bool) {
	if len(m.Stack) == 0 {
		panic("engine: Feed without a pending action")
	}
	return m.run(&Ctx[W]{W: w, M: m, Obs: obs})
}

// run resumes the top frame (with c.Ret / c.Obs as prepared) and executes commands until a Send.
func (m *Machine[W]) run(c *Ctx[W]) (nle.Action, bool) {
	for len(m.Stack) > 0 {
		top := m.Stack[len(m.Stack)-1]
		cmd, exc := m.resume(top, c)
		c.Ret, c.Obs = nil, nil
		if exc != nil {
			m.unwind(c, exc, 0)
			continue
		}
		m.count()
		switch cmd.op {
		case opCall:
			m.Stack = append(m.Stack, cmd.child)
		case opReturn:
			m.Stack[len(m.Stack)-1] = nil
			m.Stack = m.Stack[:len(m.Stack)-1]
			c.Ret = cmd.val
		case opSend:
			m.MaxOps = max(m.MaxOps, m.Ops)
			m.Ops = 0
			return cmd.act, true
		default:
			panic(fmt.Sprintf("engine: frame %T returned an empty Cmd", top))
		}
	}
	return 0, false
}

func (m *Machine[W]) count() {
	m.Ops++
	if m.OpLimit > 0 && m.Ops > m.OpLimit {
		panic(&Hang{Ops: m.Ops})
	}
}

// resume runs one Resume and turns a control exception into a value; other errors are re-raised.
func (m *Machine[W]) resume(f Frame[W], c *Ctx[W]) (cmd Cmd[W], exc any) {
	defer func() {
		if r := recover(); r != nil {
			if !pyrt.IsControl(r) {
				pyrt.NotePanicSite(r)
				panic(r)
			}
			exc = r
		}
	}()
	return f.Resume(c), nil
}

// unwind offers exc to the frames above base from the top; the first Catcher that takes it stays on top and
// runs next. If none does, the segment is emptied and exc is re-raised.
func (m *Machine[W]) unwind(c *Ctx[W], exc any, base int) {
	for len(m.Stack) > base {
		top := m.Stack[len(m.Stack)-1]
		if h, ok := top.(Catcher[W]); ok && m.catch(h, c, &exc) {
			return
		}
		if u, ok := top.(Unwinder[W]); ok {
			m.cleanup(u, c, &exc)
		}
		m.Stack[len(m.Stack)-1] = nil
		m.Stack = m.Stack[:len(m.Stack)-1]
	}
	panic(exc)
}

// catch and cleanup run handler code; an exception raised there replaces the one being handled (as in Python).
func (m *Machine[W]) catch(h Catcher[W], c *Ctx[W], exc *any) (handled bool) {
	defer func() {
		if r := recover(); r != nil {
			if !pyrt.IsControl(r) {
				pyrt.NotePanicSite(r)
				panic(r)
			}
			*exc, handled = r, false
		}
	}()
	return h.Catch(c, *exc)
}

func (m *Machine[W]) cleanup(u Unwinder[W], c *Ctx[W], exc *any) {
	defer func() {
		if r := recover(); r != nil {
			if !pyrt.IsControl(r) {
				pyrt.NotePanicSite(r)
				panic(r)
			}
			*exc = r
		}
	}()
	u.Unwind(c)
}
