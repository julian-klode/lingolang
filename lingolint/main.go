package main

import (
	"fmt"

	"github.com/julian-klode/lingolang/permission"
	"github.com/julian-klode/lingolang/permission/parser"
)

func main() {
	fmt.Printf("Owned = %v:%T\n", permission.Owned, permission.Owned)
	fmt.Printf("Read = %v:%T\n", permission.Read, permission.Read)
	fmt.Printf("Write = %v:%T\n", permission.Write, permission.Write)
	fmt.Printf("ExclRead = %v:%T\n", permission.ExclRead, permission.ExclRead)
	fmt.Printf("ExclWrite = %v:%T\n", permission.ExclWrite, permission.ExclWrite)

	sc := parser.NewScanner("of (or) func (oa, ob) oR")
	for tok := sc.Scan(); tok.Type != parser.EndOfFile; tok = sc.Scan() {
		fmt.Printf("Token %#v \n", tok)
	}

	p := parser.NewParser("om map [ov] ol")
	perm, err := p.Parse()
	fmt.Printf("Parsed %v with error %v", perm, err)

}
