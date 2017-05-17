// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package capabilities

import (
	"fmt"
	"go/ast"
	"go/token"
	"strings"

	"github.com/julian-klode/lingolang/permission"
	"github.com/julian-klode/lingolang/permission/parser"
)

// Checker is a type that keeps shared state from multiple check
// passes. It must be created by NewChecker().
type Checker struct {
	path   string
	fset   *token.FileSet
	files  []*ast.File
	conf   *Config
	info   *Info
	pmap   map[ast.Node]permission.Permission
	passes []pass
	// Errors occured during capability checking.
	Errors []error
}

// NewChecker returns a new checker with the specified settings.
func NewChecker(conf *Config, info *Info, path string, fset *token.FileSet, files ...*ast.File) *Checker {
	checker := &Checker{
		path:  path,
		fset:  fset,
		files: files,
		conf:  conf,
		info:  info,
		pmap:  make(map[ast.Node]permission.Permission),
	}
	// Configure all passes here.
	checker.passes = []pass{
		assignPass{checker: checker},
	}
	return checker
}

// Check performs the checks the checker was set up for.
//
// Returns the first error, if any. More errors can be found in the Errors
// field.
func (c *Checker) Check() (err error) {
	defer func() {
		r := recover()
		if _, ok := r.(bailout); r != nil && !ok {
			panic(r)
		}
		if len(c.Errors) > 0 {
			err = c.Errors[0]
		}
	}()
	// Perform the type check.
	_, err = c.conf.Types.Check(c.path, c.fset, c.files, &c.info.Types)
	if err != nil {
		c.Errors = append(c.Errors, err)
		return
	}

	// Run the individual capability checking passes.
	// TODO: Error handling.
	for _, p := range c.passes {
		for _, f := range c.files {
			ast.Walk(p, f)
		}
	}

	if len(c.Errors) > 0 {
		return c.Errors[0]
	}

	return
}

// bailout is a type to throw to end capability checking.
type bailout struct{}

// errorf inserts a new error into the error log.
func (c *Checker) errorf(format string, a ...interface{}) {
	c.Errors = append(c.Errors, fmt.Errorf(format, a...))
	if len(c.Errors) >= 10 {
		panic(bailout{})
	}
}

// pass is simply an interface that extends the ast.Visitor interface, and
// represents the individual passes of the type checker. Each pass my store
// its own data in its associated object. The state is shared between files
// of the same package.
type pass interface {
	ast.Visitor
}

// assignPass assigns annotations in comments to their associated nodes.
type assignPass struct {
	checker *Checker
	// Comment map for the active file
	commentMap ast.CommentMap
}

func (p assignPass) Visit(node ast.Node) (w ast.Visitor) {
	// We are entering a file, get its comment map.
	if f, ok := node.(*ast.File); ok {
		p.commentMap = ast.NewCommentMap(p.checker.fset, f, f.Comments)
	}

	// If we don't have comments for this node, no need to continue
	cmtGrps, ok := p.commentMap[node]
	if !ok {
		return p
	}

	// Iterate through the comment groups to find a comment in a group that
	// starts with @cap and parse the specification there.
	for _, cmtGrp := range cmtGrps {
		for _, cmt := range cmtGrp.List {
			text := cmt.Text[2:]
			if strings.HasPrefix(strings.TrimSpace(text), "@cap") {
				cap := text[strings.Index(text, "@cap")+len("@cap"):]

				perm, err := parser.NewParser(cap).Parse()
				if err != nil {
					p.checker.errorf("%s: Cannot parse permission: %s", p.checker.fset.Position(cmt.Slash), err)
				}
				p.checker.pmap[node] = perm
			}
		}
	}

	return p
}
