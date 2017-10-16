package permission

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

type tuplePermission []string

func MakePermission(i interface{}) (Permission, error) {
	if i == nil {
		return nil, nil
	}
	switch v := i.(type) {
	case Permission:
		return v, nil
	case string:
		return NewParser(v).Parse()
	case tuplePermission:
		base, err := NewParser(v[0]).Parse()
		if err != nil {
			return nil, err
		}
		var others []Permission
		for _, s := range v[1:] {
			o, err := NewParser(s).Parse()
			if err != nil {
				return nil, err
			}
			others = append(others, o)
		}

		return &TuplePermission{base.(BasePermission), others}, nil
	}
	return nil, fmt.Errorf("Not a permission: %v", i)
}

func MakeRecursivePointer(innerWritable bool) Permission {
	if innerWritable {
		p := &PointerPermission{
			BasePermission: Owned | Mutable,
		}
		p.Target = p
		return p
	}

	p0 := &PointerPermission{
		BasePermission: Owned | Read,
	}
	p0.Target = p0
	return &PointerPermission{
		BasePermission: Owned | Mutable,
		Target:         p0,
	}

}
func MakeRecursiveStruct(innerWritable bool) Permission {
	if innerWritable {
		p := &StructPermission{
			BasePermission: Owned | Mutable,
		}
		p.Fields = []Permission{&PointerPermission{Owned | Mutable, p}}
		return p
	}
	p0 := &StructPermission{
		BasePermission: Owned | Read,
	}
	p0.Fields = []Permission{&PointerPermission{Owned | Read, p0}}
	return &StructPermission{BasePermission: Owned | Mutable, Fields: []Permission{&PointerPermission{Owned | Read, p0}}}
}

type mergeTestCase struct {
	testfun mergeAction
	perm    interface{}
	goal    interface{}
	result  interface{}
	err     string
}

type mergeFunc func(p1, p2 Permission) (Permission, error)

func reverse(f mergeFunc) mergeFunc {
	return func(p1, p2 Permission) (Permission, error) {
		return f(p2, p1)
	}
}

func adapt(ma mergeAction) []mergeFunc {
	switch ma {
	case mergeUnion:
		return []mergeFunc{Union, reverse(Union)}
	case mergeIntersection:
		return []mergeFunc{Intersect, reverse(Intersect)}
	case mergeConversion:
		return []mergeFunc{ConvertTo}
	case mergeStrictConversion:
		return []mergeFunc{func(p1, p2 Permission) (Permission, error) {
			return StrictConvertToBase(p1, p2.(BasePermission)), nil
		}}
	}
	panic("Should not happen")
}

