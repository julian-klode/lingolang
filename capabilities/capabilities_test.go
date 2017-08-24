// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

// Not the most sophisticated tests, really.

package capabilities

import (
	"go/ast"
	goparser "go/parser"
	"go/token"
	"testing"
)

func TestCapabilitiesSuccess(t *testing.T) {
	// Parse one file.
	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, "TestCapabilitiesSuccess.go",
		`package main
            // Bananas
            // @cap ol
            // Mango
            var a = 5`, goparser.ParseComments)
	if err != nil {
		t.Fatalf("Parse error: %s", err) // parse error
	}

	config := Config{}
	info := Info{}
	err = config.Check("hello", fset, []*ast.File{f}, &info)
	if err != nil {
		t.Errorf("err = %s, expected %s", err, "Error for parsing x")
	}
	if len(info.Permissions) != 1 {
		t.Errorf("have %v, expected one permission", info.Permissions)
	}
}

func TestCapabilitiesError(t *testing.T) {
	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, "TestCapabilitiesError.go",
		`package main
            // Bananas
            // @cap olx
            // Mango
            var a = 5
            func foo() {
				// @cap error
                var x = 9
                println(x)
            }`, goparser.ParseComments)
	if err != nil {
		t.Fatalf("Parse error: %s", err) // parse error
	}

	config := Config{}
	info := Info{}
	err = config.Check("hello", fset, []*ast.File{f}, &info)

	if err == nil {
		t.Errorf("err = %s, expected %s", err, "Error for parsing x")
	}
	if len(info.Permissions) != 0 {
		t.Errorf("have %v, expected no permission", info.Permissions)
	}
}

func TestCapabilitiesErrorBailout(t *testing.T) {
	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, "TestCapabilitiesErrorBailout.go",
		`package main
            var a = 5		// @cap olx
            var b = 5		// @cap olx
            var c = 5		// @cap olx
            var d = 5		// @cap olx
            var e = 5		// @cap olx
            var f = 5		// @cap olx
            var g = 5		// @cap olx
            var h = 5		// @cap olx
            var i = 5		// @cap olx
            var j = 5		// @cap olx
            var k = 5		// @cap olx`, goparser.ParseComments)
	if err != nil {
		t.Fatalf("Parse error: %s", err) // parse error
	}

	config := Config{}
	info := Info{}
	err = config.Check("hello", fset, []*ast.File{f}, &info)

	if err == nil {
		t.Errorf("err is nil, expected an error.")
	}
	if len(info.Errors) != 10 {
		t.Errorf("have %v, expected ten errors", info.Errors)
	}
}

func TestCapabilitiesTypeError(t *testing.T) {
	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, "TestCapabilitiesTypeError.go",
		`package main
            var a = 5		// @cap ol
            var a = 5 		// @cap ol`, goparser.ParseComments)
	if err != nil {
		t.Fatalf("Parse error: %s", err) // parse error
	}

	config := Config{}
	info := Info{}
	err = config.Check("hello", fset, []*ast.File{f}, &info)

	if err == nil {
		t.Errorf("err is nil, expected an error.")
	}
	if len(info.Errors) != 1 {
		t.Errorf("have %v, expected ten errors", info.Errors)
	}
}

func TestChecker_Files_panic(t *testing.T) {
	defer func() {
		e := recover()
		if e == nil {
			t.Error("did not panic")
		}
	}()
	var c *Checker
	c.Files(nil)
}
