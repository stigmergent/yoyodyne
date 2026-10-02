package ownership

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/constant"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
)

const ownershipImport = "github.com/mason-bryant/yoyodyne/internal/ownership"

// The operator's mover value is named only in this package. Scan every non-test
// Go file outside it, including constant expressions in mover-typed contexts.
// Go's type information follows aliases, composite fields, assignments, calls,
// and returns; syntax alone misses contexts with an inferred type.
func TestTheOperatorsMoverIsNamedOnlyInTheOwnershipPackage(t *testing.T) {
	root := moduleRoot(t)
	here, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	fileSet := token.NewFileSet()
	filesByDir := map[string][]*ast.File{}
	paths := map[*ast.File]string{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if path != root && (strings.HasPrefix(name, ".") || name == "testdata" || name == "vendor" || name == "node_modules" || path == here) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fileSet, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		dir := filepath.Dir(path)
		filesByDir[dir] = append(filesByDir[dir], file)
		paths[file] = path
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatalf("no Go files were scanned under %s", root)
	}

	packages, imports := confinementImports(t, root, fileSet)
	for dir, files := range filesByDir {
		pkg, ok := packages[dir]
		if !ok {
			t.Fatalf("no build information for Go sources in %s", dir)
		}
		selected := map[string]bool{}
		for _, name := range pkg.GoFiles {
			selected[filepath.Join(dir, name)] = true
		}
		var active, inactive []*ast.File
		for _, file := range files {
			if selected[paths[file]] {
				active = append(active, file)
			} else {
				inactive = append(inactive, file)
			}
		}
		info := confinementTypes(t, pkg.ImportPath, fileSet, active, imports, false)
		for _, file := range active {
			for _, violation := range operatorUses(root, paths[file], fileSet, file, info) {
				t.Error(violation)
			}
		}
		// Platform alternatives are still scanned. Check each with the active
		// package's other declarations, replacing the declarations it supplies.
		// Its platform-specific syscall names may not exist on this host, but
		// those errors do not prevent checking its ownership expressions.
		for _, file := range inactive {
			companions := confinementCompanions(active, file)
			info := confinementTypes(t, pkg.ImportPath, fileSet, append(companions, file), imports, true)
			for _, violation := range operatorUses(root, paths[file], fileSet, file, info) {
				t.Error(violation)
			}
		}
	}
}

type confinementPackage struct {
	ImportPath string
	Dir        string
	Export     string
	GoFiles    []string
}

var confinementBuild struct {
	sync.Once
	packages []confinementPackage
	err      error
}

// Import compiled export data rather than guessing the fields of imported
// structs. This uses only the standard library and the module's existing build
// cache, and fails the test if the build or import data cannot be read.
func confinementImports(t *testing.T, root string, fileSet *token.FileSet) (map[string]confinementPackage, types.Importer) {
	t.Helper()
	confinementBuild.Do(func() {
		command := exec.Command("go", "list", "-deps", "-export", "-json", "./...")
		command.Dir = root
		var stderr bytes.Buffer
		command.Stderr = &stderr
		output, err := command.Output()
		if err != nil {
			confinementBuild.err = fmt.Errorf("go list: %w: %s", err, stderr.String())
			return
		}
		decoder := json.NewDecoder(bytes.NewReader(output))
		for {
			var pkg confinementPackage
			if err := decoder.Decode(&pkg); err != nil {
				if err != io.EOF {
					confinementBuild.err = err
				}
				return
			}
			confinementBuild.packages = append(confinementBuild.packages, pkg)
		}
	})
	if confinementBuild.err != nil {
		t.Fatal(confinementBuild.err)
	}
	packages := map[string]confinementPackage{}
	exports := map[string]string{}
	for _, pkg := range confinementBuild.packages {
		packages[pkg.Dir] = pkg
		exports[pkg.ImportPath] = pkg.Export
	}
	imports := importer.ForCompiler(fileSet, "gc", func(path string) (io.ReadCloser, error) {
		if exports[path] == "" {
			return nil, fmt.Errorf("no export data for %s", path)
		}
		return os.Open(exports[path])
	})
	return packages, imports
}

func confinementTypes(t *testing.T, path string, fileSet *token.FileSet, files []*ast.File, imports types.Importer, platformAlternative bool) *types.Info {
	t.Helper()
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}}
	config := types.Config{Importer: imports, Sizes: types.SizesFor("gc", runtime.GOARCH)}
	var problems []error
	config.Error = func(err error) { problems = append(problems, err) }
	_, _ = config.Check(path, fileSet, files, info)
	for _, err := range problems {
		if !platformAlternative || !confinementPlatformError(err, files) {
			t.Fatalf("type-check confinement sources in %s: %v", path, err)
		}
	}
	return info
}

