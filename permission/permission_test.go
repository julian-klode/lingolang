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
