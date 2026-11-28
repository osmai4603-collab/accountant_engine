package main

import (
	"errors"
	"go/token"
	"go/types"
	"os"
	"strings"
	"testing"
)

type coverageImporter struct {
	packages map[string]*types.Package
	err      error
}

func (i coverageImporter) Import(path string) (*types.Package, error) {
	if i.err != nil {
		return nil, i.err
	}
	return i.packages[path], nil
}

func coverageNamed(path, name string) (*types.Named, *types.TypeName) {
	pkg := types.NewPackage(path, "p")
	obj := types.NewTypeName(token.NoPos, pkg, name, nil)
	return types.NewNamed(obj, types.Typ[types.Int], nil), obj
}

func TestCoverageSplitNonEmpty(t *testing.T) {
	if got := splitNonEmpty(""); len(got) != 0 {
		t.Fatalf("empty input = %v", got)
	}
	if got := splitNonEmpty(" , , "); len(got) != 0 {
		t.Fatalf("blank segments = %v", got)
	}
	got := splitNonEmpty(" a , , b,c ")
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("splitNonEmpty = %v", got)
	}
}

func TestCoverageNamedObject(t *testing.T) {
	named, typeName := coverageNamed("example.com/internal/secret", "Secret")
	if namedObject(named) != typeName {
		t.Fatal("named object mismatch")
	}
	if namedObject(types.NewPointer(named)) != typeName {
		t.Fatal("pointer named object mismatch")
	}
	if namedObject(types.Typ[types.Int]) != nil {
		t.Fatal("basic type should not produce a named object")
	}

	alias := types.NewAlias(typeName, named)
	if namedObject(alias) != typeName {
		t.Fatal("alias named object mismatch")
	}
}

func TestCoverageIsPublicBridge(t *testing.T) {
	if isPublicBridge(nil) {
		t.Fatal("nil package is not a bridge")
	}
	if !isPublicBridge(types.NewPackage("example.com/project/pkg/api/v1/types", "types")) {
		t.Fatal("public bridge package not recognised")
	}
	if isPublicBridge(types.NewPackage("example.com/project/internal/kernel", "kernel")) {
		t.Fatal("internal package is not a bridge")
	}
}

func TestCoverageCheckerReport(t *testing.T) {
	_, internalObj := coverageNamed("example.com/project/internal/kernel", "KernelType")
	publicNamed, publicObj := coverageNamed("example.com/project/pkg/api/v1/types", "PublicType")
	plainNamed, _ := coverageNamed("example.com/project/domain", "DomainType")

	c := &checker{seen: map[types.Type]bool{}, bridge: map[types.Object]bool{}}
	c.report(nil, "nil")
	c.report(types.NewTypeName(token.NoPos, nil, "NoPkg", nil), "no-pkg")
	c.report(publicObj, "public")
	c.report(plainNamed.Obj(), "domain")
	if len(c.found) != 0 {
		t.Fatalf("unexpected findings: %v", c.found)
	}
	c.report(internalObj, "internal.Field")
	if len(c.found) != 1 || !strings.Contains(c.found[0], "internal.Field exposes KernelType") {
		t.Fatalf("findings = %v", c.found)
	}

	c.bridge[internalObj] = true
	c.report(internalObj, "bridged")
	if len(c.found) != 1 {
		t.Fatalf("bridged object was reported: %v", c.found)
	}
	_ = publicNamed
}

