// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package permission

type assignableMode int

const (
	assignMove      assignableMode = 0
	assignCopy      assignableMode = 1
	assignReference assignableMode = 2
)

type assignableStateKey struct {
	A, B Permission
	mode assignableMode
}
type assignableState map[assignableStateKey]bool

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
	return copyableTo(A, B, make(assignableState))
}

func copyableTo(A, B Permission, state assignableState) bool {
	// Oh dear, this is our entry point. We need to ensure we can do recursive
	// permissions correctly.
	key := assignableStateKey{A, B, assignCopy}
	isCopyable, ok := state[key]

	if !ok {
		state[key] = true
		isCopyable = A.isCopyableTo(B, state)
		state[key] = isCopyable
	}

	return isCopyable
}

// isCopyableTo for base permission means: always allowed.
func (perm BasePermission) isCopyableTo(p2 Permission, state assignableState) bool {
	_, ok := p2.(BasePermission)
	if !ok {
		return false
	}
	return true
}

// isCopyableTo for pointers means target is refcopyable
func (p *PointerPermission) isCopyableTo(p2 Permission, state assignableState) bool {
	switch p2 := p2.(type) {
	case *PointerPermission:
		return copyableTo(p.BasePermission, p2.BasePermission, state) && refcopyableTo(p.Target, p2.Target, state)
	default:
		return false
	}
}

// isCopyableTo for channels means false
func (p *ChanPermission) isCopyableTo(p2 Permission, state assignableState) bool {
	return refcopyableTo(p, p2, state)
}

// isCopyableTo for arrays means recursive
func (p *ArrayPermission) isCopyableTo(p2 Permission, state assignableState) bool {
	switch p2 := p2.(type) {
	case *ArrayPermission:
		return copyableTo(p.BasePermission, p2.BasePermission, state) && copyableTo(p.ElementPermission, p2.ElementPermission, state)
	default:
		return false
	}
}

// isCopyableTo for slices means false
func (p *SlicePermission) isCopyableTo(p2 Permission, state assignableState) bool {
	return refcopyableTo(p, p2, state)
}

// isCopyableTo for maps means false
func (p *MapPermission) isCopyableTo(p2 Permission, state assignableState) bool {
	return refcopyableTo(p, p2, state)
}

// isCopyableTo for structs means recursive.
func (p *StructPermission) isCopyableTo(p2 Permission, state assignableState) bool {
	switch p2 := p2.(type) {
	case *StructPermission:
		if !copyableTo(p.BasePermission, p2.BasePermission, state) {
			return false // grr, unreachable
		}

		// TODO: Field length, structural subtyping
		for i := 0; i < len(p.Fields); i++ {
			if !copyableTo(p.Fields[i], p2.Fields[i], state) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// isCopyableTo for func means false
func (p *FuncPermission) isCopyableTo(p2 Permission, state assignableState) bool {
	return refcopyableTo(p, p2, state)
}

// isCopyableTo for interfaces means movable methods.
func (p *InterfacePermission) isCopyableTo(p2 Permission, state assignableState) bool {
	return refcopyableTo(p, p2, state)
}
