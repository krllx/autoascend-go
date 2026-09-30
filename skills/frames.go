package skills

import (
	"aago/engine"
	"aago/world"
)

// Frames: the agent's multi-step routines as explicit state (see package engine). A routine that presses
// keys is a frame type; its callers start it with call(...) and read its result in c.Ret.

type (
	Ctx   = engine.Ctx[*world.Agent]
	Cmd   = engine.Cmd[*world.Agent]
	Frame = engine.Frame[*world.Agent]
)

func call(f Frame) Cmd { return engine.Call(f) }

func ret(v any) Cmd { return engine.Return[*world.Agent](v) }
