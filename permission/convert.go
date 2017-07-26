// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

// Compatible permissions
//
// This file defines how to make permissions compatible with each other.

package permission

import (
	"fmt"
	"reflect"
)

// convertError is a bailout error type to use with panic() and recover()
// below, so the code does not get crazy long due to ifs.
type convertErrorT struct{ error }

func convertError(e error) convertErrorT {
	return convertErrorT{e}
}

// convertState is a map that stores the temporary results of making
// a permission compatible to another, so we can handle recursive data
// structures.
type convertState map[convertStateKey]Permission
type convertStateKey struct {
	perm Permission
	goal Permission
}

// register registers a new result for a given permission and goal.
func (state convertState) register(result, perm, goal Permission) {
	state[convertStateKey{perm, goal}] = result
}

// ConvertTo takes a permission and makes it compatible with the goal
// permission. It's use is to turn incomplete annotations into permissions
// matching the type. For example, if there is a list with a next pointer:
// it could be annotated "om struct { om }". That is incomplete: The inner
// "om" refers to a list as well, so the permission needs to be recursive.
// By taking the type permission, which is "p = om struct { p }", that is
// a recursive permission refering to itself, and converting that to the
// target, we can make the permission complete.
//
// goal can be either a base permission, or, alternatively, a permission of
// the same shape as perm. In the latter case, goal must be a consistent
// permission: It must be made compatible to its base permission, otherwise
// the result of this function causes undefined behavior.
func ConvertTo(perm Permission, goal Permission) (result Permission, err error) {
	defer func() {
		val := recover()
		if val != nil {
			switch e := val.(type) {
			case convertErrorT:
				err = e
			default:
				panic(e)
			}
		}
	}()
	result = convertTo(perm, goal, make(convertState))
	if reflect.DeepEqual(result, perm) {
		result = perm
	}
	return
}

// MakeCompatibleTo takes a permission and makes it compatible with the outer
// permission.
func convertTo(perm Permission, goal Permission, state convertState) Permission {
	key := convertStateKey{perm, goal}
	result, ok := state[key]
	if !ok {
		result = perm.convertTo(goal, state)
		if result == nil {
			panic(convertError(fmt.Errorf("Cannot make %v compatible to %v", perm, goal)))
		}

		state[key] = result
	}
	return result
}

func (perm BasePermission) convertToBase(perm2 BasePermission) BasePermission {
	return perm2
}

func (perm BasePermission) convertTo(p2 Permission, state convertState) Permission {
	switch p2 := p2.(type) {
	case BasePermission:
		return perm.convertToBase(p2)
	default:
		return nil
	}
}

func (p *PointerPermission) convertTo(p2 Permission, state convertState) Permission {
	next := &PointerPermission{}
	state.register(next, p, p2)
	switch p2 := p2.(type) {
	case BasePermission:
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
	case *PointerPermission:
		next.BasePermission = p.BasePermission.convertToBase(p2.BasePermission)
		next.Target = convertTo(p.Target, p2.Target, state)
	default:
		return nil
	}

	return next
}

func (p *ChanPermission) convertTo(p2 Permission, state convertState) Permission {
	next := &ChanPermission{}
	state.register(next, p, p2)
	switch p2 := p2.(type) {
	case BasePermission:
		next.BasePermission = p.BasePermission.convertToBase(p2)
		next.ElementPermission = convertTo(p.ElementPermission, next.BasePermission, state)
	case *ChanPermission:
		next.BasePermission = p.BasePermission.convertToBase(p2.BasePermission)
		next.ElementPermission = convertTo(p.ElementPermission, p2.ElementPermission, state)
	default:
		return nil
	}

	return next
}

func (p *ArrayPermission) convertTo(p2 Permission, state convertState) Permission {
	next := &ArrayPermission{}
	state.register(next, p, p2)
	switch p2 := p2.(type) {
	case BasePermission:
		next.BasePermission = p.BasePermission.convertToBase(p2)
		next.ElementPermission = convertTo(p.ElementPermission, next.BasePermission, state)
	case *ArrayPermission:
		next.BasePermission = p.BasePermission.convertToBase(p2.BasePermission)
		next.ElementPermission = convertTo(p.ElementPermission, p2.ElementPermission, state)
	default:
		return nil
	}

	return next
}

func (p *SlicePermission) convertTo(p2 Permission, state convertState) Permission {
	next := &SlicePermission{}
	state.register(next, p, p2)
	switch p2 := p2.(type) {
	case BasePermission:
		next.BasePermission = p.BasePermission.convertToBase(p2)
		next.ElementPermission = convertTo(p.ElementPermission, next.BasePermission, state)
	case *SlicePermission:
		next.BasePermission = p.BasePermission.convertToBase(p2.BasePermission)
		next.ElementPermission = convertTo(p.ElementPermission, p2.ElementPermission, state)
	default:
		return nil
	}

	return next
}

