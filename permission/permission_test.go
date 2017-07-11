// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package permission

import "testing"

var testcasesPermissionString = []struct {
	perm     BasePermission
	expected string
}{
	// Basic combinations
	{Owned | Mutable, "om"},
	{Owned | LinearValue, "ol"},
	{Owned | Value, "ov"},
	{Owned | ReadOnly, "or"},
	{Owned | None, "on"},
	{Mutable, "m"},
	{LinearValue, "l"},
	{Value, "v"},
	{Any, "a"},
	{ReadOnly, "r"},
	{None, "n"},
	{Write, "w"},
	// Other cases
	{Write | ExclWrite, "wW"},
	{Write | ExclRead, "wR"},
	{Read | ExclRead, "rR"},
}

func TestPermissionString(t *testing.T) {
	for _, testCase := range testcasesPermissionString {
		testCase := testCase
		t.Run(testCase.expected, func(t *testing.T) {
			result := testCase.perm.String()
			if result != testCase.expected {
				t.Errorf("Unexpected result %s, expected %s", result, testCase.expected)
			}
		})
	}
}

var testcasesPermissionIsLinear = []struct {
	perm     string
	expected bool
}{
	{"m", true},
	{"v", false},
	{"l", true},
	{"r", false},
	{"w", false},
	{"a", false},
	{"om chan om", true},
	{"om chan ov", true},
	{"ov chan ov", false},
	{"m * v", true},
	{"v * v", false},
	{"m struct {v}", true},
	{"v struct {v}", false},
	{"m [] v", true},
	{"v [] v", false},
	{"m [1] v", true},
	{"v [1] v", false},
	{"m chan v", true},
	{"v chan v", false},
	{"m map[v] v", true},
	{"v map[v] v", false},
	{"m interface {}", true},
	{"v interface {}", false},
	{"m func ()", true},
	{"v func ()", false},
}

func TestPermissionIsLinear(t *testing.T) {
	for _, testCase := range testcasesPermissionIsLinear {
		testCase := testCase
		t.Run(testCase.perm, func(t *testing.T) {
			p1, err := NewParser(testCase.perm).Parse()
			if err != nil {
				t.Fatalf("Invalid from: %v", err)
			}
			result := p1.IsLinear()
			if result != testCase.expected {
				t.Errorf("Unexpected result %v, expected %v", result, testCase.expected)
			}
		})
	}
}
