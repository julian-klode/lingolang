// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

// Package capabilities implements the algorithms for checking capabilities
// in a lingolang program. It is loosely modelled after the API of the standard
// go/types package, which implements type checking.
package capabilities

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/julian-klode/lingolang/permission"
)

// Config configures the capability checker.
type Config struct {
	// Parent value from the types package.
	Types types.Config
}

// Info stores the results of a capability check.
type Info struct {
	// Parent object from the types package.
	Types types.Info

	// Permissions associates nodes with permissions.
	Permissions map[ast.Node]permission.Permission

	// Errors occured during capability checking.
	Errors []error
}

// Check performs a capability check on a package.
func (conf *Config) Check(path string, fset *token.FileSet, files []*ast.File, info *Info) error {
	checker := NewChecker(conf, info, path, fset, files...)
	err := checker.Check()
	info.Errors = checker.Errors
	if err != nil {
		return err
	}

	info.Permissions = checker.pmap
	return nil
}
