// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package permission

import (
	"fmt"
	"testing"
)

type assignableToTestCase struct {
	from        interface{}
	to          interface{}
	assignable  bool
	refcopyable bool
	copyable    bool
}

var testcasesAssignableTo = []assignableToTestCase{
	// Basic types
	{"om", "om", true, false, true},
	{"ow", "ow", true, true, false}, // TODO: Move should be false too
	{"ov", "ov", true, true, true},
	{"om", "ov", true, false, true},
	{"ov", "om", false, false, true},
	// pointers
	{"om *ov", "om *om", false, false, false},
	{"om *om", "om *ov", true, false, false},
	{"ov *ov", "ov *ov", true, true, true},
	{"om * om", "orR * om", true, false, false},
	{"om * om", "or * or", true, false, false},
	// channels
	{"om chan ov", "om chan om", false, false, false},
	{"om chan om", "om chan ov", true, false, false},
	{"ov chan ov", "ov chan ov", true, true, true}, // useless chan?
	// Array slice
	{"ov []ov", "ov []ov", true, true, true},
	{"ov [1]ov", "ov [1]ov", true, true, true},
	{"om []om", "om []ov", true, false, false},
	{"om [1]om", "om [1]ov", true, false, true},
	{"om []om", "om [1]ov", false, false, false},
	{"om [1]om", "om []ov", false, false, false},
	{"ov [1]ov", "ov []ov", false, true, false}, // can refcopy array to slice
	{"om []ov", "om []om", false, false, false},
	{"ov []ov", "om []ov", false, false, false},
	{"om []ov", "ov []ov", true, false, false},
	// channels
	{"ov map[ov] ov", "ov map[ov] ov", true, true, true},
	{"om map[ov] ov", "om map[om] ov", false, false, false},
	{"om map[om] ov", "om map[ov] ov", true, false, false},
	{"om map[ov] ov", "om map[ov] om", false, false, false},
	{"om map[ov] om", "om map[ov] ov", true, false, false},
	{"om map[ov] ov", "om map[ov] ov", true, false, false},
	// structs
	{"om struct {om}", "om struct {ov}", true, false, true},
	{"om struct {ov}", "om struct {om}", false, false, true},
	{"om struct {ov}", "ov struct {ov}", true, false, true},
	{"ov struct {ov}", "om struct {ov}", false, false, true},
	{"ov struct {ov}", "ov struct {ov}", true, true, true},
	{"ov struct {om * om}", "ov struct {om * om}", true, false, false},
	// Incompatible types
	{"om", "om func ()", false, false, false},
	{"om func ()", "om", false, false, false},
	{"om interface {}", "om", false, false, false},
	{"om chan om", "om", false, false, false},
	{"om map[om] om", "om", false, false, false},
	{"om struct {om}", "om", false, false, false},
	{"om *om", "om", false, false, false},
	{"om []om", "om", false, false, false},
	{"om [1]om", "om", false, false, false},
	// Functions themselves are complicated: Here writeable actually means
	// that the function can return different values for the same arguments,
	// hence we can assign a constant function to a non-constant one.
	{"ov (ov) func ()", "ov (ov) func ()", true, true, true},
	{"om (ov) func ()", "om (ov) func ()", true, false, false},
	{"om (ov) func ()", "ov (ov) func ()", false, false, false},
	{"ov (ov) func ()", "om (ov) func ()", true, false, false},
	// Owned functions can be assigned to unowned however, and not vice versa
	{"om (ov) func ()", "m (ov) func ()", true, false, false},
	{"m (ov) func ()", "om (ov) func ()", false, false, false},
	// A function accepting mutable values cannot be used
	// as a function accepting values, but vice versa it works.
	{"om (om) func ()", "om (ov) func ()", false, false, false},
	{"om (ov) func ()", "om (om) func ()", true, false, false},
	{"om func (om)", "om func (ov)", false, false, false},
	{"om func (ov)", "om func (om)", true, false, false},
	// Function results: May be wider (excess rights stripped away)
	{"om func (om) ov", "om func (om) om", false, false, false},
	{"om func (om) om", "om func (om) ov", true, false, false},

	// Interfaces
	{"ov interface{}", "ov interface{}", true, true, true},
	{"om interface{}", "ov interface{}", true, false, false},
	{"ov interface{}", "om interface{}", false, false, false},
	{"om interface{}", "m interface{}", true, false, false},
	{"m interface{}", "om interface{}", false, false, false},
	// TODO: The receiver might actually be recursive, if no @cap declaration is
	// given, the receiver actually becomes identical (!) to the interface
	// permission itself. That's not possible to express in this syntax, and
	// usually not wanted anyway.
	{"om interface { ov (om) func()}", "om interface { om (om) func()}", true, false, false},
	{"om interface { om (om) func()}", "om interface { ov (om) func()}", false, false, false},
	{"ov interface { ov (ov) func()}", "ov interface { ov (ov) func()}", true, true, true},
	{"ov interface { om (om) func()}", "ov interface { om (om) func()}", true, true, true},
	{"ov interface { om (om) func()}", "ov interface { ov (om) func()}", false, false, false},

	// FIXME: Do we actually want to allow permissions like these?
	{"ov struct {ov}", "ov struct {om}", false, false, true},
	{"ov func (om)", "ov func (om)", true, true, true},
	{"ov func (om)", "ov func (ov)", false, false, false},
	{"ov func (om) (om)", "ov func (om) (ov)", true, true, true},
	{"ov func (om) (ov)", "ov func (om) (om)", false, false, false},
	{"ov (ov) func (om)", "ov (ov) func (om)", true, true, true},
	{"ov (ov) func (om)", "ov (om) func (om)", true, true, true},
	{"ov (om) func (om)", "ov (ov) func (om)", false, false, false},
	{"ov struct { ov []ov }", "ov struct {ov [] ov}", true, true, true},
	{"om * om", "m * m", true, false, false},
	{"_", "om", false, false, false},

	// Tuples
	{tuplePermission{"om", "om"}, tuplePermission{"om", "ov"}, true, false, true},
	{tuplePermission{"om", "ov"}, tuplePermission{"om", "om"}, false, false, true},
	{tuplePermission{"om", "ov"}, tuplePermission{"ov", "ov"}, true, false, true},
	{tuplePermission{"ov", "ov"}, tuplePermission{"om", "ov"}, false, false, true},
	{tuplePermission{"ov", "ov"}, tuplePermission{"ov", "ov"}, true, true, true},
	{tuplePermission{"ov", "om * om"}, tuplePermission{"ov", "om * om"}, true, false, false},
	{tuplePermission{"or", "or"}, tuplePermission{"or", "or"}, true, true, true},
	{tuplePermission{"or", "or"}, tuplePermission{"or", "or", "ov"}, false, false, false},
	{tuplePermission{"or", "or"}, "ov", false, false, false},
}

func TestAssignableTo(t *testing.T) {

	for _, testCase := range testcasesAssignableTo {
		testCase := testCase
		t.Run(fmt.Sprint(testCase.from)+"=> "+fmt.Sprint(testCase.to), func(t *testing.T) {
			p1, err := MakePermission(testCase.from)
			if err != nil {
				t.Fatalf("Invalid from: %v", err)
			}
			p2, err := MakePermission(testCase.to)
			if err != nil {
				t.Fatalf("Invalid to: %v", err)
			}
			assignable := MovableTo(p1, p2)
			if assignable != testCase.assignable {
				t.Errorf("Unexpected move result %v, expected %v", assignable, testCase.assignable)
			}
			refcopyable := RefcopyableTo(p1, p2)
			if refcopyable != testCase.refcopyable {
				t.Errorf("Unexpected refcopy result %v, expected %v", refcopyable, testCase.refcopyable)
			}
			copyable := CopyableTo(p1, p2)
			if copyable != testCase.copyable {
				t.Errorf("Unexpected copy result %v, expected %v", copyable, testCase.copyable)
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
