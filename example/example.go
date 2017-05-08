// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

/*
Package example provides examples for the lingolang project.

At the current state, these examples do not have a concrete syntax for the
annotations, as this is still being figured out.

Capability specification - Here be dragons

	// Entry point.
	<spec>  ::= <cspec> <rspec> | <rspec>

	// Value modifiers
	<rspec> ::= func (<rec spec>)(<par spec>)<ret spec>
		     | map [<rspec>] <rspec>
			 | chan <rspec>
			 | interface <rspec>
			 | * <rspec>
			 | [] <rspec>
			 | <cspec>

	// Access modifiers: Capabilities + Ownership

	<cspec> ::=
			 | [o](r|w|i|R|W|I)+        - caps: own,read,write,id,excl. read,...
			 | [o|u](m|l|c|v|a)         - Short cuts for below
			 | [[un]owned] mutable		- [o]rwiRW
			 | [[un]owned] linear		- [o]riRW
			 | [[un]owned] const		- [o]ri
			 | [[un]owned] value		- [o]riW
			 | [[un]owned] any          - [o]rwi

// TODO: What does identity permission mean in a language that has no refs?
*/
package example

// Foo is a bastard
// Do we want to allow annotating fields in the annotation for the type?
//
// @cap(c, Value=c, Function=cap(c, x=c, ret=c), Fun2=c fun(c)c)
type Foo struct {
	Value    int             // @cap c
	Function func(x int) int // @cap c func(c) c
	Fun2     func(int) int   // @cap c func(c) c
}

// ChannelOfMutableFoo is a channel of mutable Foo objects.
//
// TODO: Same as: @cap m chan *m or what?
//
// @cap chan *m
var ChannelOfMutableFoo = make(chan *Foo, 0)

// Constant is a constant variable. You can't write to it, and nobody else
// can (it has the exclusive write right, but no write right). Either of the
// following annotations works:
//
// @cap orW
// @cap ov
var Constant = 5

// MutableState is a linear mutable state. I don't think that works.
//
// @cap orwRW
var MutableState int

// ObjectCache caches a read-only object (value).
//
// The following reads as: This is a mutable pointer to a value.
//
// @cap m *v
// @cap rwRW*rWo
var ObjectCache *interface{}

// InterfaceWithMutableMethods represents an interface with methods for
// mutable, value, and const receivers.
//
// TODO: Allow specifying interface field annotations in the interface?
type InterfaceWithMutableMethods interface {
	MutableMethod(x int) // @cap u m
	ValueMethod(x int)   // @cap o v
	ConstMethod(x int)   // @cap u c
}

// Examples contains various annotations for a lot of things, including a
// method body.
//
// TODO: How should we annotate parameters?
//
// @cap(om, x=oc, return[0]=oc, return[1]=m)	   or rather:
// @cap om, x: oc, return[0]: oc, return[1]: m
// @cap om func(oc)(oc, m)					       Just an alternative either way
//
// Or just in the fields themselves (that looks ugly)?
func Examples(x interface{} /*@cap oc */) (interface{} /*@cap c*/, interface{} /*@cap m*/) {
	switch t := x.(type) {
	// Erasure hole. Capabilitities are compile-time only. Don't specify the
	// cap inline, as that binds to the identifier rather than the case ...
	// @cap m
	case int:
		return x, t
	}

	// Annotate a variable shortcut thingy
	// @cap m
	z := 5

	// Multiple variables
	// @cap v, m
	x, y := Examples(x)

	// The real variable annotation
	var a = y.(int) // @cap v

	// Can we annotate a type assertion?
	if y.(*Foo /*@cap v * v */) != nil && a > z {
		return x, z
	}
	return x, a
}

// @acap
// @rcap
// @cap
