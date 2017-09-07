// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.
package permission

import (
	"go/ast"
	goparser "go/parser"
	"go/token"
	"go/types"
	"reflect"
	"testing"
)

// newInterfaceWithMethod creates a recursive interface permission
func newInterfaceWithMethod() *InterfacePermission {
	iface := &InterfacePermission{
		BasePermission: Mutable,
		Methods: []Permission{
			&FuncPermission{
				Name:           "foo",
				BasePermission: Mutable,
				Params:         []Permission{Mutable},
			},
		},
	}
	iface.Methods[0].(*FuncPermission).Receivers = []Permission{iface}
	iface.Methods[0].(*FuncPermission).Results = []Permission{iface}
	return iface
}

// testCases contains tests for the permission parser.
var testCases = map[string]Permission{
	"struct { x int64; y interface {}}": &StructPermission{
		BasePermission: Mutable,
		Fields: []Permission{Mutable, &InterfacePermission{
			BasePermission: Mutable,
		}},
	},
	"[]interface{}": &SlicePermission{
		BasePermission: Mutable,
		ElementPermission: &InterfacePermission{
			BasePermission: Mutable,
		},
	},
	"[5]interface{}": &ArrayPermission{
		BasePermission: Mutable,
		ElementPermission: &InterfacePermission{
			BasePermission: Mutable,
		},
	},
	"chan interface{}": &ChanPermission{
		BasePermission: Mutable,
		ElementPermission: &InterfacePermission{
			BasePermission: Mutable,
		},
	},
	"*interface{}": &PointerPermission{
		BasePermission: Mutable,
		Target: &InterfacePermission{
			BasePermission: Mutable,
		},
	},
	"map[interface{}] int": &MapPermission{
		BasePermission: Mutable,
		KeyPermission: &InterfacePermission{
			BasePermission: Mutable,
		},
		ValuePermission: Mutable,
	},
	"func()": &FuncPermission{
		BasePermission: Mutable,
	},
	"func(b interface{})": &FuncPermission{
		BasePermission: Mutable,
		Params: []Permission{
			&InterfacePermission{
				BasePermission: Mutable,
			},
		},
	},
	"func(b interface{})(c int)": &FuncPermission{
		BasePermission: Mutable,
		Params: []Permission{
			&InterfacePermission{
				BasePermission: Mutable,
			},
		},
		Results: []Permission{Mutable},
	},
	"interface { foo(int) t}": newInterfaceWithMethod(),
}

func Parsed(s string) (types.Type, error) {
	config := types.Config{}
	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, s+".go", "package test\ntype t "+s, goparser.ParseComments)
	if err != nil {
		return nil, err
	}
	info := types.Info{Defs: make(map[*ast.Ident]types.Object)}
	_, err = config.Check("hello", fset, []*ast.File{f}, &info)
	if err != nil {
		return nil, err
	}

	for ident, typ := range info.Defs {
		if ident.Name == "t" {
			return typ.Type(), nil
		}
	}
	return nil, nil
}

func TestNewFromType(t *testing.T) {
	for input, expected := range testCases {
		input := input
		expected := expected
		t.Run(input, func(t *testing.T) {
			typ, err := Parsed(input)
			if err != nil {
				t.Errorf("Invalid test input: %s", err)
				return
			}
			if typ == nil {
				t.Errorf("Does not parse to a type")
				return
			}
			perm := NewTypeMapper().NewFromType(typ)
			if !reflect.DeepEqual(perm, expected) {
				t.Errorf("Input %s: Unexpected permission %#v, expected %#v - error: %v", input, perm, expected, err)
			}
		})
	}
}

func TestNewFromType_invalid(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Expected panic")
		}
	}()

	var typ *types.Tuple
	NewTypeMapper().NewFromType(typ)
}
