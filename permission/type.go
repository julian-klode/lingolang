// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.
//

package permission

import (
	"fmt"
	"go/types"
)

const basicPermission = Mutable

// TypeMapper maintains a map from types to permissions, so recursive types
// and type references can be handled correctly.
type TypeMapper map[types.Type]Permission

// NewTypeMapper constructs a new type mapper
func NewTypeMapper() TypeMapper {
	return make(TypeMapper)
}

// NewFromType constructs a new permission that is as permissive
// as possible for a given type.
//
// TODO ownership
func (typeMapper TypeMapper) NewFromType(t0 types.Type) (result Permission) {
	if r, ok := typeMapper[t0]; ok {
		return r
	}
	// Simple type dispatch
	switch t := t0.(type) {
	case *types.Array:
		return typeMapper.newFromArrayType(t)
	case *types.Slice:
		return typeMapper.newFromSliceType(t)
	case *types.Chan:
		return typeMapper.newFromChanType(t)
	case *types.Pointer:
		return typeMapper.newFromPointerType(t)
	case *types.Map:
		return typeMapper.newFromMapType(t)
	case *types.Struct:
		return typeMapper.newFromStructType(t)
	case *types.Signature:
		return typeMapper.newFromSignatureType(t)
	case *types.Interface:
		return typeMapper.newFromInterfaceType(t)
	case *types.Basic:
		return basicPermission
	default:
		// Fall through to the underlying type.
		t0 = t.Underlying()
		if t0 != t {
			return typeMapper.NewFromType(t0)
		}
	}
	panic(fmt.Errorf("Cannot create permission for type %#v", t0))
}

func (typeMapper TypeMapper) newFromChanType(t *types.Chan) Permission {
	perm := &ChanPermission{BasePermission: basicPermission}
	typeMapper[t] = perm
	perm.ElementPermission = typeMapper.NewFromType(t.Elem())
	return perm
}

type arrayOrSliceType interface {
	types.Type
	Elem() types.Type
}

func (typeMapper TypeMapper) newFromArrayType(t arrayOrSliceType) Permission {
	perm := &ArrayPermission{BasePermission: basicPermission}
	typeMapper[t] = perm
	perm.ElementPermission = typeMapper.NewFromType(t.Elem())
	return perm
}

func (typeMapper TypeMapper) newFromSliceType(t arrayOrSliceType) Permission {
	perm := &SlicePermission{BasePermission: basicPermission}
	typeMapper[t] = perm
	perm.ElementPermission = typeMapper.NewFromType(t.Elem())
	return perm
}

func (typeMapper TypeMapper) newFromMapType(t *types.Map) Permission {
	perm := &MapPermission{BasePermission: basicPermission}
	typeMapper[t] = perm
	perm.KeyPermission = typeMapper.NewFromType(t.Key())
	perm.ValuePermission = typeMapper.NewFromType(t.Elem())
	return perm
}
func (typeMapper TypeMapper) newFromPointerType(t *types.Pointer) Permission {
	perm := &PointerPermission{BasePermission: basicPermission}
	typeMapper[t] = perm
	perm.Target = typeMapper.NewFromType(t.Elem())
	return perm
}
func (typeMapper TypeMapper) newFromStructType(t *types.Struct) Permission {
	perm := &StructPermission{BasePermission: basicPermission}
	typeMapper[t] = perm
	for i := 0; i < t.NumFields(); i++ {
		perm.Fields = append(perm.Fields, typeMapper.NewFromType(t.Field(i).Type()))
	}
	return perm
}

func (typeMapper TypeMapper) newFromSignatureType(t *types.Signature) Permission {
	perm := &FuncPermission{BasePermission: basicPermission}
	typeMapper[t] = perm
	if r := t.Recv(); r != nil {
		perm.Receivers = append(perm.Receivers, typeMapper.NewFromType(r.Type()))
	}
	for ps, i := t.Params(), 0; i < ps.Len(); i++ {
		perm.Params = append(perm.Params, typeMapper.NewFromType(ps.At(i).Type()))
	}
	for res, i := t.Results(), 0; i < res.Len(); i++ {
		perm.Results = append(perm.Results, typeMapper.NewFromType(res.At(i).Type()))
	}
	return perm
}

func (typeMapper TypeMapper) newFromInterfaceType(t *types.Interface) Permission {
	perm := &InterfacePermission{BasePermission: basicPermission}
	typeMapper[t] = perm
	for i := 0; i < t.NumMethods(); i++ {
		perm.Methods = append(perm.Methods, typeMapper.NewFromType(t.Method(i).Type()))
	}
	return perm
}
