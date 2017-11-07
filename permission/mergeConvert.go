// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

// Compatible permissions
//
// This file defines how to make permissions compatible with each other.

package permission

type convertToBaseState mergeState

// register registers a new result for a given permission and goal.
func (state *convertToBaseState) register(result, perm, goal Permission) {
	state.state[mergeStateKey{perm, goal, state.action}] = result
}

func convertToBase(perm Permission, goal BasePermission, state *convertToBaseState) Permission {
	key := mergeStateKey{perm, goal, state.action}
	result, ok := state.state[key]
	if !ok {
		result = perm.convertToBase(goal, state)
	}
	return result
}

func (perm BasePermission) convertToBaseBase(perm2 BasePermission) BasePermission {
	return perm2
}

func (perm BasePermission) convertToBase(p2 BasePermission, state *convertToBaseState) Permission {
	return perm.convertToBaseBase(p2)
}

func (p *PointerPermission) convertToBase(p2 BasePermission, state *convertToBaseState) Permission {
	next := &PointerPermission{}
	state.register(next, p, p2)

	next.BasePermission = p.BasePermission.convertToBaseBase(p2)
	switch state.action {
	case mergeConversion:
		// If we loose linearity of the pointer, the target we are pointing
		// to is not linear anymore either. For writes, we drop the write
		// permission.
		nextTarget := p.Target.GetBasePermission()&^Owned | (next.BasePermission & Owned)
		// Strip linear write rights.
		if (next.BasePermission&ExclRead) == 0 && (nextTarget&(ExclWrite|Write)) == (ExclWrite|Write) {
			nextTarget &^= Write | ExclWrite
		}
		// Strip linearity from linear read rights
		if (next.BasePermission&ExclRead) == 0 && (nextTarget&(ExclRead|Read)) == (ExclRead|Read) {
			nextTarget &^= ExclRead
		}
		next.Target = convertToBase(p.Target, nextTarget, state)
	case mergeStrictConversion:
		next.Target = convertToBase(p.Target, next.BasePermission, state)
	}

	return next
}

func (p *ChanPermission) convertToBase(p2 BasePermission, state *convertToBaseState) Permission {
	next := &ChanPermission{}
	state.register(next, p, p2)
	next.BasePermission = p.BasePermission.convertToBaseBase(p2)
	next.ElementPermission = convertToBase(p.ElementPermission, next.BasePermission, state)

	return next
}

func (p *ArrayPermission) convertToBase(p2 BasePermission, state *convertToBaseState) Permission {
	next := &ArrayPermission{}
	state.register(next, p, p2)

	next.BasePermission = p.BasePermission.convertToBaseBase(p2)
	next.ElementPermission = convertToBase(p.ElementPermission, next.BasePermission, state)

	return next
}

func (p *SlicePermission) convertToBase(p2 BasePermission, state *convertToBaseState) Permission {
	next := &SlicePermission{}
	state.register(next, p, p2)

	next.BasePermission = p.BasePermission.convertToBaseBase(p2)
	next.ElementPermission = convertToBase(p.ElementPermission, next.BasePermission, state)

	return next
}

func (p *MapPermission) convertToBase(p2 BasePermission, state *convertToBaseState) Permission {
	next := &MapPermission{}
	state.register(next, p, p2)

	next.BasePermission = p.BasePermission.convertToBaseBase(p2)
	next.KeyPermission = convertToBase(p.KeyPermission, next.BasePermission, state)
	next.ValuePermission = convertToBase(p.ValuePermission, next.BasePermission, state)

	return next
}

func (p *StructPermission) convertToBase(p2 BasePermission, state *convertToBaseState) Permission {
	next := &StructPermission{}
	state.register(next, p, p2)

	next.BasePermission = p.BasePermission.convertToBaseBase(p2)
	next.Fields = make([]Permission, len(p.Fields))
	for i := 0; i < len(p.Fields); i++ {
		next.Fields[i] = convertToBase(p.Fields[i], next.BasePermission, state)
	}

	return next
}

func (p *FuncPermission) convertToBase(p2 BasePermission, state *convertToBaseState) Permission {
	next := &FuncPermission{}
	state.register(next, p, p2)

	next.BasePermission = p.BasePermission.convertToBaseBase(p2)
	next.Name = p.Name
	if p.Receivers != nil {
		next.Receivers = make([]Permission, len(p.Receivers))
		for i := 0; i < len(p.Receivers); i++ {
			next.Receivers[i] = convertToBase(p.Receivers[i], p.Receivers[i].GetBasePermission(), state)
		}
	}
	if p.Params != nil {
		next.Params = make([]Permission, len(p.Params))
		for i := 0; i < len(p.Params); i++ {
			next.Params[i] = convertToBase(p.Params[i], p.Params[i].GetBasePermission(), state)
		}
	}
	if p.Results != nil {
		next.Results = make([]Permission, len(p.Results))
		for i := 0; i < len(p.Results); i++ {
			next.Results[i] = convertToBase(p.Results[i], p.Results[i].GetBasePermission(), state)
		}
	}

	return next
}

func (p *InterfacePermission) convertToBase(p2 BasePermission, state *convertToBaseState) Permission {
	next := &InterfacePermission{}
	state.register(next, p, p2)

	next.BasePermission = p.BasePermission.convertToBaseBase(p2)
	if p.Methods != nil {
		next.Methods = make([]*FuncPermission, len(p.Methods))
		for i := 0; i < len(p.Methods); i++ {
			next.Methods[i] = convertToBase(p.Methods[i], p.Methods[i].GetBasePermission(), state).(*FuncPermission)
		}
	}

	return next
}

func (p *WildcardPermission) convertToBase(p2 BasePermission, state *convertToBaseState) Permission {
	return p2
}

func (p *TuplePermission) convertToBase(p2 BasePermission, state *convertToBaseState) Permission {
	next := &TuplePermission{}
	state.register(next, p, p2)

	next.BasePermission = p.BasePermission.convertToBaseBase(p2)
	if p.Elements != nil {
		next.Elements = make([]Permission, len(p.Elements))
		for i := 0; i < len(p.Elements); i++ {
			next.Elements[i] = convertToBase(p.Elements[i], next.BasePermission, state)
		}
	}

	return next
}

func (p *NilPermission) convertToBase(p2 BasePermission, state *convertToBaseState) Permission {
	return p
}
