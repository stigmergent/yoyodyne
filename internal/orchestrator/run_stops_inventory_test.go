package orchestrator

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const runStopsInventoryPath = "docs/run-stops.md"

type runStopSite struct {
	file, declaration, signal string
}

// Count sites, not just declarations: another ending inside a function already
// in the inventory must still fail. The document supplies the answers; this
// sweep only identifies the syntax the run's endings use.
func TestRunEndingSitesAreInventoried(t *testing.T) {
	t.Parallel()
	problems, err := checkRunStopsInventory("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range problems {
		t.Error(problem)
	}
}

func checkRunStopsInventory(root string) ([]string, error) {
	content, err := os.ReadFile(filepath.Join(root, runStopsInventoryPath))
	if err != nil {
		return nil, err
	}
	listed := map[runStopSite]int{}
	var problems []string
	for _, heading := range []string{"## Run-ending sites", "## Sites that do not end a run"} {
		rows, err := runStopsRows(string(content), heading, 5)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			site := runStopSite{row[0], row[1], row[2]}
			count, err := strconv.Atoi(row[3])
			if err != nil || count < 1 || row[4] == "" {
				return nil, fmt.Errorf("%s: %v needs a positive count and an explanation", runStopsInventoryPath, site)
			}
			if _, duplicate := listed[site]; duplicate {
				problems = append(problems, fmt.Sprintf("%s: %v is listed twice", runStopsInventoryPath, site))
			}
			listed[site] = count
		}
	}
	if len(listed) == 0 {
		return nil, fmt.Errorf("%s lists no run-ending sites", runStopsInventoryPath)
	}
	found := map[runStopSite]int{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".dolt", "testdata", "dist", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		for site, count := range runEndingSitesIn(parsed, filepath.ToSlash(relative)) {
			found[site] += count
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, fmt.Errorf("the run-ending sweep found nothing in %s", root)
	}
	for site, count := range found {
		if listed[site] != count {
			problems = append(problems, fmt.Sprintf("%s: %s has %d %s site(s), %s lists %d; account for the ending or explain why it is not one",
				site.file, site.declaration, count, site.signal, runStopsInventoryPath, listed[site]))
		}
	}
	for site := range listed {
		if found[site] == 0 {
			problems = append(problems, fmt.Sprintf("%s: %s %s %s is no longer found; correct or remove the row",
				runStopsInventoryPath, site.file, site.declaration, site.signal))
		}
	}
	sort.Strings(problems)
	return problems, nil
}

func runEndingSitesIn(parsed *ast.File, file string) map[runStopSite]int {
	found := map[runStopSite]int{}
	add := func(declaration, signal string) { found[runStopSite{file, declaration, signal}]++ }
	stateNames := map[string]bool{}
	for _, imported := range parsed.Imports {
		path, _ := strconv.Unquote(imported.Path.Value)
		if !strings.HasSuffix(path, "/internal/runstate") {
			continue
		}
		name := "runstate"
		if imported.Name != nil {
			name = imported.Name.Name
		}
		stateNames[name] = true
	}
	terminal := func(expression ast.Expr) bool {
		name := ""
		switch value := expression.(type) {
		case *ast.SelectorExpr:
			if qualifier, ok := value.X.(*ast.Ident); ok && stateNames[qualifier.Name] {
				name = value.Sel.Name
			}
		case *ast.Ident:
			if parsed.Name.Name == "runstate" {
				name = value.Name
			}
		}
		return strings.HasPrefix(name, "Status") && name != "StatusRunning" && name != "StatusPending" && name != "Status"
	}
	for _, declaration := range parsed.Decls {
		switch declared := declaration.(type) {
		case *ast.FuncDecl:
			if declared.Body == nil {
				continue
			}
			name := declared.Name.Name
			if declared.Recv != nil {
				name = "(" + runStopsExpression(declared.Recv.List[0].Type) + ")." + name
			}
			ast.Inspect(declared.Body, func(node ast.Node) bool {
				switch value := node.(type) {
				case *ast.CallExpr:
					if parsed.Name.Name != "orchestrator" {
						break
					}
					switch called := value.Fun.(type) {
					case *ast.SelectorExpr:
						switch called.Sel.Name {
						case "fail", "stop", "finish", "complete", "escalate":
							add(name, "call:"+called.Sel.Name)
						}
					case *ast.Ident:
						if called.Name == "stoppedBy" {
							add(name, "classified-stop")
						}
					}
				case *ast.CompositeLit:
					if parsed.Name.Name == "orchestrator" && runStopsExpression(value.Type) == "phaseError" {
						add(name, "phase-error")
					}
					for _, element := range value.Elts {
						if field, ok := element.(*ast.KeyValueExpr); ok && runStopsExpression(field.Key) == "Status" && terminal(field.Value) {
							add(name, "terminal-literal")
						}
					}
				case *ast.AssignStmt:
					for index, target := range value.Lhs {
						if field, ok := target.(*ast.SelectorExpr); ok && field.Sel.Name == "Status" {
							if parsed.Name.Name == "orchestrator" || (index < len(value.Rhs) && terminal(value.Rhs[index])) {
								add(name, "status-write")
							}
						}
					}
				}
				return true
			})
		case *ast.GenDecl:
			if declared.Tok != token.CONST || parsed.Name.Name != "runstate" {
				continue
			}
			for _, specification := range declared.Specs {
				value, ok := specification.(*ast.ValueSpec)
				if !ok {
					continue
				}
				switch runStopsExpression(value.Type) {
				case "EnvironmentalCause", "StopClass":
					for _, name := range value.Names {
						add(name.Name, runStopsExpression(value.Type))
					}
				}
			}
		}
	}
	return found
}

