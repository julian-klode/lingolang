package main

import (
	"fmt"
	"go/ast"
	goparser "go/parser"
	"go/token"
	"log"

	"github.com/julian-klode/lingolang/capabilities"
	"github.com/julian-klode/lingolang/permission"
)

func main() {
	fmt.Printf("Owned = %v:%T\n", permission.Owned, permission.Owned)
	fmt.Printf("Read = %v:%T\n", permission.Read, permission.Read)
	fmt.Printf("Write = %v:%T\n", permission.Write, permission.Write)
	fmt.Printf("ExclRead = %v:%T\n", permission.ExclRead, permission.ExclRead)
	fmt.Printf("ExclWrite = %v:%T\n", permission.ExclWrite, permission.ExclWrite)

	sc := permission.NewScanner("of (or) func (oa, ob) oR")
	for tok := sc.Scan(); tok.Type != permission.TokenEndOfFile; tok = sc.Scan() {
		fmt.Printf("Token %#v \n", tok)
	}

	p := permission.NewParser("om map [ov] ol")
	perm, err := p.Parse()
	fmt.Printf("Parsed %v with error %v\n", perm, err)

	// Parse one file.
	fset := token.NewFileSet()
	f, err := goparser.ParseFile(fset, "/home/jak/Projects/Go/src/github.com/golang/example/gotypes/defsuses/example/test.go",
		`package main
				// Bananas
				// @perm ol
				// Mango
				var a = 5

				func foo() {
					var x = 9
					println(x)
				}
			`, goparser.ParseComments)
	if err != nil {
		log.Fatal(err) // parse error
	}

	config := capabilities.Config{}
	info := capabilities.Info{}
	err = config.Check("hello", fset, []*ast.File{f}, &info)
	if err != nil {
		panic(err)
	}

}
