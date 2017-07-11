// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package permission

// This file defines when permissions are considered refcopyable from one
// value to another.
//
// TODO: Annotate functions with logic rules

// refcopyableTo is a map to help with recursive types.
//
// TODO: We want a local map, a global one is ugly (or it needs a lock)
var refcopyableTo = make(map[struct{ A, B Permission }]bool)

// RefcopyableTo checks that a capability of permission A can be referenced
// by another capability whose target permissions are checked.
//
// A refcopy is similar to a move: They only allow narrowing permissions.
// There are two differences: (1) A refcopy cannot have linear operands, and (2)
// an array can be refcopied into a slice, because this allows easily
// implementing slicing checks as a refcopy check.
//
// RefcopyableTo() is used on the target types of pointers: So, "ov" is not
// refcopyable to "ov *ov", but refcopyable to "ov".
func RefcopyableTo(A, B Permission) bool {
	// Oh dear, this is our entry point. We need to ensure we can do recursive
	// permissions correctly.
	isRefcopyable, ok := refcopyableTo[struct{ A, B Permission }{A, B}]

	if !ok {
		refcopyableTo[struct{ A, B Permission }{A, B}] = true
		isRefcopyable = A.isRefcopyableTo(B)
		refcopyableTo[struct{ A, B Permission }{A, B}] = isRefcopyable
	}

	return isRefcopyable
}

// isRefcopyableTo for base permission means: no new permissions and non–linear
func (perm BasePermission) isRefcopyableTo(p2 Permission) bool {
	perm2, ok := p2.(BasePermission)
	if !ok {
		return false
	}
	return perm2&^perm == 0 && !perm.IsLinear() && !p2.IsLinear()
}

// isRefcopyableTo for pointers means recursive
func (p *PointerPermission) isRefcopyableTo(p2 Permission) bool {
	switch p2 := p2.(type) {
	case *PointerPermission:
		return RefcopyableTo(p.BasePermission, p2.BasePermission) && RefcopyableTo(p.Target, p2.Target)
	default:
		return false
	}
}

// isRefcopyableTo for channels means recursive.
func (p *ChanPermission) isRefcopyableTo(p2 Permission) bool {
	switch p2 := p2.(type) {
	case *ChanPermission:
		return RefcopyableTo(p.BasePermission, p2.BasePermission) && RefcopyableTo(p.ElementPermission, p2.ElementPermission)
	default:
		return false
	}
}

// isRefcopyableTo for arrays means recursive to array or slice
func (p *ArrayPermission) isRefcopyableTo(p2 Permission) bool {
	switch p2 := p2.(type) {
	case *ArrayPermission:
		return RefcopyableTo(p.BasePermission, p2.BasePermission) && RefcopyableTo(p.ElementPermission, p2.ElementPermission)
	case *SlicePermission:
		return RefcopyableTo(p.BasePermission, p2.BasePermission) && RefcopyableTo(p.ElementPermission, p2.ElementPermission)
	default:
		return false
	}
}

// isRefcopyableTo for slices means recursive.
func (p *SlicePermission) isRefcopyableTo(p2 Permission) bool {
	switch p2 := p2.(type) {
	case *SlicePermission:
		return RefcopyableTo(p.BasePermission, p2.BasePermission) && RefcopyableTo(p.ElementPermission, p2.ElementPermission)
	default:
		return false
	}
}

// isRefcopyableTo for maps means recursive.
func (p *MapPermission) isRefcopyableTo(p2 Permission) bool {
	switch p2 := p2.(type) {
	case *MapPermission:
		return RefcopyableTo(p.BasePermission, p2.BasePermission) && RefcopyableTo(p.KeyPermission, p2.KeyPermission) && RefcopyableTo(p.ValuePermission, p2.ValuePermission)
	default:
		return false
	}
}

// isRefcopyableTo for structs means recursive.
func (p *StructPermission) isRefcopyableTo(p2 Permission) bool {
	switch p2 := p2.(type) {
	case *StructPermission:
		if !RefcopyableTo(p.BasePermission, p2.BasePermission) {
			return false
		}

		// TODO: Field length, structural subtyping
		for i := 0; i < len(p.Fields); i++ {
			if !RefcopyableTo(p.Fields[i], p2.Fields[i]) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// isRefcopyableTo for func means receivers, params, results movable.
func (p *FuncPermission) isRefcopyableTo(p2 Permission) bool {
	switch p2 := p2.(type) {
	case *FuncPermission:
		// Ownership needs to be respected
		if !RefcopyableTo(p.BasePermission&Owned, p2.BasePermission&Owned) {
			return false
		}
		// We cannot assign a mutable function to a value function, but vice
		// versa. This is reversed, because essentially
		if !RefcopyableTo(p2.BasePermission&^Owned, p.BasePermission&^Owned) {
			return false
		}
		// Receivers and parameters are contravariant: If f2 takes argument
		// of a permission p2, and we want to assign f1 with permission p1,
		// then that permission may be more narrow.
		for i := 0; i < len(p.Receivers); i++ {
			if !MovableTo(p2.Receivers[i], p.Receivers[i]) {
				return false
			}
		}
		for i := 0; i < len(p.Params); i++ {
			if !MovableTo(p2.Params[i], p.Params[i]) {
				return false
			}
		}
		// Results are covariant
		for i := 0; i < len(p.Results); i++ {
			if !MovableTo(p.Results[i], p2.Results[i]) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// isRefcopyableTo for interfaces means movable methods.
func (p *InterfacePermission) isRefcopyableTo(p2 Permission) bool {
	switch p2 := p2.(type) {
	case *InterfacePermission:
		if !RefcopyableTo(p.BasePermission, p2.BasePermission) {
			return false
		}

		// TODO: Field length, structural subtyping
		for i := 0; i < len(p.Methods); i++ {
			if !MovableTo(p.Methods[i], p2.Methods[i]) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
