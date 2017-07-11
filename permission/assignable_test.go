// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package permission

import (
	"testing"
)

type assignableToTestCase struct {
	from        string
	to          string
	assignable  bool
	refcopyable bool
}

var testcasesAssignableTo = []assignableToTestCase{
	// Basic types
	{"om", "om", true, false},
	{"ov", "ov", true, true},
	{"om", "ov", true, false},
	{"ov", "om", false, false},
	// pointers
	{"om *ov", "om *om", false, false},
	{"om *om", "om *ov", true, false},
	{"ov *ov", "ov *ov", true, true},
	// channels
	{"om chan ov", "om chan om", false, false},
	{"om chan om", "om chan ov", true, false},
	{"ov chan ov", "ov chan ov", true, true}, // useless chan?
	// Array slice
	{"ov []ov", "ov []ov", true, true},
	{"ov [1]ov", "ov [1]ov", true, true},
	{"om []om", "om []ov", true, false},
	{"om [1]om", "om [1]ov", true, false},
	{"om []om", "om [1]ov", false, false},
	{"om [1]om", "om []ov", false, false},
	{"ov [1]ov", "ov []ov", false, true}, // can refcopy array to slice
	{"om []ov", "om []om", false, false},
	{"ov []ov", "om []ov", false, false},
	{"om []ov", "ov []ov", true, false},
	// channels
	{"ov map[ov] ov", "ov map[ov] ov", true, true},
	{"om map[ov] ov", "om map[om] ov", false, false},
	{"om map[om] ov", "om map[ov] ov", true, false},
	{"om map[ov] ov", "om map[ov] om", false, false},
	{"om map[ov] om", "om map[ov] ov", true, false},
	{"om map[ov] ov", "om map[ov] ov", true, false},
	// structs
	{"om struct {om}", "om struct {ov}", true, false},
	{"om struct {ov}", "om struct {om}", false, false},
	{"om struct {ov}", "ov struct {ov}", true, false},
	{"ov struct {ov}", "om struct {ov}", false, false}, // TODO: We can copy.
	{"ov struct {ov}", "ov struct {ov}", true, true},
	// Incompatible types
	{"om", "om func ()", false, false},
	{"om func ()", "om", false, false},
	{"om interface {}", "om", false, false},
	{"om chan om", "om", false, false},
	{"om map[om] om", "om", false, false},
	{"om struct {om}", "om", false, false},
	{"om *om", "om", false, false},
	{"om []om", "om", false, false},
	{"om [1]om", "om", false, false},
	// Functions themselves are complicated: Here writeable actually means
	// that the function can return different values for the same arguments,
	// hence we can assign a constant function to a non-constant one.
	{"ov (ov) func ()", "ov (ov) func ()", true, true},
	{"om (ov) func ()", "om (ov) func ()", true, false},
	{"om (ov) func ()", "ov (ov) func ()", false, false},
	{"ov (ov) func ()", "om (ov) func ()", true, false},
	// Owned functions can be assigned to unowned however, and not vice versa
	{"om (ov) func ()", "m (ov) func ()", true, false},
	{"m (ov) func ()", "om (ov) func ()", false, false},
	// A function accepting mutable values cannot be used
	// as a function accepting values, but vice versa it works.
	{"om (om) func ()", "om (ov) func ()", false, false},
	{"om (ov) func ()", "om (om) func ()", true, false},
	{"om func (om)", "om func (ov)", false, false},
	{"om func (ov)", "om func (om)", true, false},
	// Function results: May be wider (excess rights stripped away)
	{"om func (om) ov", "om func (om) om", false, false},
	{"om func (om) om", "om func (om) ov", true, false},

	// Interfaces
	{"ov interface{}", "ov interface{}", true, true},
	{"om interface{}", "ov interface{}", true, false},
	{"ov interface{}", "om interface{}", false, false},
	{"om interface{}", "m interface{}", true, false},
	{"m interface{}", "om interface{}", false, false},
	// TODO: We might actually have to do things differently. These are
	// actually somewhat inconsistent, the receiver is not expanded to the
	// type.
	{"om interface { ov (om) func()}", "om interface { om (om) func()}", true, false},
	{"om interface { om (om) func()}", "om interface { ov (om) func()}", false, false},
	{"ov interface { ov (ov) func()}", "ov interface { ov (ov) func()}", true, true},
	{"ov interface { om (om) func()}", "ov interface { om (om) func()}", true, true},
	{"ov interface { om (om) func()}", "ov interface { ov (om) func()}", false, false},

	// FIXME: Do we actually want to allow permissions like these?
	{"ov struct {ov}", "ov struct {om}", false, false}, // TODO: We can copy.
	{"ov func (om)", "ov func (om)", true, true},
	{"ov func (om)", "ov func (ov)", false, false},
	{"ov func (om) (om)", "ov func (om) (ov)", true, true},
	{"ov func (om) (ov)", "ov func (om) (om)", false, false},
	{"ov (ov) func (om)", "ov (ov) func (om)", true, true},
	{"ov (ov) func (om)", "ov (om) func (om)", true, true},
	{"ov (om) func (om)", "ov (ov) func (om)", false, false},
}

func TestAssignableTo(t *testing.T) {

	for _, testCase := range testcasesAssignableTo {
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
			assignable := MovableTo(p1, p2)
			if assignable != testCase.assignable {
				t.Errorf("Unexpected result %v, expected %v", assignable, testCase.assignable)
			}
			refcopyable := RefcopyableTo(p1, p2)
			if refcopyable != testCase.refcopyable {
				t.Errorf("Unexpected result %v, expected %v", refcopyable, testCase.refcopyable)
			}
		})
	}
}

func TestAssignableTo_Recursive(t *testing.T) {
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

	if RefcopyableTo(recursiveType, recursiveType) {
		t.Error("Could move recursive type to itself")
	}
	if RefcopyableTo(recursiveType, Mutable) {
		t.Error("Could move recursive type to mutable value")
	}
	if RefcopyableTo(recursiveType, nonrecursiveType) {
		t.Error("Could move recursive type to non-recursive type")
	}
	if RefcopyableTo(nonrecursiveType, recursiveType) {
		t.Error("Could move non-recursive type to recursive type")
	}

}