func runStopsExpression(node ast.Node) string {
	if node == nil {
		return ""
	}
	var rendered bytes.Buffer
	_ = format.Node(&rendered, token.NewFileSet(), node)
	return rendered.String()
}

func runStopsRows(content, heading string, columns int) ([][]string, error) {
	var rows [][]string
	inSection := false
	header := false
	for line, text := range strings.Split(content, "\n") {
		text = strings.TrimSpace(text)
		if strings.HasPrefix(text, "#") {
			inSection = text == heading
			continue
		}
		if !inSection || !strings.HasPrefix(text, "|") {
			continue
		}
		if !header {
			header = true
			continue
		}
		if strings.Trim(text, "| -:") == "" {
			continue
		}
		cells := strings.Split(strings.Trim(text, "|"), "|")
		if len(cells) != columns {
			return nil, fmt.Errorf("%s:%d: want %d cells under %s, got %d", runStopsInventoryPath, line+1, columns, heading, len(cells))
		}
		for i := range cells {
			cells[i] = strings.Trim(strings.TrimSpace(cells[i]), "`")
		}
		rows = append(rows, cells)
	}
	return rows, nil
}

func TestRunEndingSweepFindsNewSitesWithinExistingFunctions(t *testing.T) {
	t.Parallel()
	source := `package orchestrator
import state "example/internal/runstate"
func (a *activeRun) develop() {
 a.fail(first, state.StatusFailed)
 a.fail(second, state.StatusTimedOut)
 a.stop(ctx, stoppedBy(state.StopProvider, phaseError{}))
 a.state.Status = state.StatusCancelled
}

`
	parsed, err := parser.ParseFile(token.NewFileSet(), "pipeline.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := runEndingSitesIn(parsed, "pipeline.go")
	for signal, count := range map[string]int{"call:fail": 2, "call:stop": 1, "classified-stop": 1, "phase-error": 1, "status-write": 1} {
		if got := found[runStopSite{"pipeline.go", "(*activeRun).develop", signal}]; got != count {
			t.Errorf("%s = %d, want %d", signal, got, count)
		}
	}
}

func TestRunEndingSweepFindsVocabularyAndEndingsOutsideThePipeline(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		source, declaration, signal string
	}{
		{`package runstate; const CauseNew EnvironmentalCause = "new"`, "CauseNew", "EnvironmentalCause"},
		{`package runstate; const StopNew StopClass = "new"`, "StopNew", "StopClass"},
		{`package elsewhere; import state "example/internal/runstate"; func end() { s.Status = state.StatusCancelled }`, "end", "status-write"},
		{`package elsewhere; import state "example/internal/runstate"; func end() { s := state.State{Status: state.StatusFailed} }`, "end", "terminal-literal"},
	} {
		parsed, err := parser.ParseFile(token.NewFileSet(), "ending.go", test.source, 0)
		if err != nil {
			t.Fatal(err)
		}
		found := runEndingSitesIn(parsed, "ending.go")
		if found[runStopSite{"ending.go", test.declaration, test.signal}] != 1 {
			t.Errorf("%s %s was not found: %v", test.declaration, test.signal, found)
		}
	}
}

