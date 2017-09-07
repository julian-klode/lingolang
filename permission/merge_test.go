package permission

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

type mergeTestCase struct {
	testfun func(p1, p2 Permission) (Permission, error)
	perm    interface{}
	goal    interface{}
	result  interface{}
	err     string
}

var testcasesMerge = []mergeTestCase{
	{Intersect, "orw", "or", "or", ""},
	{Union, "orw", "or", "orw", ""},
	{Intersect, "om", "or", "or", ""},
	{Intersect, "m", "or", "r", ""},
	{Intersect, "om * om", "or * or", "or * or", ""},
	{Intersect, "om * om", "ol * om", "ol * om", ""},
	{Intersect, "om chan om", "or chan or", "or chan or", ""},
	{Intersect, "or chan or", "om chan om", "or chan or", ""},
	{Intersect, "om []om", "or []or", "or []or", ""},
	{Intersect, "or []or", "om []om", "or []or", ""},
	{Intersect, "om [1]om", "or [1]or", "or [1]or", ""},
	{Intersect, "or [1]or", "om [1]om", "or [1]or", ""},
	{Intersect, "om map[om]om", "or map[or]or", "or map[or]or", ""},
	{Intersect, "or map[or]or", "om map[om]om", "or map[or]or", ""},
	{Intersect, "om struct { om }", "or struct { or }", "or struct { or }", ""},
	{Intersect, "om struct { om }", "or struct { or; or }", nil, "number of fields"},
	{Intersect, "om (om) func(om) om", "or (or) func(or) or", "om (om) func(om) or", ""},
	{Intersect, "om func(om)", "or func(or)", "om func(om)", ""},
	{Union, "om func(om)", "or func(or)", "or func(or)", ""},
	{Intersect, "om func(om, om)", "or func(or)", nil, "number of parameters"},
	{Intersect, "om func()(om, om)", "or func()(or)", nil, "number of results"},
	{Intersect, "om (om)func()", "or func()", nil, "number of receivers"},
	{Intersect, "om func()", "or func()", "om func()", ""},
	{Intersect, "om interface{}", "or interface{}", "or interface{}", ""},
	{Intersect, "om interface{}", "or interface{om (om) func()}", nil, "number of methods"},
	{Intersect, "om interface{om (om) func()}", "or interface{or (or) func()}", "or interface { om (om) func()}", ""},
	// nil cases: Incompatible permission types
	{Intersect, "om", "or * or", nil, "Cannot merge"},
	{Intersect, "om * om", "om", nil, "Cannot merge"},
	{Intersect, "om chan om", "om", nil, "Cannot merge"},
	{Intersect, "om []om", "om", nil, "Cannot merge"},
	{Intersect, "om [1]om", "om", nil, "Cannot merge"},
	{Intersect, "om map[om]om", "om", nil, "Cannot merge"},
	{Intersect, "om struct { om }", "om", nil, "Cannot merge"},
	{Intersect, "om func()", "om", nil, "Cannot merge"},
	{Intersect, "om interface {}", "om", nil, "Cannot merge"},
	{Union, "om", "or * or", nil, "Cannot merge"},
	{Union, "om * om", "om", nil, "Cannot merge"},
	{Union, "om chan om", "om", nil, "Cannot merge"},
	{Union, "om []om", "om", nil, "Cannot merge"},
	{Union, "om [1]om", "om", nil, "Cannot merge"},
	{Union, "om map[om]om", "om", nil, "Cannot merge"},
	{Union, "om struct { om }", "om", nil, "Cannot merge"},
	{Union, "om func()", "om", nil, "Cannot merge"},
	{Union, "om interface {}", "om", nil, "Cannot merge"},
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

			realResult, err := testCase.testfun(perm, goal)
			if !reflect.DeepEqual(realResult, result) {
				t.Errorf("Unexpected result %v, expected %v (%v)", realResult, result, testCase.result)
			}
			switch {
			case testCase.err != "" && (err == nil || !strings.Contains(err.Error(), testCase.err)):
				t.Errorf("Expected an error containing %s, got %v", testCase.err, err)
			case testCase.err == "" && err != nil:
				t.Errorf("Expected nil error, got %v", err)
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
