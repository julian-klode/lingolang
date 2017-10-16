// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package permission

import (
	"fmt"
	"reflect"
)

// mergeError is a bailout error type to use with panic() and recover()
// below, so the code does not get crazy long due to ifs.
type mergeErrorT struct{ error }

func mergeError(e error) mergeErrorT {
	return mergeErrorT{e}
}

// What are we doing?
type mergeAction int

const (
	mergeConversion mergeAction = iota
	mergeStrictConversion
	mergeIntersection
	mergeUnion
)

// *mergeState is a map that stores the temporary results of making
// a permimergeConvertssion compatible to another, so we can handle recursive data
// structures.
type mergeState struct {
	state  map[mergeStateKey]Permission
	action mergeAction
}

type mergeStateKey struct {
	perm   Permission
	goal   Permission
	action mergeAction
}

// register registers a new result for a given permission and goal.
func (state *mergeState) register(result, perm, goal Permission) {
	state.state[mergeStateKey{perm, goal, state.action}] = result
}

// return a new state that toggles union/intersect
func (state *mergeState) contravariant() *mergeState {
	switch state.action {
	case mergeIntersection:
		return &mergeState{state.state, mergeUnion}
	case mergeUnion:
		return &mergeState{state.state, mergeIntersection}
	default:
		return state
	}
}

func (state *mergeState) mergeBase(p1, p2 BasePermission) BasePermission {
	switch state.action {
	case mergeConversion, mergeStrictConversion:
		return p2
	case mergeIntersection:
		return p1 & p2
	case mergeUnion:
		return p1 | p2
	}
	panic(fmt.Errorf("Invalid merge action %d", state.action))
}

func mergeRecover(err *error) {
	val := recover()
	if val != nil {
		switch e := val.(type) {
		case mergeErrorT:
			*err = e
		default:
			panic(e)
		}
	}
}

// ConvertTo takes a permission and makes it compatible with the goal
// permission. It's use is to turn incomplete annotations into permissions
// matching the type. For example, if there is a list with a next pointer:
// it could be annotated "om struct { om }". That is incomplete: The inner
// "om" refers to a list as well, so the permission needs to be recursive.
//
// By taking the type permission, which is "p = om struct { p }", that is
// a recursive permission refering to itself, and converting that to the		-
// target, we can make the permission complete.
//
// goal can be either a base permission, or, alternatively, a permission of
// the same shape as perm. In the latter case, goal must be a consistent
// permission: It must be made compatible to its base permission, otherwise
// the result of this function causes undefined behavior.
func ConvertTo(perm Permission, goal Permission) (result Permission, err error) {
	defer mergeRecover(&err)
	result = merge(perm, goal, &mergeState{make(map[mergeStateKey]Permission), mergeConversion})
	if reflect.DeepEqual(result, perm) {
		result = perm
	}
	return
}

// StrictConvertToBase makes sure that the actual permissions of the object
// only depend on the goal - all base permissions are replaced by the goal.
//
// This is necessary to ensure safe conversions from interfaces to concrete
// types.
func StrictConvertToBase(perm Permission, goal BasePermission) Permission {
	result := convertToBase(perm, goal, (*convertToBaseState)(&mergeState{make(map[mergeStateKey]Permission), mergeStrictConversion}))
	if reflect.DeepEqual(result, perm) {
		result = perm
	}
	return result
}

// ConvertToBase converts a permission to a base permission, by limiting all
// inner permissions to make things consistent.
func ConvertToBase(perm Permission, goal BasePermission) Permission {
	result := convertToBase(perm, goal, (*convertToBaseState)(&mergeState{make(map[mergeStateKey]Permission), mergeConversion}))
	if reflect.DeepEqual(result, perm) {
		result = perm
	}
	return result
}

// Intersect takes two permissions and returns a permission that is a subset
// of both. An intersection can be used to join permissions from two branches:
// Given if { v has permission A } else { v has permission B }, the permission
// for v afterwards will be Intersect(A, B).
func Intersect(perm Permission, goal Permission) (result Permission, err error) {
	defer mergeRecover(&err)
	result = merge(perm, goal, &mergeState{make(map[mergeStateKey]Permission), mergeIntersection})
	if reflect.DeepEqual(result, perm) {
		result = perm
	}
	return
}