func TestRunStopsInventoryRejectsDrift(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, source, rows, want string
	}{
		{"listed", `func end() { a.fail(err, status) }`, "| pipeline.go | end | call:fail | 1 | Ends the run. |", ""},
		{"new function", `func end() { a.fail(err, status) }; func other() { a.stop(ctx, err) }`, "| pipeline.go | end | call:fail | 1 | Ends the run. |", "other has 1 call:stop"},
		{"new branch", `func end() { a.fail(first, status); a.fail(second, status) }`, "| pipeline.go | end | call:fail | 1 | Ends the run. |", "end has 2 call:fail"},
		{"renamed", `func other() { a.fail(err, status) }`, "| pipeline.go | end | call:fail | 1 | Ends the run. |", "end call:fail is no longer found"},
		{"duplicate", `func end() { a.fail(err, status) }`, "| pipeline.go | end | call:fail | 1 | Ends the run. |\n| pipeline.go | end | call:fail | 1 | Ends it again. |", "is listed twice"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "docs"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "pipeline.go"), []byte("package orchestrator\n"+test.source), 0600); err != nil {
				t.Fatal(err)
			}
			document := "## Run-ending sites\n\n| File | Declaration | Signal | Count | Meaning |\n| --- | --- | --- | --- | --- |\n" + test.rows + "\n"
			if err := os.WriteFile(filepath.Join(root, runStopsInventoryPath), []byte(document), 0600); err != nil {
				t.Fatal(err)
			}
			problems, err := checkRunStopsInventory(root)
			if err != nil {
				t.Fatal(err)
			}
			if test.want == "" && len(problems) != 0 {
				t.Fatalf("listed ending failed: %v", problems)
			}
			if test.want != "" && !strings.Contains(strings.Join(problems, "\n"), test.want) {
				t.Fatalf("problems = %v, want %q", problems, test.want)
			}
		})
	}
}

// These are source expressions, so even a changed hard-coded bound cannot
// silently leave the default printed in the operator's inventory behind.
func TestRunStopBoundValuesMatchInventory(t *testing.T) {
	t.Parallel()
	root := "../.."
	content, err := os.ReadFile(filepath.Join(root, runStopsInventoryPath))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := runStopsRows(string(content), "## Bound source values", 3)
	if err != nil || len(rows) == 0 {
		t.Fatalf("bound source values: %v; %d rows", err, len(rows))
	}
	files := map[string]map[string]string{}
	seen := map[string]bool{}
	for _, row := range rows {
		file, declaration, want := row[0], row[1], row[2]
		key := file + " " + declaration
		if seen[key] {
			t.Errorf("%s: %s is listed twice", runStopsInventoryPath, key)
		}
		seen[key] = true
		if files[file] == nil {
			parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(root, file), nil, 0)
			if err != nil {
				t.Error(err)
				continue
			}
			files[file] = runStopsSourceValues(parsed)
		}
		if got := files[file][declaration]; got != want {
			t.Errorf("%s: %s = %q, inventory says %q; update the bound and its account together", file, declaration, got, want)
		}
	}
}

func runStopsSourceValues(parsed *ast.File) map[string]string {
	values := map[string]string{}
	for _, declaration := range parsed.Decls {
		switch declared := declaration.(type) {
		case *ast.GenDecl:
			for _, specification := range declared.Specs {
				if value, ok := specification.(*ast.ValueSpec); ok {
					for index, name := range value.Names {
						if index < len(value.Values) {
							values[name.Name] = runStopsExpression(value.Values[index])
						}
					}
				}
			}
		case *ast.FuncDecl:
			if declared.Body == nil {
				continue
			}
			ast.Inspect(declared.Body, func(node ast.Node) bool {
				if literal, ok := node.(*ast.CompositeLit); ok {
					for _, element := range literal.Elts {
						if field, ok := element.(*ast.KeyValueExpr); ok {
							name := declared.Name.Name + ":" + runStopsExpression(literal.Type) + "." + runStopsExpression(field.Key)
							values[name] = runStopsExpression(field.Value)
						}
					}
				}
				return true
			})
		}
	}
	return values
}
