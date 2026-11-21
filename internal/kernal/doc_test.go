package kernal_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
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
