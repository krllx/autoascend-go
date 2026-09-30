// Package archtest checks the architecture rules of the bot: which package may import which, and the
// determinism rules (no map iteration, no goroutines or channels, no writes to package-level variables).
package archtest

import (
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const module = "aago"

// allowed lists the module packages each package may import directly. snapshot is a leaf codec every bot
// package registers its types with. Nothing below brain may import term (the io layer) or brain.
var allowed = map[string][]string{
	"internal/pyrt": {"snapshot"},
	"grid":          {"internal/pyrt"},
	"nle":           {"grid", "internal/pyrt", "snapshot"},
	"engine":        {"internal/pyrt", "nle"},
	"snapshot":      {},
	"perception":    {"grid", "internal/pyrt", "nle", "snapshot"},
	"world":         {"grid", "internal/pyrt", "nle", "snapshot", "engine", "perception"},
	"assess":        {"grid", "internal/pyrt", "nle", "snapshot", "engine", "perception", "world"},
	"flow":          {"grid", "internal/pyrt", "nle", "snapshot", "engine", "perception", "world"},
	"sense":         {"grid", "internal/pyrt", "nle", "snapshot", "engine", "perception", "world", "assess", "flow"},
	"skills":        {"grid", "internal/pyrt", "nle", "snapshot", "engine", "perception", "world", "assess", "flow", "sense"},
	"tactics":       {"grid", "internal/pyrt", "nle", "snapshot", "engine", "perception", "world", "assess", "flow", "sense", "skills"},
	"strategy":      {"grid", "internal/pyrt", "nle", "snapshot", "engine", "perception", "world", "assess", "flow", "sense", "skills", "tactics"},
	"term":          {"grid", "internal/pyrt", "nle", "snapshot", "engine", "perception", "world", "flow", "sense"},
	"brain":         {"grid", "internal/pyrt", "nle", "snapshot", "engine", "perception", "world", "assess", "flow", "sense", "skills", "tactics", "strategy", "term"},
}

func root(t *testing.T) string {
	dir, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

type pkg struct {
	rel   string
	fset  *token.FileSet
	files []*ast.File
	names []string
}

func parse(t *testing.T, rel string) *pkg {
	p := &pkg{rel: rel, fset: token.NewFileSet()}
	matches, _ := filepath.Glob(filepath.Join(root(t), rel, "*.go"))
	for _, m := range matches {
		if !buildFile(m) {
			continue
		}
		f, err := parser.ParseFile(p.fset, m, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		p.files = append(p.files, f)
		p.names = append(p.names, filepath.Base(m))
	}
	return p
}

// buildFile: a non-test file that this build compiles (build tags such as goexperiment.simd decide between
// vec_simd.go and vec_nosimd.go).
func buildFile(path string) bool {
	if strings.HasSuffix(path, "_test.go") {
		return false
	}
	ok, err := build.Default.MatchFile(filepath.Dir(path), filepath.Base(path))
	return err == nil && ok
}

func botPackages() []string {
	var out []string
	for k := range allowed {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestLayers: every import of a module package is allowed, and every package of the module is listed.
func TestLayers(t *testing.T) {
	for _, rel := range botPackages() {
		p := parse(t, rel)
		for i, f := range p.files {
			for _, im := range f.Imports {
				path, _ := strconv.Unquote(im.Path.Value)
				if !strings.HasPrefix(path, module+"/") {
					continue
				}
				dep := strings.TrimPrefix(path, module+"/")
				ok := false
				for _, a := range allowed[rel] {
					ok = ok || a == dep
				}
				if !ok {
					t.Errorf("%s/%s imports %s", rel, p.names[i], dep)
				}
			}
		}
	}
	// a new package must be placed in the table
	filepath.WalkDir(root(t), func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root(t), path)
		switch strings.Split(rel, string(filepath.Separator))[0] {
		case ".", "cmd", "equiv", "tools", "harness", "corpus", "ref", "localcheck", ".git":
			if rel != "." {
				return filepath.SkipDir
			}
			return nil
		}
		// not bot code: corpus reader, code generator, these tests, the scripted game of unit tests
		if rel == "internal/corpus" || rel == "internal/genreg" || rel == "internal/archtest" || rel == "internal/botsim" ||
			rel == "internal" {
			return nil
		}
		if g, _ := filepath.Glob(filepath.Join(path, "*.go")); len(g) > 0 {
			if _, ok := allowed[filepath.ToSlash(rel)]; !ok {
				t.Errorf("package %s is not in the layer table of arch_test.go", rel)
			}
		}
		return nil
	})
}

// TestDeterminism type-checks the bot packages and reports: range over a map (the order changes from run to
// run), go statements, channels and select, and assignments to package-level variables outside init.
func TestDeterminism(t *testing.T) {
	// map ranges whose order is not observable, checked by hand: "<package>/<file>:<function>"
	orderFree := map[string]bool{}
	// package-level variables that are written after all, and why that is safe: "<package>.<var>"
	written := map[string]bool{
		"internal/pyrt.panicSite": true, // where the last Python-style exception was raised (crash reports; mutex)
		"nle.namedGSets":          true, // filled while the package variables are initialized
		"snapshot.reg":            true, // type registries, filled from init functions
	}
	fset := token.NewFileSet()
	imp := importer.ForCompiler(fset, "source", nil)
	for _, rel := range botPackages() {
		matches, _ := filepath.Glob(filepath.Join(root(t), rel, "*.go"))
		var files []*ast.File
		for _, m := range matches {
			if !buildFile(m) {
				continue
			}
			f, err := parser.ParseFile(fset, m, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			files = append(files, f)
		}
		info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Uses: map[*ast.Ident]types.Object{},
			Defs: map[*ast.Ident]types.Object{}}
		conf := types.Config{Importer: imp}
		tp, err := conf.Check(module+"/"+rel, fset, files, info)
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		for _, f := range files {
			file := rel + "/" + filepath.Base(fset.Position(f.Pos()).Filename)
			for _, d := range f.Decls {
				fd, ok := d.(*ast.FuncDecl)
				fname := ""
				if ok {
					fname = fd.Name.Name
				}
				ast.Inspect(d, func(n ast.Node) bool {
					pos := func() string { return fset.Position(n.Pos()).String() }
					switch n := n.(type) {
					case *ast.RangeStmt:
						if _, isMap := info.TypeOf(n.X).Underlying().(*types.Map); isMap && !orderFree[file+":"+fname] {
							t.Errorf("%s: range over a map in %s", pos(), fname)
						}
					case *ast.GoStmt:
						t.Errorf("%s: go statement", pos())
					case *ast.SelectStmt:
						t.Errorf("%s: select", pos())
					case *ast.ChanType:
						t.Errorf("%s: channel type", pos())
					case *ast.AssignStmt:
						if fname == "init" || n.Tok == token.DEFINE {
							return true
						}
						for _, l := range n.Lhs {
							if v := rootVar(l, info); v != nil && v.Parent() == tp.Scope() && !written[rel+"."+v.Name()] {
								t.Errorf("%s: assignment to package-level %s in %s", pos(), v.Name(), fname)
							}
						}
					case *ast.IncDecStmt:
						if v := rootVar(n.X, info); v != nil && v.Parent() == tp.Scope() && !written[rel+"."+v.Name()] {
							t.Errorf("%s: assignment to package-level %s in %s", pos(), v.Name(), fname)
						}
					}
					return true
				})
			}
		}
	}
}

// rootVar is the variable an assignment target starts from (x in x, x[i], x.f, *x).
func rootVar(e ast.Expr, info *types.Info) *types.Var {
	for {
		switch x := e.(type) {
		case *ast.Ident:
			v, _ := info.Uses[x].(*types.Var)
			return v
		case *ast.IndexExpr:
			e = x.X
		case *ast.SelectorExpr:
			if _, isPkg := info.Uses[identOf(x.X)].(*types.PkgName); isPkg {
				v, _ := info.Uses[x.Sel].(*types.Var)
				return v
			}
			e = x.X
		case *ast.StarExpr:
			e = x.X
		case *ast.ParenExpr:
			e = x.X
		default:
			return nil
		}
	}
}

func identOf(e ast.Expr) *ast.Ident {
	id, _ := e.(*ast.Ident)
	return id
}
