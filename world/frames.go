package world

import (
	"aago/engine"
)

// Frames: the agent's multi-step routines as explicit state (see package engine). A routine that presses
// keys is a frame type; its callers start it with call(...) and read its result in c.Ret.

type (
	Ctx   = engine.Ctx[*Agent]
	Cmd   = engine.Cmd[*Agent]
	Frame = engine.Frame[*Agent]
)
