//go:build !(goexperiment.simd && amd64)

package grid

// Without GOEXPERIMENT=simd on amd64 there is no vector code: every bulk operation runs its plain loop.

// SIMDOn reports whether the bulk operations of vec.go run their SIMD versions.
func SIMDOn() bool { return false }

func inRanges2SIMD(g *I16Grid, r0, r1 Range16, out *BoolGrid, or bool) { panic("no SIMD") }
func inRanges4SIMD(g *I16Grid, r [4]Range16, out *BoolGrid, or bool)   { panic("no SIMD") }
func anyInRanges2SIMD(g *I16Grid, r0, r1 Range16) bool                 { panic("no SIMD") }
func anyInRanges4SIMD(g *I16Grid, r [4]Range16) bool                   { panic("no SIMD") }
func selectI16SIMD(g, src *I16Grid, m *BoolGrid, v int16, constant bool) {
	selectI16Plain(g, src, m, v, constant)
}
func setWhereI32SIMD(g *I32Grid, m *BoolGrid, v int32) { setWhereI32Plain(g, m, v) }
func cmpI32SIMD(g *I32Grid, v int32, op cmpOp, not bool, out *BoolGrid) {
	cmpI32Plain(g, v, op, not, out)
}
func mulI32SIMD(g, o, out *I32Grid)                      { mulI32Plain(g, o, out) }
func mulScalarI32SIMD(g *I32Grid, c int32, out *I32Grid) { mulScalarI32Plain(g, c, out) }
func appendPositiveSIMD(g *I32Grid, dst []int32) []int32 { return appendPositivePlain(g, dst) }
func fullF32SIMD(v float32, out *F32Grid)                { fullF32Plain(v, out) }
func subI32SIMD(g *F32Grid, v *I32Grid)                  { subI32Plain(g, v) }
func addWhereSIMD(g *F32Grid, m *BoolGrid, c float32)    { addWherePlain(g, m, c) }
func setWhereSIMD(g *F32Grid, m *BoolGrid, c float32)    { setWherePlain(g, m, c) }
func geF32SIMD(g *F32Grid, v float64, out *BoolGrid)     { geF32Plain(g, v, out) }
func boolOpSIMD(a, b *[cells]byte, op boolOpKind)        { boolOpWords(a, b, op, 0) }
func ltF32SIMD(g *F32Grid, v float64, out *BoolGrid)     { ltF32Plain(g, v, out) }
func eqF32SIMD(g *F32Grid, v float32, out *BoolGrid)     { eqF32Plain(g, v, out) }
func isFiniteF32SIMD(g *F32Grid, out *BoolGrid)          { isFiniteF32Plain(g, out) }
