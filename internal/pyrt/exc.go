package pyrt

import (
	"fmt"
	"runtime/debug"
	"sync"
)

// Python exceptions are Go panics with these values. Anything that is not *AgentPanic /
// *AgentFinished / *AgentChangeStrategy (including Go runtime errors) behaves like a Python
// exception that kills the AutoAscend thread (panic_on_errors=False in the arena adapter).

type PyExc struct {
	Type string // "AssertionError", "ValueError", "KeyError", "IndexError", "TypeError", "ZeroDivisionError", ...
	Msg  string
}

func (e *PyExc) Error() string { return e.Type + ": " + e.Msg }

type AgentPanic struct{ Msg string }

func (e *AgentPanic) Error() string { return "AgentPanic: " + e.Msg }

type AgentFinished struct{}

func (e *AgentFinished) Error() string { return "AgentFinished" }

// AgentChangeStrategy carries the identity of the preempting callback and the continuation
// (the rest of the preempting strategy's generator after its `yield True`): a closure (Cont) from
// goroutine-era code, or a frame (Body) from data strategies. It is never part of the saved state.
type AgentChangeStrategy struct {
	ID   any
	Cont func() any
	Body any
}

func (e *AgentChangeStrategy) Error() string { return "AgentChangeStrategy" }

func Raise(typ string, format string, args ...any) {
	panic(&PyExc{Type: typ, Msg: fmt.Sprintf(format, args...)})
}

// Assert mirrors `assert cond, args`.
func Assert(cond bool, args ...any) {
	if !cond {
		panic(&PyExc{Type: "AssertionError", Msg: fmt.Sprint(args...)})
	}
}

func Panic(format string, args ...any) {
	panic(&AgentPanic{Msg: fmt.Sprintf(format, args...)})
}

// Try runs f and returns the recovered panic value (nil if f returned normally).
func Try(f func()) (r any) {
	defer func() {
		r = recover()
		if r != nil && !IsControl(r) {
			NotePanicSite(r)
		}
	}()
	f()
	return nil
}

// IsControl reports whether r is one of the agent's control-flow exceptions (AgentPanic, AgentFinished,
// AgentChangeStrategy) rather than an error that kills the bot.
func IsControl(r any) bool {
	switch r.(type) {
	case *AgentPanic, *AgentFinished, *AgentChangeStrategy:
		return true
	}
	return false
}

// NotePanicSite remembers where the error r was raised; call it from a deferred recover before re-raising r,
// so that the crash report shows the original site (see PanicStack).
func NotePanicSite(r any) {
	panicSite.Lock()
	if !samePanic(panicSite.v, r) { // nested handlers re-raise the same value: keep the first site
		panicSite.v, panicSite.stack = r, debug.Stack()
	}
	panicSite.Unlock()
}

// panicSite keeps the stack of the last exception Try caught. Try unwinds the stack, so when the
// caller re-raises the exception (Python `raise` in an except block), the crash report would
// otherwise point at the re-raise instead of the line that failed.
var panicSite struct {
	sync.Mutex
	v     any
	stack []byte
}

// PanicStack returns the stack where r was raised if Try caught it on the way up, else nil.
func PanicStack(r any) []byte {
	panicSite.Lock()
	defer panicSite.Unlock()
	if samePanic(panicSite.v, r) {
		return panicSite.stack
	}
	return nil
}

func samePanic(a, b any) (eq bool) {
	defer func() {
		if recover() != nil { // values of incomparable types
			eq = false
		}
	}()
	return a != nil && a == b
}

func IsPyExc(r any, typ string) bool {
	e, ok := r.(*PyExc)
	return ok && e.Type == typ
}