// A platform alternative can name syscall constants absent on the host, and
// removing an active declaration can leave its imports unused. Tolerate those
// errors only: unresolved local types or ownership imports must fail the scan,
// rather than leave mover expressions without usable type information.
func confinementPlatformError(err error, files []*ast.File) bool {
	problem, ok := err.(types.Error)
	if !ok {
		return false
	}
	if problem.Soft && strings.Contains(problem.Msg, "imported and not used") {
		return true
	}
	if !strings.HasPrefix(problem.Msg, "undefined:") {
		return false
	}
	for _, file := range files {
		aliases := map[string]bool{}
		for _, spec := range file.Imports {
			path, _ := strconv.Unquote(spec.Path.Value)
			if path == "syscall" {
				name := "syscall"
				if spec.Name != nil {
					name = spec.Name.Name
				}
				aliases[name] = true
			}
		}
		allowed := false
		ast.Inspect(file, func(node ast.Node) bool {
			if selector, ok := node.(*ast.SelectorExpr); ok && selector.Sel.Pos() == problem.Pos {
				if pkg, ok := selector.X.(*ast.Ident); ok && aliases[pkg.Name] {
					allowed = true
				}
			}
			return true
		})
		if allowed {
			return true
		}
	}
	return false
}

// Keep package helpers when checking a platform alternative, but omit an
// active declaration with the same name as one the alternative replaces.
func confinementCompanions(active []*ast.File, alternative *ast.File) []*ast.File {
	names := map[string]bool{}
	declNames := func(decl ast.Decl) []string {
		var found []string
		switch decl := decl.(type) {
		case *ast.FuncDecl:
			if decl.Recv == nil {
				found = append(found, decl.Name.Name)
			}
		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				switch spec := spec.(type) {
				case *ast.TypeSpec:
					found = append(found, spec.Name.Name)
				case *ast.ValueSpec:
					for _, name := range spec.Names {
						found = append(found, name.Name)
					}
				}
			}
		}
		return found
	}
	for _, decl := range alternative.Decls {
		for _, name := range declNames(decl) {
			names[name] = true
		}
	}
	var companions []*ast.File
	for _, file := range active {
		copy := *file
		copy.Decls = nil
		for _, decl := range file.Decls {
			replaced := false
			for _, name := range declNames(decl) {
				replaced = replaced || names[name]
			}
			if !replaced {
				copy.Decls = append(copy.Decls, decl)
			}
		}
		companions = append(companions, &copy)
	}
	return companions
}

// operatorUses is every naming of the operator's mover in one parsed file.
func operatorUses(root, path string, fileSet *token.FileSet, file *ast.File, info *types.Info) []string {
	aliases := map[string]bool{}
	for _, spec := range file.Imports {
		imported, _ := strconv.Unquote(spec.Path.Value)
		if imported != ownershipImport {
			continue
		}
		name := "ownership"
		if spec.Name != nil {
			name = spec.Name.Name
		}
		aliases[name] = true
	}
	relative, _ := filepath.Rel(root, path)
	var found []string
	say := func(node ast.Node, what string) {
		found = append(found, relative+":"+strconv.Itoa(fileSet.Position(node.Pos()).Line)+": "+what+" names the operator's mover outside internal/ownership; ask the registry, or Mover.IsOperator")
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.SelectorExpr:
			if pkg, ok := n.X.(*ast.Ident); ok && aliases[pkg.Name] && n.Sel.Name == "Operator" {
				say(n, pkg.Name+".Operator")
				return false
			}
		case *ast.Ident:
			if n.Name == "MoverOperator" || (aliases["."] && n.Name == "Operator") {
				say(n, n.Name)
				return false
			}
		}
		if expression, ok := node.(ast.Expr); ok {
			value := info.Types[expression]
			if moverType(value.Type) && value.Value != nil && value.Value.Kind() == constant.String && constant.StringVal(value.Value) == string(Operator) {
				say(expression, "the constant \"operator\" in a mover-typed expression")
				return false
			}
		}
		return true
	})
	return found
}

func moverType(value types.Type) bool {
	if value == nil {
		return false
	}
	named, ok := types.Unalias(value).(*types.Named)
	return ok && named.Obj().Name() == "Mover" && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == ownershipImport
}

// moduleRoot is the directory holding go.mod, found by walking up.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the ownership package")
		}
		dir = parent
	}
}

