package main

import (
	"fmt"

	"github.com/julian-klode/lingolang/permission"
)

func main() {
	fmt.Printf("Owned = %v:%T\n", lingo.Owned, lingo.Owned)
	fmt.Printf("Read = %v:%T\n", lingo.Read, lingo.Read)
	fmt.Printf("Write = %v:%T\n", lingo.Write, lingo.Write)
	fmt.Printf("ExclRead = %v:%T\n", lingo.ExclRead, lingo.ExclRead)
	fmt.Printf("ExclWrite = %v:%T\n", lingo.ExclWrite, lingo.ExclWrite)
}
