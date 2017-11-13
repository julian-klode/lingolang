// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package permission

import (
	"fmt"
	"strings"
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
	{"ow", "ow", false, true, false},
	{"ov", "ov", true, true, true},
	{"om", "ov", true, false, true},
	{"ov", "om", false, false, true},
	{&NilPermission{}, "om chan om", true, true, true},
	{&NilPermission{}, "om func () n", true, true, true},
	{&NilPermission{}, "om interface{}", true, true, true},
	{&NilPermission{}, "om map[om] om", true, true, true},
	{&NilPermission{}, &NilPermission{}, true, true, true},
	{&NilPermission{}, "om * om", true, true, true},
	{&NilPermission{}, "om [] om", true, true, true},
	{&NilPermission{}, "om", false, false, false},
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
	{"ov [1]ov", "ov []ov", false, false, false},
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
	// Inaccurate test: The receiver might actually be recursive, if no
	// @perm declaration is given, the receiver actually becomes identical (!)
	// to the interface permission itself.
	{"om interface { ov (om) func()}", "om interface { om (om) func()}", true, false, false},
	{"om interface { om (om) func()}", "om interface { ov (om) func()}", false, false, false},
	{"ov interface { ov (ov) func()}", "ov interface { ov (ov) func()}", true, true, true},
	{"ov interface { om (om) func()}", "ov interface { om (om) func()}", true, true, true},
	{"ov interface { om (om) func()}", "ov interface { ov (om) func()}", false, false, false},

	// An inconsistent permission. Should not be possible to actually generate (atm).
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
	recursiveType.Methods = []*FuncPermission{
		&FuncPermission{
			BasePermission: Mutable,
			Receivers:      []Permission{recursiveType},
		},
	}
	var nonrecursiveType = &InterfacePermission{BasePermission: Mutable,
		Methods: []*FuncPermission{
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

func TestAssignableToBase_InvalidMode(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil || !strings.Contains(fmt.Sprint(r), "Unreachable, assign mode is -1") {
			t.Fatalf("Did not panic or produced unexpected panic %v", r)
		}

	}()
	Mutable.isAssignableTo(Mutable, assignableState{nil, -1})
}

func TestAssignableTo_InterfaceSubset(t *testing.T) {
	a := &InterfacePermission{Owned | Mutable, []*FuncPermission{
		&FuncPermission{Owned | Mutable, "func1", nil, nil, nil},
		&FuncPermission{Owned | Mutable, "func2", nil, nil, nil},
	}}
	b := &InterfacePermission{Owned | Mutable, []*FuncPermission{
		&FuncPermission{Owned | Mutable, "func2", nil, nil, nil},
	}}
	if !MovableTo(a, b) {
		t.Fatalf("Cannot move a to b")
	}
	func() {
		defer func() {
			r := recover()
			if r == nil || !strings.Contains(fmt.Sprint(r), "Trying to move method func1, but does not exist in source") {
				t.Fatalf("Did not panic or produced unexpected panic %v", r)
			}

		}()
		if !MovableTo(b, a) {
			t.Fatalf("Cannot move a to b")

		}
	}()
}
