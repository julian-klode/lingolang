// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.
package permission

import (
	"reflect"
	"sort"
	"testing"
)

// testCasesParser contains tests for the permission parser.
var testCasesParser = map[string]Permission{
	"123":     nil,
	"!":       nil,
	"\xc2":    nil, // incomplete rune at beginning
	"a\xc2":   nil, // incomplete rune in word
	"a !":     nil,
	"a error": nil,
	"":        nil,
	"oe":      nil,
	"or":      Owned | Read,
	"ow":      Owned | Write,
	"orwR":    Owned | Read | Write | ExclRead,
	"orR":     Owned | Read | ExclRead,
	"owW":     Owned | Write | ExclWrite,
	"om":      Owned | Mutable,
	"ov":      Owned | Value,
	"a":       Any,
	"on":      Owned,
	"n":       None,
	"m [":     nil,
	"m [1":    nil,
	"m []":    nil,
	"m [1]":   nil,
	"m [] a": &SlicePermission{
		BasePermission:    Mutable,
		ElementPermission: Any,
	},
	"m [1] a": &ArrayPermission{
		BasePermission:    Mutable,
		ElementPermission: Any,
	},
	"m [_] a": &ArrayPermission{
		BasePermission:    Mutable,
		ElementPermission: Any,
	},
	"m map[v]l": &MapPermission{
		BasePermission:  Mutable,
		KeyPermission:   Value,
		ValuePermission: LinearValue,
	},
	"n map":         nil,
	"n map [":       nil,
	"n map [error]": nil,
	"n map [n":      nil,
	"n map [n]":     nil,
	"m chan l": &ChanPermission{
		BasePermission:    Mutable,
		ElementPermission: LinearValue,
	},
	"m chan":       nil,
	"m chan error": nil,
	"m * l": &PointerPermission{
		BasePermission: Mutable,
		Target:         LinearValue,
	},
	"error":     nil,
	"m * error": nil,
	"m func (v) a": &FuncPermission{
		BasePermission: Mutable,
		Receivers:      nil,
		Params:         []Permission{Value},
		Results:        []Permission{Any},
	},
	"m (m) func (v) a": &FuncPermission{
		BasePermission: Mutable,
		Receivers:      []Permission{Mutable},
		Params:         []Permission{Value},
		Results:        []Permission{Any},
	},
	"m (m) func (v, l) a": &FuncPermission{
		BasePermission: Mutable,
		Receivers:      []Permission{Mutable},
		Params:         []Permission{Value, LinearValue},
		Results:        []Permission{Any},
	},
	"m (m) func (v, l) (a)": &FuncPermission{
		BasePermission: Mutable,
		Receivers:      []Permission{Mutable},
		Params:         []Permission{Value, LinearValue},
		Results:        []Permission{Any},
	},
	"m (m) func (v, l) (a, n)": &FuncPermission{
		BasePermission: Mutable,
		Receivers:      []Permission{Mutable},
		Params:         []Permission{Value, LinearValue},
		Results:        []Permission{Any, None},
	},
	"m (m) func (v, l)": &FuncPermission{
		BasePermission: Mutable,
		Receivers:      []Permission{Mutable},
		Params:         []Permission{Value, LinearValue},
		Results:        nil,
	},
	"m (m) func ()": &FuncPermission{
		BasePermission: Mutable,
		Receivers:      []Permission{Mutable},
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
	"m interface {}": &InterfacePermission{
		BasePermission: Mutable,
	},
	"l interface {}": &InterfacePermission{
		BasePermission: LinearValue,
	},
	"l interface {r; w}": &InterfacePermission{
		BasePermission: LinearValue,
		Methods: []Permission{
			Read,
			Write,
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
	"m struct {v}": &StructPermission{
		BasePermission: Mutable,
		Fields: []Permission{
			Value,
		},
	},
	"m struct {v; l}": &StructPermission{
		BasePermission: Mutable,
		Fields: []Permission{
			Value,
			LinearValue,
		},
	},
	"_": &WildcardPermission{},
}

func helper() (perm Permission, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = r.(error)
		}
	}()
	perm = NewParser("error").parseBasePermission()
	return perm, nil
}

func TestParser(t *testing.T) {
	for input, expected := range testCasesParser {
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
	keys := make([]string, 0, len(testCasesParser))
	for input := range testCasesParser {
		keys = append(keys, input)
	}
	sort.Strings(keys)

	for _, input := range keys {
		expected := testCasesParser[input]
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

func TestParserParse_panic(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("did not panic")
		}
	}()
	var p *Parser
	p.Parse()
}
