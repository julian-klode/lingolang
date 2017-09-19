package permission

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

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