var testcasesMerge = []mergeTestCase{
	{mergeIntersection, "orw", "or", "or", ""},
	{mergeUnion, "orw", "or", "orw", ""},
	{mergeIntersection, "om", "or", "or", ""},
	{mergeIntersection, "m", "or", "r", ""},
	{mergeIntersection, &NilPermission{}, "or * or", "or * or", ""},
	{mergeUnion, &NilPermission{}, "or * or", "or * or", ""},
	{mergeUnion, &NilPermission{}, &NilPermission{}, &NilPermission{}, ""},
	{mergeUnion, &NilPermission{}, "om", nil, "not compatible"},
	{mergeConversion, &NilPermission{}, "or * or", "or * or", ""},
	{mergeStrictConversion, &NilPermission{}, "or", &NilPermission{}, ""},
	{mergeConversion, &NilPermission{}, "or", &NilPermission{}, ""},
	{mergeIntersection, "om * om", "or * or", "or * or", ""},
	{mergeIntersection, "om * om", "ol * om", "ol * om", ""},
	{mergeIntersection, "om chan om", "or chan or", "or chan or", ""},
	{mergeIntersection, "or chan or", "om chan om", "or chan or", ""},
	{mergeIntersection, "om []om", "or []or", "or []or", ""},
	{mergeIntersection, "or []or", "om []om", "or []or", ""},
	{mergeIntersection, "om [1]om", "or [1]or", "or [1]or", ""},
	{mergeIntersection, "or [1]or", "om [1]om", "or [1]or", ""},
	{mergeIntersection, "om map[om]om", "or map[or]or", "or map[or]or", ""},
	{mergeIntersection, "or map[or]or", "om map[om]om", "or map[or]or", ""},
	{mergeIntersection, "om struct { om }", "or struct { or }", "or struct { or }", ""},
	{mergeIntersection, "om struct { om }", "or struct { or; or }", nil, "number of fields"},
	{mergeIntersection, "om (om) func(om) om", "or (or) func(or) or", "om (om) func(om) or", ""},
	{mergeIntersection, "om func(om)", "or func(or)", "om func(om)", ""},
	{mergeUnion, "om func(om)", "or func(or)", "or func(or)", ""},
	{mergeIntersection, "om func(om, om)", "or func(or)", nil, "number of parameters"},
	{mergeIntersection, "om func()(om, om)", "or func()(or)", nil, "number of results"},
	{mergeIntersection, "om (om)func()", "or func()", nil, "number of receivers"},
	{mergeIntersection, "om func()", "or func()", "om func()", ""},
	{mergeIntersection, "om interface{}", "or interface{}", "or interface{}", ""},
	{mergeIntersection, "om interface{}", "or interface{om (om) func()}", nil, "number of methods"},
	{mergeIntersection, "om interface{om (om) func()}", "or interface{or (or) func()}", "or interface { om (om) func()}", ""},
	// nil cases: Incompatible permission types
	{mergeIntersection, "om", "or * or", nil, "Cannot merge"},
	{mergeIntersection, "om * om", "om", nil, "Cannot merge"},
	{mergeIntersection, "om chan om", "om", nil, "Cannot merge"},
	{mergeIntersection, "om []om", "om", nil, "Cannot merge"},
	{mergeIntersection, "om [1]om", "om", nil, "Cannot merge"},
	{mergeIntersection, "om map[om]om", "om", nil, "Cannot merge"},
	{mergeIntersection, "om struct { om }", "om", nil, "Cannot merge"},
	{mergeIntersection, "om func()", "om", nil, "Cannot merge"},
	{mergeIntersection, "om interface {}", "om", nil, "Cannot merge"},
	{mergeUnion, "om", "or * or", nil, "Cannot merge"},
	{mergeUnion, "om * om", "om", nil, "Cannot merge"},
	{mergeUnion, "om chan om", "om", nil, "Cannot merge"},
	{mergeUnion, "om []om", "om", nil, "Cannot merge"},
	{mergeUnion, "om [1]om", "om", nil, "Cannot merge"},
	{mergeUnion, "om map[om]om", "om", nil, "Cannot merge"},
	{mergeUnion, "om struct { om }", "om", nil, "Cannot merge"},
	{mergeUnion, "om func()", "om", nil, "Cannot merge"},
	{mergeUnion, "om interface {}", "om", nil, "Cannot merge"},
	/* Wildcard cases */
	{mergeUnion, "om chan om", "_", "om chan om", ""},
	{mergeUnion, "om interface {}", "_", "om interface{}", ""},
	{mergeUnion, "om func ()", "_", "om func ()", ""},
	{mergeUnion, "om struct {om}", "_", "om struct {om}", ""},
	{mergeUnion, "om map[om] om", "_", "om map[om] om", ""},
	{mergeUnion, "om * om", "_", "om * om", ""},
	{mergeUnion, "om [] om", "_", "om [] om", ""},
	{mergeUnion, "om [1] om", "_", "om [1] om", ""},
	{mergeUnion, "om", "_", "om", ""},
	{mergeUnion, tuplePermission{"om"}, "_", tuplePermission{"om"}, ""},
	{mergeStrictConversion, "om * om", "or", "or * or", ""},
	{mergeStrictConversion, "om * om", "om", "om * om", ""},
	{mergeConversion, "or", "_", "or", ""},
	{mergeConversion, "_", "or", "or", ""},
	{mergeConversion, "or", "r", "r", ""},
	{mergeConversion, "r", "or", "or", ""},
	{mergeConversion, "a", "or", "or", ""},
	{mergeConversion, "a", "on * on", nil, "compatible"},
	{mergeConversion, "om * om", "or", "or * or", ""},
	{mergeConversion, "om * om", "ol", "ol * om", ""},
	{mergeConversion, "om * om", "ol", "ol * om", ""},
	{mergeConversion, "or * a", "or", "or * oa", ""},
	{mergeConversion, "or * r", "or", "or * or", ""},
	{mergeConversion, "r * or", "r", "r * r", ""},
	{mergeConversion, "or * om", "or", "or * or", ""},           // inconsistent: non-linear ptr to lin val
	{mergeConversion, "om * om * om", "or", "or * or * or", ""}, // inconsistent: non-linear ptr to lin val
	{mergeConversion, "or * ov", "or", "or * ov", ""},           // this is consistent ov=orW is not linear
	{mergeConversion, "or * owR", "or", "or * owR", ""},         // consistent: owR is not linear
	{mergeConversion, "or * om", "or * ov", "or * ov", ""},
	{mergeConversion, "or * om", "or chan or", nil, "compatible"},
	{mergeConversion, "om chan om", "or", "or chan or", ""},
	{mergeConversion, "om chan om", "or chan or", "or chan or", ""},
	{mergeConversion, "om chan om", "or * on", nil, "compatible"},
	{mergeConversion, "om []om", "or", "or []or", ""},
	{mergeConversion, "om []om", "or []or", "or []or", ""},
	{mergeConversion, "om []om", "or * on", nil, "compatible"},
	{mergeConversion, "om [1]om", "or", "or [1]or", ""},
	{mergeConversion, "om [1]om", "or [1]or", "or [1]or", ""},
	{mergeConversion, "om [1]om", "or * on", nil, "compatible"},
	{mergeConversion, "om map[om]om", "or", "or map[or]or", ""},
	{mergeConversion, "om map[om]om", "or map[or]or", "or map[or]or", ""},
	{mergeConversion, "om map[om]om", "or * on", nil, "compatible"},
	{mergeConversion, "om struct { om }", "or", "or struct {or}", ""},
	{mergeConversion, "om struct { om }", "or struct {on}", "or struct {on}", ""},
	{mergeConversion, "om struct { om }", "or struct {on; ov}", nil, "1 vs 2"},
	{mergeConversion, "om struct { om }", "or * or", nil, "compatible"},
	{mergeConversion, "om func (om) ol", "or", "or func (om) ol", ""},
	{mergeConversion, "om func (om, om) ol", "or", "or func (om, om) ol", ""},
	{mergeConversion, "om func (om, om) ol", "or func (om, om) ol", "or func (om, om) ol", ""},
	{mergeConversion, "om (om) func (om) ol", "or", "or (om) func (om) ol", ""},
	{mergeConversion, "om (om) func (om) om", "or (or) func (or) or", "or (or) func (or) or", ""},
	{mergeConversion, "om (om) func (om) om", "or (or) func (or)", nil, "number of results"},
	{mergeConversion, "om (om) func (om) om", "or (or) func () om", nil, "number of parameters"},
	{mergeConversion, "om (om) func (om) om", "or func (or) om", nil, "number of receivers"},
	{mergeConversion, "om (om) func (om) om", "or chan om", nil, "compatible"},
	{mergeConversion, "om interface { }", "or", "or interface { }", ""},
	{mergeConversion, "om interface { }", "om struct {om}", nil, "compatible"},
	{mergeConversion, "om interface { om (om) func () }", "or", "or interface { om (om) func () }", ""},                               // unsafe
	{mergeConversion, "om interface { om (om) func () }", "or interface { ov (om) func () }", "or interface { ov (om) func () }", ""}, // unsafe
	{mergeConversion, "om interface { om (om) func () }", "or interface { ov (om) func (); ov (om)  func () }", nil, "number of methods"},
	{mergeConversion, MakeRecursivePointer(true), "om", MakeRecursivePointer(true), ""},
	{mergeConversion, MakeRecursivePointer(true), "om * or", MakeRecursivePointer(false), ""},
	{mergeConversion, MakeRecursiveStruct(true), "om struct { om * om }", MakeRecursiveStruct(true), ""},
	{mergeConversion, MakeRecursiveStruct(true), "om struct { or * or }", MakeRecursiveStruct(false), ""},
	{mergeConversion, MakeRecursiveStruct(true), "om struct { or }", MakeRecursiveStruct(false), ""},

	// Linear values containing mutable stuff should not happen
	{mergeConversion, "ol []om", "ol", "ol []ol", ""},
	{mergeConversion, "ol [_]om", "ol", "ol [_]ol", ""},
	{mergeConversion, "ol map[om]om", "ol", "ol map[ol]ol", ""},
	{mergeConversion, "ol struct { om }", "ol", "ol struct { ol }", ""},
	{mergeConversion, "ol chan om", "ol", "ol chan ol", ""},
	// Two exceptions: Interfaces (methods are special) and pointers
	{mergeConversion, "ol interface { om func () }", "ol", "ol interface { om func() }", ""},
	{mergeConversion, "ol * om", "ol", "ol * om", ""},

	{mergeConversion, tuplePermission{"om", "om"}, "or", tuplePermission{"or", "or"}, ""},
	{mergeConversion, tuplePermission{"om", "om"}, tuplePermission{"or", "on"}, tuplePermission{"or", "on"}, ""},
	{mergeConversion, tuplePermission{"om", "om"}, tuplePermission{"or", "on", "ov"}, nil, "1 vs 2"},
	{mergeConversion, tuplePermission{"om", "om"}, "or * or", nil, "compatible"},
}

