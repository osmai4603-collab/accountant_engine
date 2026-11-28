package kernal_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestKernelArchitectureUsesPureInterfaces(t *testing.T) {
	root := "."
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, imp := range file.Imports {
			importPath := strings.Trim(imp.Path.Value, `"`)
			switch {
			case importPath == "database/sql", importPath == "net", importPath == "net/http",
				strings.Contains(importPath, "grpc"),
				strings.Contains(importPath, "/pkg/"),
				strings.Contains(importPath, "/storage/"),
				strings.Contains(importPath, "/orbitals/"),
				strings.Contains(importPath, "/modules/"):
				t.Errorf("%s must not import host infrastructure/external package %s", path, importPath)
			}
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch v := node.(type) {
			case *ast.SelectorExpr:
				if v.Sel.Name == "Float32" || v.Sel.Name == "Float64" {
					t.Errorf("%s must not use floating-point financial identifiers", path)
				}
				if v.Sel.Name == "SetBalance" {
					t.Errorf("%s must not define or call SetBalance", path)
				}
			case *ast.Ident:
				if v.Name == "float32" || v.Name == "float64" {
					t.Errorf("%s must not use float32 or float64", path)
				}
				if v.Name == "SetBalance" {
					t.Errorf("%s must not define or use SetBalance", path)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestKernelFinancialCodeHasNoFloatConversions extends the architecture gate
// to every Go file in the kernel, including tests. Money must only be built
// from exact decimal text or integers (Phase 2, task 2).
func TestKernelFinancialCodeHasNoFloatConversions(t *testing.T) {
	root := "."
	forbiddenSelectors := map[string]bool{
		"NewFromFloat":             true,
		"NewFromFloatWithExponent": true,
		"Float32":                  true,
		"Float64":                  true,
		"BigFloat":                 true,
	}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch v := node.(type) {
			case *ast.SelectorExpr:
				if forbiddenSelectors[v.Sel.Name] {
					t.Errorf("%s uses floating-point conversion %s at %s", path, v.Sel.Name, fset.Position(v.Pos()))
				}
			case *ast.Ident:
				if v.Name == "float32" || v.Name == "float64" {
					t.Errorf("%s uses %s at %s", path, v.Name, fset.Position(v.Pos()))
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestKernelProductionCodeAvoidsLenientCompatibilityAPIs makes the Phase 2
// compatibility decision executable: deprecated lenient helpers may appear in
// kernel tests only; production code must use the checked variants.
func TestKernelProductionCodeAvoidsLenientCompatibilityAPIs(t *testing.T) {
	legacy := map[string]bool{"RoundForPosting": true, "CurrencyMinorUnit": true}
	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fn := call.Fun.(type) {
			case *ast.Ident:
				if legacy[fn.Name] {
					t.Errorf("%s calls deprecated %s at %s", path, fn.Name, fset.Position(call.Pos()))
				}
			case *ast.SelectorExpr:
				if legacy[fn.Sel.Name] {
					t.Errorf("%s calls deprecated %s at %s", path, fn.Sel.Name, fset.Position(call.Pos()))
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestKernelInvariantDocumentIsPresentAndScoped(t *testing.T) {
	data, err := os.ReadFile("KERNEL_ACCOUNTING_INVARIANTS.md")
	if err != nil {
		t.Fatalf("read kernel invariant document: %v", err)
	}
	text := string(data)
	for _, required := range []string{
		"RULE-KERNEL-001",
		"RULE-KERNEL-009",
		"RULE-KERNEL-016",
		"RULE-KERNEL-021",
		"RULE-KERNEL-025",
		"RULE-KERNEL-027",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("kernel invariant document missing %s", required)
		}
	}
	if !strings.Contains(text, "outside this kernel") {
		t.Fatal("kernel document must state that RBAC and maker-checker approvals are outside the kernel")
	}
}

// TestKernelTraceabilityMatrixCoversAllRulesAndNamedTests makes the Phase 6
// traceability gate executable. Every one of the 27 rules must appear in both
// the invariant document and the matrix; every matrix rule row must name a
// responsible function and a concrete test; and every backticked Test/Fuzz
// reference in the matrix must resolve to a real kernel test function.
// Semantic assertion quality remains a manual review responsibility (G-012).
func TestKernelTraceabilityMatrixCoversAllRulesAndNamedTests(t *testing.T) {
	invariants, err := os.ReadFile("KERNEL_ACCOUNTING_INVARIANTS.md")
	if err != nil {
		t.Fatalf("read kernel invariant document: %v", err)
	}
	matrix, err := os.ReadFile("KERNEL_TRACEABILITY_MATRIX.md")
	if err != nil {
		t.Fatalf("read kernel traceability matrix: %v", err)
	}
	invariantText, matrixText := string(invariants), string(matrix)

	for i := 1; i <= 27; i++ {
		id := fmt.Sprintf("RULE-KERNEL-%03d", i)
		if !strings.Contains(invariantText, id) {
			t.Errorf("kernel invariant document missing %s", id)
		}
		if !strings.Contains(matrixText, id) {
			t.Errorf("kernel traceability matrix missing %s", id)
		}
	}

	testRef := regexp.MustCompile("`(Test[A-Za-z0-9_]+|Fuzz[A-Za-z0-9_]+)`")
	for row := range strings.SplitSeq(matrixText, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(row), "| `RULE-KERNEL-") {
			continue
		}
		cells := strings.Split(row, "|")
		if len(cells) < 10 {
			t.Errorf("malformed rule row %q", row)
			continue
		}
		rule := strings.TrimSpace(cells[1])
		if strings.TrimSpace(cells[4]) == "" {
			t.Errorf("%s has no responsible function", rule)
		}
		if !testRef.MatchString(cells[5]) || !testRef.MatchString(cells[6]) {
			t.Errorf("%s must name both an acceptance and a rejection test", rule)
		}
	}

	defined := map[string]bool{}
	err = filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if strings.HasPrefix(fn.Name.Name, "Test") || strings.HasPrefix(fn.Name.Name, "Fuzz") {
				defined[fn.Name.Name] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	missing := map[string]bool{}
	references := 0
	for _, match := range testRef.FindAllStringSubmatch(matrixText, -1) {
		references++
		if !defined[match[1]] {
			missing[match[1]] = true
		}
	}
	if references == 0 {
		t.Fatal("kernel traceability matrix has no backticked Test/Fuzz references")
	}
	if len(missing) > 0 {
		names := make([]string, 0, len(missing))
		for name := range missing {
			names = append(names, name)
		}
		sort.Strings(names)
		t.Errorf("traceability matrix references undefined kernel tests: %s", strings.Join(names, ", "))
	}
}