func fixtureOperatorUses(t *testing.T, source string) []string {
	t.Helper()
	root := moduleRoot(t)
	fileSet := token.NewFileSet()
	path := filepath.Join(root, "surface.go")
	file, err := parser.ParseFile(fileSet, path, source, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	_, imports := confinementImports(t, root, fileSet)
	info := confinementTypes(t, "github.com/mason-bryant/yoyodyne/surface", fileSet, []*ast.File{file}, imports, false)
	return operatorUses(root, path, fileSet, file, info)
}

func TestTheConfinementScanFindsDotImportsAndTypedLiterals(t *testing.T) {
	source := `package surface
import . "` + ownershipImport + `"
var a = Operator
var b Mover = "operator"
const c Mover = "operator"
`
	if found := fixtureOperatorUses(t, source); len(found) != 3 {
		t.Errorf("the scan found %d uses, want 3: %v", len(found), found)
	}
}

func TestTheConfinementScanFindsEachWayOfNamingTheOperator(t *testing.T) {
	source := `package surface
import own "` + ownershipImport + `"
var a = own.Operator
var MoverOperator = 1
var c = own.Mover("operator")
var d = own.Harness.IsOperator()
`
	if found := fixtureOperatorUses(t, source); len(found) != 3 {
		t.Errorf("the scan found %d uses, want 3: %v", len(found), found)
	}
}

func TestTheConfinementScanFindsInferredMoverContexts(t *testing.T) {
	for _, fixture := range []struct{ name, source string }{
		{"imported composite field", `var entry = readmodel.Attention{Mover: "operator"}`},
		{"existing composite field", `func assign() { var entry readmodel.Attention; entry.Mover = "operator"; _ = entry }`},
		{"existing mover variable", `func assign() { var mover readmodel.Mover; mover = "operator"; _ = mover }`},
		{"inferred mover variable", `func assign() { mover := readmodel.MoverHarness; mover = "operator"; _ = mover }`},
		{"type alias", `type Alias = readmodel.Mover; var owner Alias = "operator"`},
		{"named local field", `type Holder struct { Next readmodel.Mover }; var entry = Holder{Next: "operator"}`},
		{"positional local field", `type Holder struct { Next readmodel.Mover }; var entry = Holder{"operator"}`},
		{"slice element", `var movers = []readmodel.Mover{"operator"}`},
		{"map value", `var movers = map[string]readmodel.Mover{"next": "operator"}`},
		{"indexed assignment", `func assign() { var movers [1]readmodel.Mover; movers[0] = "operator"; _ = movers }`},
		{"return value", `func next() readmodel.Mover { return "operator" }`},
		{"argument", `func take(readmodel.Mover) {}; func call() { take("operator") }`},
		{"constant reference", `const owner = "operator"; var entry = readmodel.Attention{Mover: owner}`},
		{"constant expression", `var entry = readmodel.Attention{Mover: "oper" + "ator"}`},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			found := fixtureOperatorUses(t, `package surface
import "github.com/mason-bryant/yoyodyne/internal/readmodel"
`+fixture.source)
			if len(found) != 1 {
				t.Errorf("the scan found %d uses, want 1: %v", len(found), found)
			}
		})
	}
}

func TestTheConfinementScanChecksPlatformAlternatives(t *testing.T) {
	root := moduleRoot(t)
	fileSet := token.NewFileSet()
	parse := func(name, source string) *ast.File {
		t.Helper()
		file, err := parser.ParseFile(fileSet, filepath.Join(root, name), source, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		return file
	}
	common := parse("common.go", `package surface
import "github.com/mason-bryant/yoyodyne/internal/readmodel"
type Mover = readmodel.Mover
type Holder struct { Next Mover }
`)
	active := parse("active.go", `package surface
func next() Mover { return "harness" }
`)
	alternative := parse("alternative.go", `package surface
import "syscall"
func next() Mover {
    _ = syscall.TargetSpecificSymbol
    entry := Holder{Next: "operator"}
    entry.Next = "operator"
    return entry.Next
}
`)
	_, imports := confinementImports(t, root, fileSet)
	files := append(confinementCompanions([]*ast.File{common, active}, alternative), alternative)
	info := confinementTypes(t, "github.com/mason-bryant/yoyodyne/surface", fileSet, files, imports, true)
	if found := operatorUses(root, filepath.Join(root, "alternative.go"), fileSet, alternative, info); len(found) != 2 {
		t.Errorf("the scan found %d uses in a platform alternative, want 2: %v", len(found), found)
	}
}

func TestTheConfinementScanRefusesUnresolvedOwnershipTypes(t *testing.T) {
	for _, source := range []string{
		`package surface; type Mover = MissingType; var owner Mover = "operator"`,
		`package surface; import own "` + ownershipImport + `"; var owner = own.MissingMover("operator")`,
	} {
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, "alternative.go", source, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		_, imports := confinementImports(t, moduleRoot(t), fileSet)
		var problems []error
		config := types.Config{Importer: imports, Error: func(err error) { problems = append(problems, err) }}
		_, _ = config.Check("github.com/mason-bryant/yoyodyne/surface", fileSet, []*ast.File{file}, nil)
		if len(problems) == 0 {
			t.Fatal("fixture did not produce its unresolved-type error")
		}
		for _, problem := range problems {
			if confinementPlatformError(problem, []*ast.File{file}) {
				t.Errorf("the scan would ignore incomplete ownership type information: %v", problem)
			}
		}
	}
}

func TestTheConfinementScanAllowsOrdinaryOperatorTextAndRegistryAnswers(t *testing.T) {
	source := `package surface
import own "` + ownershipImport + `"
import "github.com/mason-bryant/yoyodyne/internal/readmodel"
type Text struct { Mover string }
var text = Text{Mover: "operator"}
var note = "operator"
var decision = own.Resolve(own.Entry{})
var entry = readmodel.Attention{Mover: decision.Owner}
func assign() { entry.Mover = decision.Owner; text.Mover = "operator" }
var isOperator = entry.Mover.IsOperator()
`
	if found := fixtureOperatorUses(t, source); len(found) != 0 {
		t.Errorf("the scan rejected ordinary text or a registry answer: %v", found)
	}
}
