// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package permission

import (
	"testing"
)

type movableToTestCase struct {
	from   string
	to     string
	result bool
}

var testcasesMovableTo = []movableToTestCase{
	// Basic types
	{"om", "ov", true},
	{"ov", "om", false},
	// pointers
	{"om *ov", "om *om", false},
	{"om *om", "om *ov", true},
	// channels
	{"om chan ov", "om chan om", false},
	{"om chan om", "om chan ov", true},
	// Array slice
	{"om []om", "om []ov", true},
	{"om [1]om", "om [1]ov", true},
	{"om []om", "om [1]ov", false},
	{"om [1]om", "om []ov", false},
	{"om []ov", "om []om", false},
	{"ov []ov", "om []ov", false},
	{"om []ov", "ov []ov", true},
	// channels
	{"om map[ov] ov", "om map[om] ov", false},
	{"om map[om] ov", "om map[ov] ov", true},
	{"om map[ov] ov", "om map[ov] om", false},
	{"om map[ov] om", "om map[ov] ov", true},
	{"om map[ov] ov", "om map[ov] ov", true},
	// structs
	{"om struct {om}", "om struct {ov}", true},
	{"om struct {ov}", "om struct {om}", false},
	{"om struct {ov}", "ov struct {ov}", true},
	{"ov struct {ov}", "om struct {ov}", false}, // TODO: We can copy.
	// Incompatible types
	{"om", "om func ()", false},
	{"om func ()", "om", false},
	{"om interface {}", "om", false},
	{"om chan om", "om", false},
	{"om map[om] om", "om", false},
	{"om struct {om}", "om", false},
	{"om *om", "om", false},
	{"om []om", "om", false},
	// Functions themselves are complicated: Here writeable actually means
	// that the function can return different values for the same arguments,
	// hence we can assign a constant function to a non-constant one.
	{"om (ov) func ()", "om (ov) func ()", true},
	{"om (ov) func ()", "ov (ov) func ()", false},
	{"ov (ov) func ()", "om (ov) func ()", true},
	// Owned functions can be assigned to unowned however, and not vice versa
	{"om (ov) func ()", "m (ov) func ()", true},
	{"m (ov) func ()", "om (ov) func ()", false},
	// A function accepting mutable values cannot be used
	// as a function accepting values, but vice versa it works.
	{"om (om) func ()", "om (ov) func ()", false},
	{"om (ov) func ()", "om (om) func ()", true},
	{"om func (om)", "om func (ov)", false},
	{"om func (ov)", "om func (om)", true},
	// Function results: May be wider (excess rights stripped away)
	{"om func (om) ov", "om func (om) om", false},
	{"om func (om) om", "om func (om) ov", true},

	// Interfaces
	{"om interface{}", "ov interface{}", true},
	{"ov interface{}", "om interface{}", false},
	{"om interface{}", "m interface{}", true},
	{"m interface{}", "om interface{}", false},
	// TODO: We might actually have to do things differently. These are
	// actually somewhat inconsistent, the receiver is not expanded to the
	// type.
	{"om interface { ov (om) func()}", "om interface { om (om) func()}", true},
	{"om interface { om (om) func()}", "om interface { ov (om) func()}", false},
}

func TestMovableTo(t *testing.T) {

	for _, testCase := range testcasesMovableTo {
		testCase := testCase
		t.Run(testCase.from+"=> "+testCase.to, func(t *testing.T) {
			p1, err := NewParser(testCase.from).Parse()
			if err != nil {
				t.Fatalf("Invalid from: %v", err)
			}
			p2, err := NewParser(testCase.to).Parse()
			if err != nil {
				t.Fatalf("Invalid to: %v", err)
			}
			result := MovableTo(p1, p2)
			if result != testCase.result {
				t.Errorf("Unexpected result %v, expected %v", result, testCase.result)
			}
		})
	}
}

func TestMovableTo_Recursive(t *testing.T) {
	var recursiveType = &InterfacePermission{BasePermission: Mutable}
	recursiveType.Methods = []Permission{
		&FuncPermission{
			BasePermission: Mutable,
			Receivers:      []Permission{recursiveType},
		},
	}
	var nonrecursiveType = &InterfacePermission{BasePermission: Mutable,
		Methods: []Permission{
			&FuncPermission{
				BasePermission: Mutable,
				Receivers:      []Permission{Mutable},
			},
		},
	}
	if !MovableTo(recursiveType, recursiveType) {
		t.Error("Cannot move recursive type to itself")
	}

	if MovableTo(recursiveType, Mutable) {
		t.Error("Could move recursive type to mutable value")
	}
	if MovableTo(recursiveType, nonrecursiveType) {
		t.Error("Could move recursive type to non-recursive type")
	}
	if MovableTo(nonrecursiveType, recursiveType) {
		t.Error("Could move non-recursive type to recursive type")
	}
}
