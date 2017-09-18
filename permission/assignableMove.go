// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package permission

// This file defines when permissions are considered movable from one
// reference to another.
//
// TODO: Annotate functions with logic rules

// MovableTo checks that a capability of permission A can be moved to
// a capability with permission B.
//
// A move is only allowed if the permissions of the target are narrower
// than the permission of the source. Values can be copied, however: A
// copy of a value might have a larger set of permissions - see CopyableTo()
func MovableTo(A, B Permission) bool {
	return movableTo(A, B, make(assignableState))
}
func movableTo(A, B Permission, state assignableState) bool {
	// Oh dear, this is our entry point. We need to ensure we can do recursive
	// permissions correctly.
	key := assignableStateKey{A, B, assignMove}
	isMovable, ok := state[key]

	if !ok {
		state[key] = true
		isMovable = A.isMovableTo(B, state)
		state[key] = isMovable
	}

	return isMovable
}

func (perm BasePermission) isMovableTo(p2 Permission, state assignableState) bool {
	perm2, ok := p2.(BasePermission)
	if !ok {
		return false
	}
	return perm&Read != 0 && perm2&^perm == 0
}

func (p *PointerPermission) isMovableTo(p2 Permission, state assignableState) bool {
	switch p2 := p2.(type) {
	case *PointerPermission:
		return movableTo(p.BasePermission, p2.BasePermission, state) && movableTo(p.Target, p2.Target, state)
	default:
		return false
	}
}

func (p *ChanPermission) isMovableTo(p2 Permission, state assignableState) bool {
	switch p2 := p2.(type) {
	case *ChanPermission:
		return movableTo(p.BasePermission, p2.BasePermission, state) && movableTo(p.ElementPermission, p2.ElementPermission, state)
	default:
		return false
	}
}

func (p *ArrayPermission) isMovableTo(p2 Permission, state assignableState) bool {
	switch p2 := p2.(type) {
	case *ArrayPermission:
		return movableTo(p.BasePermission, p2.BasePermission, state) && movableTo(p.ElementPermission, p2.ElementPermission, state)
	default:
		return false
	}
}

func (p *SlicePermission) isMovableTo(p2 Permission, state assignableState) bool {
	switch p2 := p2.(type) {
	case *SlicePermission:
		return movableTo(p.BasePermission, p2.BasePermission, state) && movableTo(p.ElementPermission, p2.ElementPermission, state)
	default:
		return false
	}
}

func (p *MapPermission) isMovableTo(p2 Permission, state assignableState) bool {
	switch p2 := p2.(type) {
	case *MapPermission:
		return movableTo(p.BasePermission, p2.BasePermission, state) && movableTo(p.KeyPermission, p2.KeyPermission, state) && movableTo(p.ValuePermission, p2.ValuePermission, state)
	default:
		return false
	}
}

func (p *StructPermission) isMovableTo(p2 Permission, state assignableState) bool {
	switch p2 := p2.(type) {
	case *StructPermission:
		if !movableTo(p.BasePermission, p2.BasePermission, state) {
			return false
		}

		// TODO: Field length, structural subtyping
		for i := 0; i < len(p.Fields); i++ {
			if !movableTo(p.Fields[i], p2.Fields[i], state) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func (p *FuncPermission) isMovableTo(p2 Permission, state assignableState) bool {
	switch p2 := p2.(type) {
	case *FuncPermission:
		// Ownership needs to be respected
		if p.BasePermission&Owned == 0 && p2.BasePermission&Owned != 0 {
			return false
		}
		// We cannot assign a mutable function to a value function, but vice
		// versa. This is reversed, because essentially
		if !movableTo(p2.BasePermission&^Owned, p.BasePermission&^Owned, state) {
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

func (p *InterfacePermission) isMovableTo(p2 Permission, state assignableState) bool {
	switch p2 := p2.(type) {
	case *InterfacePermission:
		if !movableTo(p.BasePermission, p2.BasePermission, state) {
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

func (p *WildcardPermission) isMovableTo(p2 Permission, state assignableState) bool {
	return false
}

func (p *TuplePermission) isMovableTo(p2 Permission, state assignableState) bool {
	switch p2 := p2.(type) {
	case *TuplePermission:
		if len(p.Elements) != len(p2.Elements) || !movableTo(p.BasePermission, p2.BasePermission, state) {
			return false
		}
		for i := range p.Elements {
			if !movableTo(p.Elements[i], p2.Elements[i], state) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
