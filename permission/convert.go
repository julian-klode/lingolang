// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

// Compatible permissions
//
// This file defines how to make permissions compatible with each other.

package permission

type convertState mergeState

// register registers a new result for a given permission and goal.
func (state *convertState) register(result, perm, goal Permission) {
	state.state[mergeStateKey{perm, goal, mergeConversion}] = result
}

func convertTo(perm Permission, goal BasePermission, state *convertState) Permission {
	key := mergeStateKey{perm, goal, mergeConversion}
	result, ok := state.state[key]
	if !ok {
		result = perm.convertTo(goal, state)
	}
	return result
}

func (perm BasePermission) convertToBase(perm2 BasePermission) BasePermission {
	return perm2
}

func (perm BasePermission) convertTo(p2 BasePermission, state *convertState) Permission {
	return perm.convertToBase(p2)
}

func (p *PointerPermission) convertTo(p2 BasePermission, state *convertState) Permission {
	next := &PointerPermission{}
	state.register(next, p, p2)

	next.BasePermission = p.BasePermission.convertToBase(p2)
	// If we loose linearity of the pointer, the target we are pointing
	// to is not linear anymore either. For writes, we drop the write
	// permission.
	nextTarget := p.Target.GetBasePermission()
	// Strip linear write rights.
	if (next.BasePermission&ExclRead) == 0 && (nextTarget&(ExclWrite|Write)) == (ExclWrite|Write) {
		nextTarget &^= Write | ExclWrite
	}
	// Strip linearity from linear read rights
	if (next.BasePermission&ExclRead) == 0 && (nextTarget&(ExclRead|Read)) == (ExclRead|Read) {
		nextTarget &^= ExclRead
	}
	next.Target = convertTo(p.Target, nextTarget, state)

	return next
}

func (p *ChanPermission) convertTo(p2 BasePermission, state *convertState) Permission {
	next := &ChanPermission{}
	state.register(next, p, p2)
	next.BasePermission = p.BasePermission.convertToBase(p2)
	next.ElementPermission = convertTo(p.ElementPermission, next.BasePermission, state)

	return next
}

func (p *ArrayPermission) convertTo(p2 BasePermission, state *convertState) Permission {
	next := &ArrayPermission{}
	state.register(next, p, p2)

	next.BasePermission = p.BasePermission.convertToBase(p2)
	next.ElementPermission = convertTo(p.ElementPermission, next.BasePermission, state)

	return next
}

func (p *SlicePermission) convertTo(p2 BasePermission, state *convertState) Permission {
	next := &SlicePermission{}
	state.register(next, p, p2)

	next.BasePermission = p.BasePermission.convertToBase(p2)
	next.ElementPermission = convertTo(p.ElementPermission, next.BasePermission, state)

	return next
}

func (p *MapPermission) convertTo(p2 BasePermission, state *convertState) Permission {
	next := &MapPermission{}
	state.register(next, p, p2)

	next.BasePermission = p.BasePermission.convertToBase(p2)
	next.KeyPermission = convertTo(p.KeyPermission, next.BasePermission, state)
	next.ValuePermission = convertTo(p.ValuePermission, next.BasePermission, state)

	return next
}

func (p *StructPermission) convertTo(p2 BasePermission, state *convertState) Permission {
	next := &StructPermission{}
	state.register(next, p, p2)

	next.BasePermission = p.BasePermission.convertToBase(p2)
	next.Fields = make([]Permission, len(p.Fields))
	for i := 0; i < len(p.Fields); i++ {
		next.Fields[i] = convertTo(p.Fields[i], next.BasePermission, state)
	}

	return next
}

func (p *FuncPermission) convertTo(p2 BasePermission, state *convertState) Permission {
	next := &FuncPermission{}
	state.register(next, p, p2)

	next.BasePermission = p.BasePermission.convertToBase(p2)
	if p.Receivers != nil {
		next.Receivers = make([]Permission, len(p.Receivers))
		for i := 0; i < len(p.Receivers); i++ {
			next.Receivers[i] = convertTo(p.Receivers[i], p.Receivers[i].GetBasePermission(), state)
		}
	}
	if p.Params != nil {
		next.Params = make([]Permission, len(p.Params))
		for i := 0; i < len(p.Params); i++ {
			next.Params[i] = convertTo(p.Params[i], p.Params[i].GetBasePermission(), state)
		}
	}
	if p.Results != nil {
		next.Results = make([]Permission, len(p.Results))
		for i := 0; i < len(p.Results); i++ {
			next.Results[i] = convertTo(p.Results[i], p.Results[i].GetBasePermission(), state)
		}
	}

	return next
}

func (p *InterfacePermission) convertTo(p2 BasePermission, state *convertState) Permission {
	next := &InterfacePermission{}
	state.register(next, p, p2)

	next.BasePermission = p.BasePermission.convertToBase(p2)
	if p.Methods != nil {
		next.Methods = make([]Permission, len(p.Methods))
		for i := 0; i < len(p.Methods); i++ {
			next.Methods[i] = convertTo(p.Methods[i], next.BasePermission, state)
		}
	}

	return next
}
