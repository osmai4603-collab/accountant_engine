// Command internalapi reports exported identifiers whose public Go type
// mentions an internal package. It is used by scripts/verify_architecture.sh.
//
// The pkg/api/v1 re-export bridge is the single sanctioned exception: names
// defined there are public contracts even when implemented as aliases to
// internal/kernel types (see ADR P7-01).
//
// Usage: go run ./scripts/internalapi [-bridge p1,p2,...] <import-path>...
//
// The -bridge flag lists the pkg/api/v1/... packages whose exported names are
// the sanctioned re-exports. Their declared types are allowed to mention
// internal/kernel even though the source importer resolves aliases eagerly.
package main

import (
	"fmt"
	"go/importer"
	"go/token"
	"go/types"
	"os"
	"sort"
	"strings"
)

func main() {
	args := os.Args[1:]
	var bridgePaths []string
	if len(args) > 0 && strings.HasPrefix(args[0], "-bridge=") {
		bridgePaths = splitNonEmpty(strings.TrimPrefix(args[0], "-bridge="))
		args = args[1:]
	}
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: internalapi [-bridge p1,p2,...] <import-path>...")
		os.Exit(2)
	}
	fset := token.NewFileSet()
	imp := importer.ForCompiler(fset, "source", nil)
	bridge := collectBridgeTypes(imp, bridgePaths)
	violations := 0
	for _, path := range args {
		pkg, err := imp.Import(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "internalapi: load %s: %v\n", path, err)
			os.Exit(2)
		}
		scope := pkg.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			if !obj.Exported() {
				continue
			}
			var found []string
			checker := &checker{seen: map[types.Type]bool{}, bridge: bridge}
			checker.checkType(obj.Type(), name)
			if named, ok := obj.Type().(*types.Named); ok {
				for i := 0; i < named.NumMethods(); i++ {
					method := named.Method(i)
					if !method.Exported() {
						continue
					}
					checker.checkType(method.Type(), name+"."+method.Name())
				}
			}
			found = append(found, checker.found...)
			sort.Strings(found)
			for _, item := range found {
				fmt.Printf("%s: %s\n", path, item)
				violations++
			}
		}
	}
	if violations > 0 {
		fmt.Fprintf(os.Stderr, "internalapi: %d exported internal type exposure(s)\n", violations)
		os.Exit(1)
	}
}

func splitNonEmpty(value string) []string {
	var result []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}

// collectBridgeTypes records the underlying named objects declared by the
// sanctioned bridge packages. Aliases in those packages resolve to the same
// objects, so the set can be used to distinguish a permitted re-export from a
// raw internal type leaking through a public signature.
func collectBridgeTypes(imp types.Importer, paths []string) map[types.Object]bool {
	allowed := map[types.Object]bool{}
	for _, path := range paths {
		pkg, err := imp.Import(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "internalapi: load bridge %s: %v\n", path, err)
			os.Exit(2)
		}
		for _, name := range pkg.Scope().Names() {
			obj := pkg.Scope().Lookup(name)
			if !obj.Exported() {
				continue
			}
			if named := namedObject(obj.Type()); named != nil {
				allowed[named] = true
			}
		}
	}
	return allowed
}

func namedObject(t types.Type) types.Object {
	switch t := t.(type) {
	case *types.Alias:
		return namedObject(types.Unalias(t))
	case *types.Named:
		return t.Obj()
	case *types.Pointer:
		return namedObject(t.Elem())
	}
	return nil
}

type checker struct {
	seen   map[types.Type]bool
	bridge map[types.Object]bool
	found  []string
}

func (c *checker) checkType(t types.Type, where string) {
	if t == nil || c.seen[t] {
		return
	}
	c.seen[t] = true
	switch t := t.(type) {
	case *types.Alias:
		c.report(t.Obj(), where)
		if isPublicBridge(t.Obj().Pkg()) {
			break
		}
		c.checkType(types.Unalias(t), where)
	case *types.Named:
		c.report(t.Obj(), where)
		if isPublicBridge(t.Obj().Pkg()) {
			break
		}
		c.checkType(t.Underlying(), where)
	case *types.Pointer:
		c.checkType(t.Elem(), where)
	case *types.Slice:
		c.checkType(t.Elem(), where)
	case *types.Array:
		c.checkType(t.Elem(), where)
	case *types.Map:
		c.checkType(t.Key(), where)
		c.checkType(t.Elem(), where)
	case *types.Chan:
		c.checkType(t.Elem(), where)
	case *types.Struct:
		for i := 0; i < t.NumFields(); i++ {
			field := t.Field(i)
			if !field.Exported() {
				continue
			}
			c.checkType(field.Type(), where+"."+field.Name())
		}
	case *types.Interface:
		for i := 0; i < t.NumMethods(); i++ {
			c.checkType(t.Method(i).Type(), where+"."+t.Method(i).Name())
		}
	case *types.Signature:
		if t.TypeParams() != nil {
			for i := 0; i < t.TypeParams().Len(); i++ {
				c.checkType(t.TypeParams().At(i), where)
			}
		}
		c.checkTuple(t.Params(), where)
		c.checkTuple(t.Results(), where)
	case *types.TypeParam:
		c.checkType(t.Constraint(), where)
	case *types.Tuple:
		c.checkTuple(t, where)
	case *types.Union:
		for i := 0; i < t.Len(); i++ {
			c.checkType(t.Term(i).Type(), where)
		}
	}
}

func (c *checker) checkTuple(tuple *types.Tuple, where string) {
	if tuple == nil {
		return
	}
	for i := 0; i < tuple.Len(); i++ {
		c.checkType(tuple.At(i).Type(), where)
	}
}

func isPublicBridge(pkg *types.Package) bool {
	return pkg != nil && strings.Contains(pkg.Path(), "/pkg/api/v1")
}

func (c *checker) report(obj types.Object, where string) {
	if obj == nil || obj.Pkg() == nil {
		return
	}
	if c.bridge[obj] || isPublicBridge(obj.Pkg()) {
		return
	}
	path := obj.Pkg().Path()
	if strings.Contains(path, "/internal/") || strings.HasSuffix(path, "/internal") {
		c.found = append(c.found, fmt.Sprintf("%s exposes %s (%s)", where, obj.Name(), path))
	}
}
