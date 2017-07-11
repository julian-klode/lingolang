// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package permission

// This file defines when permissions are considered copyable from one
// value to another.
//
// TODO: Annotate functions with logic rules

// copyableTo is a map to help with recursive types.
//
// TODO: We want a local map, a global one is ugly (or it needs a lock)
var copyableTo = make(map[struct{ A, B Permission }]bool)

// CopyableTo checks that a capability of permission A can be copied to
// a capability with permission B.
//
// Reference types (map, chan, slice, interface) are not copyable. They can
// be refcopied however. This is needed because copying allows permissions to
// get wider: For example, if I have a value "foo", I can copy it to a mutable
// variable "bar", as they are distinct memory locations.
//
// Pointers however, are not reference types, and are copyable if the target
// is refcopyable.
func CopyableTo(A, B Permission) bool {
	// Oh dear, this is our entry point. We need to ensure we can do recursive
	// permissions correctly.
	isCopyable, ok := copyableTo[struct{ A, B Permission }{A, B}]

	if !ok {
		copyableTo[struct{ A, B Permission }{A, B}] = true
		isCopyable = A.isCopyableTo(B)
		copyableTo[struct{ A, B Permission }{A, B}] = isCopyable
	}

	return isCopyable
}

// isCopyableTo for base permission means: always allowed.
func (perm BasePermission) isCopyableTo(p2 Permission) bool {
	_, ok := p2.(BasePermission)
	if !ok {
		return false
	}
	return true
}

// isCopyableTo for pointers means target is refcopyable
func (p *PointerPermission) isCopyableTo(p2 Permission) bool {
	switch p2 := p2.(type) {
	case *PointerPermission:
		return CopyableTo(p.BasePermission, p2.BasePermission) && RefcopyableTo(p.Target, p2.Target)
	default:
		return false
	}
}

// isCopyableTo for channels means false
func (p *ChanPermission) isCopyableTo(p2 Permission) bool {
	return false
}

// isCopyableTo for arrays means recursive
func (p *ArrayPermission) isCopyableTo(p2 Permission) bool {
	switch p2 := p2.(type) {
	case *ArrayPermission:
		return CopyableTo(p.BasePermission, p2.BasePermission) && CopyableTo(p.ElementPermission, p2.ElementPermission)
	default:
		return false
	}
}

// isCopyableTo for slices means false
func (p *SlicePermission) isCopyableTo(p2 Permission) bool {
	return false
}

// isCopyableTo for maps means false
func (p *MapPermission) isCopyableTo(p2 Permission) bool {
	return false
}

// isCopyableTo for structs means recursive.
func (p *StructPermission) isCopyableTo(p2 Permission) bool {
	switch p2 := p2.(type) {
	case *StructPermission:
		if !CopyableTo(p.BasePermission, p2.BasePermission) {
			return false // grr, unreachable
		}

		// TODO: Field length, structural subtyping
		for i := 0; i < len(p.Fields); i++ {
			if !CopyableTo(p.Fields[i], p2.Fields[i]) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// isCopyableTo for func means false
func (p *FuncPermission) isCopyableTo(p2 Permission) bool {
	return false
}

// isCopyableTo for interfaces means movable methods.
func (p *InterfacePermission) isCopyableTo(p2 Permission) bool {
	return false
}