// Union takes two permissions and returns a permission that is a superset
// of both permissions. It exists mostly to allow intersecting function
// values: For parameters, we need to calculate unions when intersecting
// (and vice versa). Imagine func(om) and func(or): If we have code that
// wants to use either of those, we need to have a func(om) - after all, we
// cannot pass a readable object to a function expecting a writable one, but
// removing permissions is fine.
func Union(perm Permission, goal Permission) (result Permission, err error) {
	defer mergeRecover(&err)
	result = merge(perm, goal, &mergeState{make(map[mergeStateKey]Permission), mergeUnion})
	if reflect.DeepEqual(result, perm) {
		result = perm
	}
	return
}

// MakeCompatibleTo takes a permission and makes it compatible with the outer
// permission.
func merge(perm Permission, goal Permission, state *mergeState) Permission {
	key := mergeStateKey{perm, goal, state.action}
	result, ok := state.state[key]
	if !ok {
		// FIXME(jak): Temporary code, need to refactor convert to base.
		goalAsBase, goalIsBase := goal.(BasePermission)
		_, permIsBase := perm.(BasePermission)
		if (state.action == mergeConversion || state.action == mergeStrictConversion) && !permIsBase && goalIsBase {
			result = perm.convertToBase(goalAsBase, (*convertToBaseState)(state))
		} else {
			result = perm.merge(goal, state)
		}
		if result == nil {
			panic(mergeError(fmt.Errorf("Cannot merge %v with %v - not compatible", perm, goal)))
		}
	}
	return result
}

func (perm BasePermission) merge(p2 Permission, state *mergeState) Permission {
	switch p2 := p2.(type) {
	case BasePermission:
		return state.mergeBase(perm, p2)
	case *WildcardPermission:
		return perm
	default:
		return nil
	}
}

func (p *PointerPermission) merge(p2 Permission, state *mergeState) Permission {
	switch p2 := p2.(type) {
	case *PointerPermission:
		next := &PointerPermission{}
		state.register(next, p, p2)
		next.BasePermission = state.mergeBase(p.BasePermission, p2.BasePermission)
		next.Target = merge(p.Target, p2.Target, state)
		return next
	case *WildcardPermission, *NilPermission:
		return p
	default:
		return nil
	}
}

func (p *ChanPermission) merge(p2 Permission, state *mergeState) Permission {
	switch p2 := p2.(type) {
	case *ChanPermission:
		next := &ChanPermission{}
		state.register(next, p, p2)
		next.BasePermission = state.mergeBase(p.BasePermission, p2.BasePermission)
		next.ElementPermission = merge(p.ElementPermission, p2.ElementPermission, state)
		return next
	case *WildcardPermission:
		return p
	default:
		return nil
	}
}

func (p *ArrayPermission) merge(p2 Permission, state *mergeState) Permission {
	switch p2 := p2.(type) {
	case *ArrayPermission:
		next := &ArrayPermission{}
		state.register(next, p, p2)
		next.BasePermission = state.mergeBase(p.BasePermission, p2.BasePermission)
		next.ElementPermission = merge(p.ElementPermission, p2.ElementPermission, state)
		return next
	case *WildcardPermission:
		return p
	default:
		return nil
	}
}

func (p *SlicePermission) merge(p2 Permission, state *mergeState) Permission {
	switch p2 := p2.(type) {
	case *SlicePermission:
		next := &SlicePermission{}
		state.register(next, p, p2)
		next.BasePermission = state.mergeBase(p.BasePermission, p2.BasePermission)
		next.ElementPermission = merge(p.ElementPermission, p2.ElementPermission, state)
		return next
	case *WildcardPermission:
		return p
	default:
		return nil
	}
}

func (p *MapPermission) merge(p2 Permission, state *mergeState) Permission {
	switch p2 := p2.(type) {
	case *MapPermission:
		next := &MapPermission{}
		state.register(next, p, p2)
		next.BasePermission = state.mergeBase(p.BasePermission, p2.BasePermission)
		next.KeyPermission = merge(p.KeyPermission, p2.KeyPermission, state)
		next.ValuePermission = merge(p.ValuePermission, p2.ValuePermission, state)
		return next
	case *WildcardPermission:
		return p
	default:
		return nil
	}
}

