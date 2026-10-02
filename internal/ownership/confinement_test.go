package ownership

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// ownershipImport is this package's import path, which the scan below reads a
// selector's package by.
const ownershipImport = "github.com/mason-bryant/yoyodyne/internal/ownership"

// The operator's mover value is named only in this package. A test over every
// non-test Go file in the module outside it fails on any use of Operator
// through this package's import, on any identifier spelling the read model's
// old name for it, and on any conversion of the literal "operator" into a
// mover — the three ways a surface could decide an owner of its own.
func TestTheOperatorsMoverIsNamedOnlyInTheOwnershipPackage(t *testing.T) {
	root := moduleRoot(t)
	here, err := filepath.Abs(".")
	if err != nil {
		t.Fatal(err)
	}
	var violations []string
	scanned := 0
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if path != root && (strings.HasPrefix(name, ".") || name == "testdata" || name == "vendor" || name == "node_modules") {
				return filepath.SkipDir
			}
			if path == here {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		scanned++
		violations = append(violations, operatorUses(t, root, path)...)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned == 0 {
		t.Fatalf("no Go files were scanned under %s", root)
	}
	for _, violation := range violations {
		t.Error(violation)
	}
}

// operatorUses is every naming of the operator's mover in one file.
func operatorUses(t *testing.T, root, path string) []string {
	t.Helper()
	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	// The names this file imports the ownership package under.
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
			}
		case *ast.Ident:
			if n.Name == "MoverOperator" || (aliases["."] && n.Name == "Operator") {
				say(n, n.Name)
			}
		case *ast.ValueSpec:
			if namesMover(n.Type) {
				for _, value := range n.Values {
					if literal, ok := value.(*ast.BasicLit); ok && literal.Kind == token.STRING {
						if value, _ := strconv.Unquote(literal.Value); value == string(Operator) {
							say(n, "a mover initialized to \"operator\"")
						}
					}
				}
			}
		case *ast.CallExpr:
			if len(n.Args) != 1 || !namesMover(n.Fun) {
				return true
			}
			if literal, ok := n.Args[0].(*ast.BasicLit); ok && literal.Kind == token.STRING {
				if value, _ := strconv.Unquote(literal.Value); value == string(Operator) {
					say(n, "a conversion of \"operator\" to a mover")
				}
			}
		}
		return true
	})
	return found
}

func TestTheConfinementScanFindsDotImportsAndTypedLiterals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "surface.go")
	source := `package surface
import . "` + ownershipImport + `"
var a = Operator
var b Mover = "operator"
const c Mover = "operator"
`
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if found := operatorUses(t, dir, path); len(found) != 3 {
		t.Errorf("the scan found %d uses, want 3: %v", len(found), found)
	}
}

// namesMover reports a conversion's target that is the mover type: Mover, or
// any package's Mover.
func namesMover(fun ast.Expr) bool {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name == "Mover"
	case *ast.SelectorExpr:
		return f.Sel.Name == "Mover"
	}
	return false
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

// The scan finds what it is for: a file naming the operator each of the three
// ways is reported three times, and one asking IsOperator is not.
func TestTheConfinementScanFindsEachWayOfNamingTheOperator(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "surface.go")
	source := `package surface

import own "` + ownershipImport + `"

var a = own.Operator
var MoverOperator = 1
var c = own.Mover("operator")
var d = own.Harness.IsOperator()
`
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if found := operatorUses(t, dir, path); len(found) != 3 {
		t.Errorf("the scan found %d uses, want 3: %v", len(found), found)
	}
}