func TestMergeTo(t *testing.T) {
	for _, testCase := range testcasesMerge {
		testCase := testCase
		t.Run(fmt.Sprintf("%v to %v", testCase.perm, testCase.goal), func(t *testing.T) {
			perm, err := MakePermission(testCase.perm)
			if err != nil {
				t.Fatalf("Invalid perm %v", testCase.perm)
			}
			goal, err := MakePermission(testCase.goal)
			if err != nil {
				t.Fatalf("Invalid goal %v", testCase.goal)
			}
			result, err := MakePermission(testCase.result)
			if err != nil {
				t.Fatalf("Invalid result %v", testCase.result)
			}
			if goal, ok := goal.(BasePermission); ok && testCase.testfun == mergeConversion {
				realResult := ConvertToBase(perm, goal)
				if !reflect.DeepEqual(realResult, result) {
					t.Errorf("Unexpected result %v, expected %v (%v)", realResult, result, testCase.result)
				}
			}

			for i, mergeFunc := range adapt(testCase.testfun) {
				realResult, err := mergeFunc(perm, goal)
				if !reflect.DeepEqual(realResult, result) {
					t.Errorf("%d: Unexpected result %v, expected %v (%v)", i, realResult, result, testCase.result)
				}
				switch {
				case testCase.err != "" && (err == nil || !strings.Contains(err.Error(), testCase.err)):
					t.Errorf("%d: Expected an error containing %s, got %v", i, testCase.err, err)
				case testCase.err == "" && err != nil:
					t.Errorf("%d: Expected nil error, got %v", i, err)
				}
			}
		})
	}
}

// TestMergeTo_panic checks that code actually panics on stuff that should
// not be returned as errors.
func TestMergeTo_panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("The code did not panic")
		}
	}()
	Intersect(nil, nil)
}

func TestMergeBase_panic(t *testing.T) {
	defer func() {
		if r := fmt.Sprint(recover()); !strings.Contains(r, "nvalid merge action") {
			t.Errorf("The code did not panic about invalid merge, but with %s", r)
		}
	}()
	st := mergeState{action: -1}
	st.mergeBase(None, None)
}

// TestConvertTo_panic checks that code actually panics on stuff that should
// not be returned as errors.
func TestConvertTo_panic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("The code did not panic")
		}
	}()
	ConvertTo(nil, nil)
}
