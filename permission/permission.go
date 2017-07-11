// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

// Package permission implements the data structures and algorithms for parsing
// permission annotations, infering permissions, and checking permissions for
// correctness.
package permission

// BasePermission represents a set of read, write, owned, and exclusive flags,
// describing how a value may be used.
type BasePermission int

// BasePermission flags. These can be combined via OR to get a usable
// base permission.
const (
	// Owned means that the value is owned. An unowned value cannot be stored
	// into an owned value, essentially.
	Owned BasePermission = 1 << iota
	// Read means the value can be read from.
	Read BasePermission = 1 << iota
	// Write means the value can be written from
	Write BasePermission = 1 << iota
	// ExclRead is the exclusive read permission, meaning no other alias has
	// read access. It does not imply read permission.
	ExclRead BasePermission = 1 << iota
	// ExclWrite is the exclusive write permission, meaning no other alias has
	// write access.  It does not imply write permission.
	ExclWrite BasePermission = 1 << iota
)

// The basic builtin shortcuts to permissions.
const (
	// A linear mutable type
	Mutable BasePermission = Read | Write | ExclRead | ExclWrite
	// A linear immutable type
	LinearValue BasePermission = Read | ExclRead | ExclWrite
	// A non-linear value type that ensures nobody else is writing to the
	// value.
	Value BasePermission = Read | ExclWrite
	// A non-linear read-only reference to a value that might be written from
	// other references. TODO: Does this permission even make sense?
	ReadOnly BasePermission = Read
	// Unsafe: Any is used for return values of external Go functions.
	Any BasePermission = Owned | Read | Write
	// Unsafe: None is used for parameters of external Go functions.
	None BasePermission = 0
)

// String renders the base permission in its Canonical form.
func (perm BasePermission) String() string {
	var result string
	if perm&Owned != 0 {
		result = "o"
	}

	switch perm &^ Owned {
	case Mutable:
		return result + "m"
	case LinearValue:
		return result + "l"
	case Value:
		return result + "v"
	case ReadOnly:
		return result + "r"
	case Any &^ Owned:
		return "a" // special case: any implies owned.
	case None:
		return result + "n"
	default:
		if perm&Read != 0 {
			result += "r"
		}
		if perm&Write != 0 {
			result += "w"
		}
		if perm&ExclRead != 0 {
			result += "R"
		}
		if perm&ExclWrite != 0 {
			result += "W"
		}
		return result
	}
}

// isLinear checks if the type is linear
func (perm BasePermission) isLinear() bool {
	return (perm&(ExclWrite) != 0 && perm&Write != 0) || (perm&(ExclRead) != 0 && perm&Read != 0)
}

// Permission is an entity associated with an value that describes in which
// ways the value can be used.
type Permission interface {
	isMovableTo(p2 Permission, state movableState) bool
	isRefcopyableTo(p2 Permission, state refcopyableState) bool
	isCopyableTo(p2 Permission) bool
}

// PointerPermission describes permissions on a pointer value.
type PointerPermission struct {
	BasePermission BasePermission // The permission on the pointer value itself
	Target         Permission     // The permission of the value we are pointing to
}

// ChanPermission describes permissions on channels and their elements.
type ChanPermission struct {
	BasePermission    BasePermission // The permission on the chan value itself
	ElementPermission Permission     // The permission of the elements it contains
}

// ArrayPermission describes permissions on arrays
type ArrayPermission struct {
	BasePermission    BasePermission // The permission on the array/slice value itself
	ElementPermission Permission     // The permission of the elements it contains
}

// SlicePermission describes permissions on slices
type SlicePermission struct {
	BasePermission    BasePermission // The permission on the array/slice value itself
	ElementPermission Permission     // The permission of the elements it contains
}

// MapPermission describes permissions on map values, their keys and values.
type MapPermission struct {
	BasePermission  BasePermission // The permission of the map itself
	KeyPermission   Permission     // The permission of contained keys
	ValuePermission Permission     // The permission of contained values
}

// StructPermission describes permissions of structs.
type StructPermission struct {
	BasePermission BasePermission // Permission of the struct itself
	Fields         []Permission   // Permissions of the fields, in order
}

// FuncPermission describes permissions of functions
type FuncPermission struct {
	BasePermission BasePermission // Permission of the function itself
	Receivers      []Permission   // Permissions of the receiver
	Params         []Permission   // Permissions of the parameters
	Results        []Permission   // Permissions of results
}

// InterfacePermission manages permissions on an interface.
type InterfacePermission struct {
	BasePermission BasePermission // Permission of the interface itself
	Methods        []Permission   // Permission of the methods
}