func (p *StructPermission) merge(p2 Permission, state *mergeState) Permission {
	switch p2 := p2.(type) {
	case *StructPermission:
		next := &StructPermission{}
		state.register(next, p, p2)
		next.BasePermission = state.mergeBase(p.BasePermission, p2.BasePermission)
		next.Fields = make([]Permission, len(p.Fields))
		if len(p.Fields) != len(p2.Fields) {
			panic(mergeError(fmt.Errorf("Cannot make %v compatible to %v: Different number of fields: %d vs %d", p, p2, len(p.Fields), len(p2.Fields))))
		}
		for i := 0; i < len(p.Fields); i++ {
			next.Fields[i] = merge(p.Fields[i], p2.Fields[i], state)
		}
		return next
	case *WildcardPermission:
		return p
	default:
		return nil
	}
}

func (p *FuncPermission) merge(p2 Permission, state *mergeState) Permission {
	switch p2 := p2.(type) {
	case *FuncPermission:
		next := &FuncPermission{}
		state.register(next, p, p2)

		next.BasePermission = state.contravariant().mergeBase(p.BasePermission, p2.BasePermission)
		if len(p.Receivers) != len(p2.Receivers) {
			panic(mergeError(fmt.Errorf("Cannot merge %v to %v: Different number of receivers", p, p2)))
		}
		if p.Receivers != nil {
			next.Receivers = make([]Permission, len(p.Receivers))
			for i := 0; i < len(p.Receivers); i++ {
				next.Receivers[i] = merge(p.Receivers[i], p2.Receivers[i], state.contravariant())
			}
		}
		if len(p.Params) != len(p2.Params) {
			panic(mergeError(fmt.Errorf("Cannot merge %v to %v: Different number of parameters", p, p2)))
		}
		if p.Params != nil {
			next.Params = make([]Permission, len(p.Params))
			for i := 0; i < len(p.Params); i++ {
				next.Params[i] = merge(p.Params[i], p2.Params[i], state.contravariant())
			}
		}
		if len(p.Results) != len(p2.Results) {
			panic(mergeError(fmt.Errorf("Cannot merge %v to %v: Different number of results", p, p2)))
		}
		if p.Results != nil {
			next.Results = make([]Permission, len(p.Results))
			for i := 0; i < len(p.Results); i++ {
				next.Results[i] = merge(p.Results[i], p2.Results[i], state)
			}
		}
		return next
	case *WildcardPermission:
		return p
	default:
		return nil
	}
}

func (p *InterfacePermission) merge(p2 Permission, state *mergeState) Permission {
	switch p2 := p2.(type) {
	case *InterfacePermission:
		next := &InterfacePermission{}
		state.register(next, p, p2)
		next.BasePermission = state.mergeBase(p.BasePermission, p2.BasePermission)
		if len(p.Methods) != len(p2.Methods) {
			panic(mergeError(fmt.Errorf("Cannot merge %v to %v: Different number of methods", p, p2)))
		}
		if p.Methods != nil {
			next.Methods = make([]Permission, len(p.Methods))
			for i := 0; i < len(p.Methods); i++ {
				next.Methods[i] = merge(p.Methods[i], p2.Methods[i], state)
			}
		}
		return next
	case *WildcardPermission:
		return p
	default:
		return nil
	}
}

func (p *WildcardPermission) merge(p2 Permission, state *mergeState) Permission {
	return p2
}

func (p *TuplePermission) merge(p2 Permission, state *mergeState) Permission {
	switch p2 := p2.(type) {
	case *TuplePermission:
		next := &TuplePermission{}
		state.register(next, p, p2)
		next.BasePermission = state.mergeBase(p.BasePermission, p2.BasePermission)
		if len(p.Elements) != len(p2.Elements) {
			panic(mergeError(fmt.Errorf("Cannot merge tuples %v and %v: Different number of elements: %d vs %d", p, p2, len(p.Elements), len(p2.Elements))))
		}
		if p.Elements != nil {
			next.Elements = make([]Permission, len(p.Elements))
			for i := 0; i < len(p.Elements); i++ {
				next.Elements[i] = merge(p.Elements[i], p2.Elements[i], state)
			}
		}
		return next
	case *WildcardPermission:
		return p
	default:
		return nil
	}
}

func (p *NilPermission) merge(p2 Permission, state *mergeState) Permission {
	switch p2 := p2.(type) {
	case *NilPermission, *PointerPermission:
		return p2
	default:
		return nil
	}
}
