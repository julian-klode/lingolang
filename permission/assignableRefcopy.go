// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package permission

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
	return refcopyableTo(A, B, make(assignableState))
}

func refcopyableTo(A, B Permission, state assignableState) bool {
	// Oh dear, this is our entry point. We need to ensure we can do recursive
	// permissions correctly.
	key := assignableStateKey{A, B, assignReference}
	isRefcopyable, ok := state[key]

	if !ok {
		state[key] = true
		isRefcopyable = A.isRefcopyableTo(B, state)
		state[key] = isRefcopyable
	}

	return isRefcopyable
}

// isRefcopyableTo for base permission means: no new permissions and non–linear
func (perm BasePermission) isRefcopyableTo(p2 Permission, state assignableState) bool {
	perm2, ok := p2.(BasePermission)
	if !ok {
		return false
	}
	return perm2&^perm == 0 && !perm.isLinear() && !perm2.isLinear()
}

// isRefcopyableTo for pointers means recursive
func (p *PointerPermission) isRefcopyableTo(p2 Permission, state assignableState) bool {
	switch p2 := p2.(type) {
	case *PointerPermission:
		return refcopyableTo(p.BasePermission, p2.BasePermission, state) && refcopyableTo(p.Target, p2.Target, state)
	default:
		return false
	}
}

// isRefcopyableTo for channels means recursive.
func (p *ChanPermission) isRefcopyableTo(p2 Permission, state assignableState) bool {
	switch p2 := p2.(type) {
	case *ChanPermission:
		return refcopyableTo(p.BasePermission, p2.BasePermission, state) && refcopyableTo(p.ElementPermission, p2.ElementPermission, state)
	default:
		return false
	}
}

// isRefcopyableTo for arrays means recursive to array or slice
func (p *ArrayPermission) isRefcopyableTo(p2 Permission, state assignableState) bool {
	switch p2 := p2.(type) {
	case *ArrayPermission:
		return refcopyableTo(p.BasePermission, p2.BasePermission, state) && refcopyableTo(p.ElementPermission, p2.ElementPermission, state)
	case *SlicePermission:
		return refcopyableTo(p.BasePermission, p2.BasePermission, state) && refcopyableTo(p.ElementPermission, p2.ElementPermission, state)
	default:
		return false
	}
}

// isRefcopyableTo for slices means recursive.
func (p *SlicePermission) isRefcopyableTo(p2 Permission, state assignableState) bool {
	switch p2 := p2.(type) {
	case *SlicePermission:
		return refcopyableTo(p.BasePermission, p2.BasePermission, state) && refcopyableTo(p.ElementPermission, p2.ElementPermission, state)
	default:
		return false
	}
}

// isRefcopyableTo for maps means recursive.
func (p *MapPermission) isRefcopyableTo(p2 Permission, state assignableState) bool {
	switch p2 := p2.(type) {
	case *MapPermission:
		return refcopyableTo(p.BasePermission, p2.BasePermission, state) && refcopyableTo(p.KeyPermission, p2.KeyPermission, state) && refcopyableTo(p.ValuePermission, p2.ValuePermission, state)
	default:
		return false
	}
}

// isRefcopyableTo for structs means recursive.
func (p *StructPermission) isRefcopyableTo(p2 Permission, state assignableState) bool {
	switch p2 := p2.(type) {
	case *StructPermission:
		if !refcopyableTo(p.BasePermission, p2.BasePermission, state) {
			return false
		}

		// TODO: Field length, structural subtyping
		for i := 0; i < len(p.Fields); i++ {
			if !refcopyableTo(p.Fields[i], p2.Fields[i], state) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// isRefcopyableTo for func means receivers, params, results movable.
func (p *FuncPermission) isRefcopyableTo(p2 Permission, state assignableState) bool {
	switch p2 := p2.(type) {
	case *FuncPermission:
		// Ownership needs to be respected
		if !refcopyableTo(p.BasePermission&Owned, p2.BasePermission&Owned, state) {
			return false
		}
		// We cannot assign a mutable function to a value function, but vice
		// versa. This is reversed, because essentially
		if !refcopyableTo(p2.BasePermission&^Owned, p.BasePermission&^Owned, state) {
			return false
		}
		// Receivers and parameters are contravariant: If f2 takes argument
		// of a permission p2, and we want to assign f1 with permission p1,
		// then that permission may be more narrow.
		for i := 0; i < len(p.Receivers); i++ {
			if !movableTo(p2.Receivers[i], p.Receivers[i], state) {
				return false
			}
		}
		for i := 0; i < len(p.Params); i++ {
			if !movableTo(p2.Params[i], p.Params[i], state) {
				return false
			}
		}
		// Results are covariant
		for i := 0; i < len(p.Results); i++ {
			if !movableTo(p.Results[i], p2.Results[i], state) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// isRefcopyableTo for interfaces means movable methods.
func (p *InterfacePermission) isRefcopyableTo(p2 Permission, state assignableState) bool {
	switch p2 := p2.(type) {
	case *InterfacePermission:
		if !refcopyableTo(p.BasePermission, p2.BasePermission, state) {
			return false
		}

		// TODO: Field length, structural subtyping
		for i := 0; i < len(p.Methods); i++ {
			if !movableTo(p.Methods[i], p2.Methods[i], state) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (p *WildcardPermission) isRefcopyableTo(p2 Permission, state assignableState) bool {
	return false
}

func (p *TuplePermission) isRefcopyableTo(p2 Permission, state assignableState) bool {
	switch p2 := p2.(type) {
	case *TuplePermission:
		if len(p.Elements) != len(p2.Elements) || !refcopyableTo(p.BasePermission, p2.BasePermission, state) {
			return false
		}
		for i := range p.Elements {
			if !refcopyableTo(p.Elements[i], p2.Elements[i], state) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