func (p *MapPermission) convertTo(p2 Permission, state convertState) Permission {
	next := &MapPermission{}
	state.register(next, p, p2)
	switch p2 := p2.(type) {
	case BasePermission:
		next.BasePermission = p.BasePermission.convertToBase(p2)
		next.KeyPermission = convertTo(p.KeyPermission, next.BasePermission, state)
		next.ValuePermission = convertTo(p.ValuePermission, next.BasePermission, state)
	case *MapPermission:
		next.BasePermission = p.BasePermission.convertToBase(p2.BasePermission)
		next.KeyPermission = convertTo(p.KeyPermission, p2.KeyPermission, state)
		next.ValuePermission = convertTo(p.ValuePermission, p2.ValuePermission, state)
	default:
		return nil
	}

	return next
}

func (p *StructPermission) convertTo(p2 Permission, state convertState) Permission {
	next := &StructPermission{}
	state.register(next, p, p2)
	switch p2 := p2.(type) {
	case BasePermission:
		next.BasePermission = p.BasePermission.convertToBase(p2)
		next.Fields = make([]Permission, len(p.Fields))
		for i := 0; i < len(p.Fields); i++ {
			next.Fields[i] = convertTo(p.Fields[i], next.BasePermission, state)
		}
	case *StructPermission:
		next.BasePermission = p.BasePermission.convertToBase(p2.BasePermission)
		next.Fields = make([]Permission, len(p.Fields))
		if len(p.Fields) != len(p2.Fields) {
			panic(convertError(fmt.Errorf("Cannot make %v compatible to %v: Different number of fields: %d vs %d", p, p2, len(p.Fields), len(p2.Fields))))
		}
		for i := 0; i < len(p.Fields); i++ {
			next.Fields[i] = convertTo(p.Fields[i], p2.Fields[i], state)
		}
	default:
		return nil
	}

	return next
}

func (p *FuncPermission) convertTo(p2 Permission, state convertState) Permission {
	next := &FuncPermission{}
	state.register(next, p, p2)
	switch p2 := p2.(type) {
	case BasePermission:
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
	case *FuncPermission:
		next.BasePermission = p.BasePermission.convertToBase(p2.GetBasePermission())
		if len(p.Receivers) != len(p2.Receivers) {
			panic(convertError(fmt.Errorf("Cannot convert %v to %v: Different number of receivers", p, p2)))
		}
		if p.Receivers != nil {
			next.Receivers = make([]Permission, len(p.Receivers))
			for i := 0; i < len(p.Receivers); i++ {
				next.Receivers[i] = convertTo(p.Receivers[i], p2.Receivers[i], state)
			}
		}
		if len(p.Params) != len(p2.Params) {
			panic(convertError(fmt.Errorf("Cannot convert %v to %v: Different number of parameters", p, p2)))
		}
		if p.Params != nil {
			next.Params = make([]Permission, len(p.Params))
			for i := 0; i < len(p.Params); i++ {
				next.Params[i] = convertTo(p.Params[i], p2.Params[i], state)
			}
		}
		if len(p.Results) != len(p2.Results) {
			panic(convertError(fmt.Errorf("Cannot convert %v to %v: Different number of results", p, p2)))
		}
		if p.Results != nil {
			next.Results = make([]Permission, len(p.Results))
			for i := 0; i < len(p.Results); i++ {
				next.Results[i] = convertTo(p.Results[i], p2.Results[i], state)
			}
		}
	default:
		return nil
	}

	return next
}

func (p *InterfacePermission) convertTo(p2 Permission, state convertState) Permission {
	next := &InterfacePermission{}
	state.register(next, p, p2)
	switch p2 := p2.(type) {
	case BasePermission:
		next.BasePermission = p.BasePermission.convertToBase(p2)
		if p.Methods != nil {
			next.Methods = make([]Permission, len(p.Methods))
			for i := 0; i < len(p.Methods); i++ {
				next.Methods[i] = convertTo(p.Methods[i], next.BasePermission, state)
			}
		}
	case *InterfacePermission:
		next.BasePermission = p.BasePermission.convertToBase(p2.BasePermission)
		if len(p.Methods) != len(p2.Methods) {
			panic(convertError(fmt.Errorf("Cannot convert %v to %v: Different number of methods", p, p2)))
		}
		if p.Methods != nil {
			next.Methods = make([]Permission, len(p.Methods))
			for i := 0; i < len(p.Methods); i++ {
				next.Methods[i] = convertTo(p.Methods[i], p2.Methods[i], state)
			}
		}
	default:
		return nil
	}
	return next
}