func TestCoverageCheckerCheckType(t *testing.T) {
	internalNamed, _ := coverageNamed("example.com/project/internal/kernel", "KernelType")
	publicNamed, _ := coverageNamed("example.com/project/pkg/api/v1/types", "PublicType")
	aliasObj := types.NewTypeName(token.NoPos, types.NewPackage("example.com/project/internal/alias", "alias"), "Alias", nil)
	internalAlias := types.NewAlias(aliasObj, internalNamed)
	publicObj := types.NewTypeName(token.NoPos, types.NewPackage("example.com/project/pkg/api/v1/types", "types"), "Bridge", nil)
	publicAlias := types.NewAlias(publicObj, internalNamed)

	check := func(t types.Type) *checker {
		c := &checker{seen: map[types.Type]bool{}, bridge: map[types.Object]bool{}}
		c.checkType(t, "root")
		return c
	}

	if got := check(nil); len(got.found) != 0 {
		t.Fatalf("nil type findings = %v", got.found)
	}
	namedResult := check(internalNamed)
	if len(namedResult.found) == 0 {
		t.Fatal("internal named type should be reported")
	}
	// Visited types are skipped on a second pass.
	namedResult.checkType(internalNamed, "root")

	if got := check(types.NewPointer(internalNamed)); len(got.found) == 0 {
		t.Fatal("pointer element type should be inspected")
	}
	if got := check(types.NewSlice(internalNamed)); len(got.found) == 0 {
		t.Fatal("slice element type should be inspected")
	}
	if got := check(types.NewArray(internalNamed, 2)); len(got.found) == 0 {
		t.Fatal("array element type should be inspected")
	}
	if got := check(types.NewMap(internalNamed, types.Typ[types.String])); len(got.found) == 0 {
		t.Fatal("map key type should be inspected")
	}
	if got := check(types.NewMap(types.Typ[types.String], internalNamed)); len(got.found) == 0 {
		t.Fatal("map element type should be inspected")
	}
	if got := check(types.NewChan(types.SendRecv, internalNamed)); len(got.found) == 0 {
		t.Fatal("channel element type should be inspected")
	}

	structType := types.NewStruct([]*types.Var{
		types.NewField(token.NoPos, types.NewPackage("example.com/project/domain", "domain"), "Exported", internalNamed, false),
		types.NewField(token.NoPos, types.NewPackage("example.com/project/domain", "domain"), "hidden", internalNamed, false),
	}, nil)
	if got := check(structType); len(got.found) == 0 {
		t.Fatal("exported struct field should be inspected")
	}

	sig := types.NewSignatureType(nil, nil, nil, types.NewTuple(types.NewVar(token.NoPos, nil, "in", internalNamed)),
		types.NewTuple(types.NewVar(token.NoPos, nil, "out", internalNamed)), false)
	if got := check(sig); len(got.found) == 0 {
		t.Fatal("signature parameters and results should be inspected")
	}

	constraint := types.NewInterfaceType([]*types.Func{
		types.NewFunc(token.NoPos, nil, "Method", types.NewSignatureType(nil, nil, nil, nil,
			types.NewTuple(types.NewVar(token.NoPos, nil, "", internalNamed)), false)),
	}, nil)
	constraint.Complete()
	param := types.NewTypeParam(types.NewTypeName(token.NoPos, nil, "T", nil), constraint)
	genericSig := types.NewSignatureType(nil, nil, []*types.TypeParam{param},
		types.NewTuple(types.NewVar(token.NoPos, nil, "value", param)), nil, false)
	genericResult := check(genericSig)
	if len(genericResult.found) == 0 {
		t.Fatal("generic signature should be inspected")
	}
	if got := check(types.NewTuple(types.NewVar(token.NoPos, nil, "value", internalNamed))); len(got.found) == 0 {
		t.Fatal("tuple element should be inspected")
	}

	union := types.NewUnion([]*types.Term{
		types.NewTerm(false, internalNamed),
		types.NewTerm(false, types.Typ[types.String]),
	})
	if got := check(union); len(got.found) == 0 {
		t.Fatal("union terms should be inspected")
	}

	iface := types.NewInterfaceType([]*types.Func{
		types.NewFunc(token.NoPos, nil, "Read", types.NewSignatureType(nil, nil, nil, nil,
			types.NewTuple(types.NewVar(token.NoPos, nil, "", internalNamed)), false)),
	}, nil)
	iface.Complete()
	if got := check(iface); len(got.found) == 0 {
		t.Fatal("interface method types should be inspected")
	}

	if got := check(internalAlias); len(got.found) == 0 {
		t.Fatal("internal alias should be reported")
	}
	publicAliasResult := check(publicAlias)
	if len(publicAliasResult.found) != 0 {
		t.Fatalf("public bridge alias leaked: %v", publicAliasResult.found)
	}
	if got := check(publicNamed); len(got.found) != 0 {
		t.Fatalf("public named type leaked: %v", got.found)
	}
}

func TestCoverageCollectBridgeTypes(t *testing.T) {
	if got := collectBridgeTypes(coverageImporter{}, nil); len(got) != 0 {
		t.Fatalf("empty bridge paths = %v", got)
	}
	pkg := types.NewPackage("example.com/project/pkg/api/v1/types", "types")
	internalPkg := types.NewPackage("example.com/project/internal/kernel", "kernel")
	newNamedObject := func(pkg *types.Package, name string) *types.TypeName {
		obj := types.NewTypeName(token.NoPos, pkg, name, nil)
		types.NewNamed(obj, types.Typ[types.Int], nil)
		return obj
	}
	publicObj := newNamedObject(pkg, "Public")
	hiddenObj := newNamedObject(pkg, "hidden")
	internalObj := newNamedObject(internalPkg, "Kernel")
	scope := pkg.Scope()
	scope.Insert(publicObj)
	scope.Insert(hiddenObj)
	scope.Insert(internalObj)
	imp := coverageImporter{packages: map[string]*types.Package{"example.com/project/pkg/api/v1/types": pkg}}
	allowed := collectBridgeTypes(imp, []string{"example.com/project/pkg/api/v1/types"})
	if !allowed[publicObj] || !allowed[internalObj] || allowed[hiddenObj] {
		t.Fatalf("collected bridge objects = %v", allowed)
	}
}

func TestCoverageCheckTuple(t *testing.T) {
	c := &checker{seen: map[types.Type]bool{}, bridge: map[types.Object]bool{}}
	c.checkTuple(nil, "nil")
	named, _ := coverageNamed("example.com/project/internal/kernel", "KernelType")
	c.checkTuple(types.NewTuple(types.NewVar(token.NoPos, nil, "a", named)), "tuple")
	if len(c.found) == 0 {
		t.Fatal("checkTuple did not inspect variables")
	}
}

func TestCoverageInternalAPIMain(t *testing.T) {
	original := os.Args
	defer func() { os.Args = original }()
	os.Args = []string{"internalapi", "-bridge=fmt", "fmt"}
	main()
}

func TestCoverageImporterError(t *testing.T) {
	if _, err := (coverageImporter{err: errors.New("boom")}).Import("fmt"); err == nil {
		t.Fatal("fake importer error was swallowed")
	}
}
