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

// IsLinear checks if the type is linear
func (perm BasePermission) IsLinear() bool {
	return (perm&(ExclWrite) != 0 && perm&Write != 0) || (perm&(ExclRead) != 0 && perm&Read != 0)
}

// Permission is an entity associated with an value that describes in which
// ways the value can be used.
type Permission interface {
	isMovableTo(p2 Permission) bool
	isRefcopyableTo(p2 Permission) bool
	isCopyableTo(p2 Permission) bool
	// IsLinear checks if the type is linear
	IsLinear() bool
}

// PointerPermission describes permissions on a pointer value.
type PointerPermission struct {
	BasePermission BasePermission // The permission on the pointer value itself
	Target         Permission     // The permission of the value we are pointing to
}

// IsLinear checks if the type is linear
func (perm PointerPermission) IsLinear() bool {
	return perm.BasePermission.IsLinear()
}

// ChanPermission describes permissions on channels and their elements.
type ChanPermission struct {
	BasePermission    BasePermission // The permission on the chan value itself
	ElementPermission Permission     // The permission of the elements it contains
}

// IsLinear checks if the type is linear
func (perm ChanPermission) IsLinear() bool {
	return perm.BasePermission.IsLinear()
}

// ArrayPermission describes permissions on arrays
type ArrayPermission struct {
	BasePermission    BasePermission // The permission on the array/slice value itself
	ElementPermission Permission     // The permission of the elements it contains
}

// IsLinear checks if the type is linear
func (perm ArrayPermission) IsLinear() bool {
	return perm.BasePermission.IsLinear()
}

// SlicePermission describes permissions on slices
type SlicePermission struct {
	BasePermission    BasePermission // The permission on the array/slice value itself
	ElementPermission Permission     // The permission of the elements it contains
}

// IsLinear checks if the type is linear
func (perm SlicePermission) IsLinear() bool {
	return perm.BasePermission.IsLinear()
}

// MapPermission describes permissions on map values, their keys and values.
type MapPermission struct {
	BasePermission  BasePermission // The permission of the map itself
	KeyPermission   Permission     // The permission of contained keys
	ValuePermission Permission     // The permission of contained values
}

// IsLinear checks if the type is linear
func (perm MapPermission) IsLinear() bool {
	return perm.BasePermission.IsLinear()
}

// StructPermission describes permissions of structs.
type StructPermission struct {
	BasePermission BasePermission // Permission of the struct itself
	Fields         []Permission   // Permissions of the fields, in order
}

// IsLinear checks if the type is linear
func (perm StructPermission) IsLinear() bool {
	return perm.BasePermission.IsLinear()
}

// FuncPermission describes permissions of functions
//
// TODO: We need to make sure to encode a function that can return different
// values for invocations with the same input, or methods that bind linear
// variables - we cannot have two references to those (hence, mark them as
// with the same exclusive bits as bound variables)
// TODO: If we use excl. to mark bound excl. vars, can we "freeze" the func?
type FuncPermission struct {
	BasePermission BasePermission // Permission of the function itself
	Receivers      []Permission   // Permissions of the receiver
	Params         []Permission   // Permissions of the parameters
	Results        []Permission   // Permissions of results
}

// IsLinear checks if the type is linear
func (perm FuncPermission) IsLinear() bool {
	return perm.BasePermission.IsLinear()
}

// InterfacePermission manages permissions on an interface.
type InterfacePermission struct {
	BasePermission BasePermission // Permission of the interface itself
	Methods        []Permission   // Permission of the methods
}

// IsLinear checks if the type is linear
func (perm InterfacePermission) IsLinear() bool {
	return perm.BasePermission.IsLinear()
}
