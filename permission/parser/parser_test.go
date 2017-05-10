// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.
package parser

import (
	"reflect"
	"sort"
	"testing"

	"github.com/julian-klode/lingolang/permission"
)

// testCases contains tests for the permission parser.
var testCases = map[string]permission.Permission{
	"123":     nil,
	"!":       nil,
	"\xc2":    nil, // incomplete rune at beginning
	"a\xc2":   nil, // incomplete rune in word
	"a !":     nil,
	"a error": nil,
	"":        nil,
	"oe":      nil,
	"or":      permission.Owned | permission.Read,
	"ow":      permission.Owned | permission.Write,
	"orwR":    permission.Owned | permission.Read | permission.Write | permission.ExclRead,
	"orR":     permission.Owned | permission.Read | permission.ExclRead,
	"owW":     permission.Owned | permission.Write | permission.ExclWrite,
	"om":      permission.Owned | permission.Mutable,
	"ov":      permission.Owned | permission.Value,
	"a":       permission.Any,
	"on":      permission.Owned,
	"n":       permission.None,
	"m [":     nil,
	"m [1":    nil,
	"m []":    nil,
	"m [1]":   nil,
	"m [] a": &permission.ArraySlicePermission{
		BasePermission:    permission.Mutable,
		ElementPermission: permission.Any,
	},
	"m [1] a": &permission.ArraySlicePermission{
		BasePermission:    permission.Mutable,
		ElementPermission: permission.Any,
	},
	"m map[v]l": &permission.MapPermission{
		BasePermission:  permission.Mutable,
		KeyPermission:   permission.Value,
		ValuePermission: permission.LinearValue,
	},
	"n map":         nil,
	"n map [":       nil,
	"n map [error]": nil,
	"n map [n":      nil,
	"n map [n]":     nil,
	"m chan l": &permission.ChanPermission{
		BasePermission:    permission.Mutable,
		ElementPermission: permission.LinearValue,
	},
	"m chan":       nil,
	"m chan error": nil,
	"m * l": &permission.PointerPermission{
		BasePermission: permission.Mutable,
		Target:         permission.LinearValue,
	},
	"error":     nil,
	"m * error": nil,
	"m func (v) a": &permission.FuncPermission{
		BasePermission: permission.Mutable,
		Receivers:      nil,
		Params:         []permission.Permission{permission.Value},
		Results:        []permission.Permission{permission.Any},
	},
	"m (m) func (v) a": &permission.FuncPermission{
		BasePermission: permission.Mutable,
		Receivers:      []permission.Permission{permission.Mutable},
		Params:         []permission.Permission{permission.Value},
		Results:        []permission.Permission{permission.Any},
	},
	"m (m) func (v, l) a": &permission.FuncPermission{
		BasePermission: permission.Mutable,
		Receivers:      []permission.Permission{permission.Mutable},
		Params:         []permission.Permission{permission.Value, permission.LinearValue},
		Results:        []permission.Permission{permission.Any},
	},
	"m (m) func (v, l) (a)": &permission.FuncPermission{
		BasePermission: permission.Mutable,
		Receivers:      []permission.Permission{permission.Mutable},
		Params:         []permission.Permission{permission.Value, permission.LinearValue},
		Results:        []permission.Permission{permission.Any},
	},
	"m (m) func (v, l) (a, n)": &permission.FuncPermission{
		BasePermission: permission.Mutable,
		Receivers:      []permission.Permission{permission.Mutable},
		Params:         []permission.Permission{permission.Value, permission.LinearValue},
		Results:        []permission.Permission{permission.Any, permission.None},
	},
	"m (m) func (v, l)": &permission.FuncPermission{
		BasePermission: permission.Mutable,
		Receivers:      []permission.Permission{permission.Mutable},
		Params:         []permission.Permission{permission.Value, permission.LinearValue},
		Results:        nil,
	},
	"m (m) func ()": &permission.FuncPermission{
		BasePermission: permission.Mutable,
		Receivers:      []permission.Permission{permission.Mutable},
		Params:         nil,
		Results:        nil,
	},
	"m () func (v, l)":       nil,
	"m (m":                   nil,
	"m (m)":                  nil,
	"m (m) func":             nil,
	"m (m) func (":           nil,
	"m (m) func (v":          nil,
	"m (m) func (v,)":        nil,
	"m (m) func (v) error":   nil,
	"m (m) func (v) (error)": nil,
	"m (m) func (v) (v,)":    nil,
	"m (m) func (v) (v !)":   nil,
	"m (m) func (v) (v":      nil,
	"m (m) func (v) hello":   nil,
	// Interface
	"m interface {}": &permission.InterfacePermission{
		BasePermission: permission.Mutable,
	},
	"l interface {}": &permission.InterfacePermission{
		BasePermission: permission.LinearValue,
	},
	"l interface {r; w}": &permission.InterfacePermission{
		BasePermission: permission.LinearValue,
		Methods: []permission.Permission{
			permission.Read,
			permission.Write,
		},
	},
	"m interface {":   nil,
	"m interface {a":  nil,
	"m interface }":   nil,
	"error interface": nil,
	"interface error": nil,
	"{}":              nil,
	"m struct {}":     nil,
	"m struct":        nil,
	"m struct {":      nil,
	"m struct }":      nil,
	"m struct v":      nil,
	"m struct {v}": &permission.StructPermission{
		BasePermission: permission.Mutable,
		Fields: []permission.Permission{
			permission.Value,
		},
	},
	"m struct {v; l}": &permission.StructPermission{
		BasePermission: permission.Mutable,
		Fields: []permission.Permission{
			permission.Value,
			permission.LinearValue,
		},
	},
}

func helper() (perm permission.Permission, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = r.(error)
		}
	}()
	perm = NewParser("error").parseBasePermission()
	return perm, nil
}

func TestParser(t *testing.T) {
	for input, expected := range testCases {
		input := input
		expected := expected
		t.Run(input, func(t *testing.T) {
			perm, err := NewParser(input).Parse()
			if !reflect.DeepEqual(perm, expected) {
				t.Errorf("Input %s: Unexpected permission %v, expected %v - error: %v", input, perm, expected, err)
			}
		})
	}

	perm, err := helper()
	if err == nil {
		t.Errorf("Input 'error' parsed to valid base permission %v", perm)
	}
}

func BenchmarkParser(b *testing.B) {
	keys := make([]string, 0, len(testCases))
	for input := range testCases {
		keys = append(keys, input)
	}
	sort.Strings(keys)

	for _, input := range keys {
		expected := testCases[input]
		if expected == nil {
			continue
		}
		input := input
		b.Run(input, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				NewParser(input).Parse()
			}
		})
	}
}
