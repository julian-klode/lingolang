package permission

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

type convertTestCase struct {
	perm   interface{}
	goal   interface{}
	result interface{}
	err    string
}

func MakePermission(i interface{}) (Permission, error) {
	if i == nil {
		return nil, nil
	}
	switch v := i.(type) {
	case Permission:
		return v, nil
	case string:
		return NewParser(v).Parse()
	}
	return nil, fmt.Errorf("Not a permission: %v", i)
}

var testcasesConvert = []convertTestCase{
	{"or", "r", "r", ""},
	{"r", "or", "or", ""},
	{"a", "or", "or", ""},
	{"a", "on * on", nil, "compatible"},
	{"om * om", "or", "or * or", ""},
	{"om * om", "ol", "ol * om", ""},
	{"om * om", "l", "l * om", ""},
	{"or * a", "or", "or * a", ""},
	{"or * om", "or", "or * or", ""},           // inconsistent: non-linear ptr to lin val
	{"om * om * om", "or", "or * or * or", ""}, // inconsistent: non-linear ptr to lin val
	{"or * ov", "or", "or * ov", ""},           // this is consistent ov=orW is not linear
	{"or * owR", "or", "or * owR", ""},         // consistent: owR is not linear
	{"or * om", "or * ov", "or * ov", ""},
	{"or * om", "or chan or", nil, "compatible"},
	{"om chan om", "or", "or chan or", ""},
	{"om chan om", "or chan or", "or chan or", ""},
	{"om chan om", "or * on", nil, "compatible"},
	{"om []om", "or", "or []or", ""},
	{"om []om", "or []or", "or []or", ""},
	{"om []om", "or * on", nil, "compatible"},
	{"om [1]om", "or", "or [1]or", ""},
	{"om [1]om", "or [1]or", "or [1]or", ""},
	{"om [1]om", "or * on", nil, "compatible"},
	{"om map[om]om", "or", "or map[or]or", ""},
	{"om map[om]om", "or map[or]or", "or map[or]or", ""},
	{"om map[om]om", "or * on", nil, "compatible"},
	{"om struct { om }", "or", "or struct {or}", ""},
	{"om struct { om }", "or struct {on}", "or struct {on}", ""},
	{"om struct { om }", "or struct {on; ov}", nil, "1 vs 2"},
	{"om struct { om }", "or * or", nil, "compatible"},
	{"om func (om) ol", "or", "or func (om) ol", ""},
	{"om func (om, om) ol", "or", "or func (om, om) ol", ""},
	{"om func (om, om) ol", "or func (om, om) ol", "or func (om, om) ol", ""},
	{"om (om) func (om) ol", "or", "or (om) func (om) ol", ""},
	{"om (om) func (om) om", "or (or) func (or) or", "or (or) func (or) or", ""},
	{"om (om) func (om) om", "or (or) func (or)", nil, "number of results"},
	{"om (om) func (om) om", "or (or) func () om", nil, "number of parameters"},
	{"om (om) func (om) om", "or func (or) om", nil, "number of receivers"},
	{"om (om) func (om) om", "or chan om", nil, "compatible"},
	{"om interface { }", "or", "or interface { }", ""},
	{"om interface { }", "om struct {om}", nil, "compatible"},
	{"om interface { om (om) func () }", "or", "or interface { or (om) func () }", ""},                               // unsafe
	{"om interface { om (om) func () }", "or interface { ov (om) func () }", "or interface { ov (om) func () }", ""}, // unsafe
	{"om interface { om (om) func () }", "or interface { ov (om) func (); ov (om)  func () }", nil, "number of methods"},
	{MakeRecursivePointer(true), "om", MakeRecursivePointer(true), ""},
	{MakeRecursivePointer(true), "om * or", MakeRecursivePointer(false), ""},
	{MakeRecursiveStruct(true), "om struct { om * om }", MakeRecursiveStruct(true), ""},
	{MakeRecursiveStruct(true), "om struct { or * or }", MakeRecursiveStruct(false), ""},
	{MakeRecursiveStruct(true), "om struct { or }", MakeRecursiveStruct(false), ""},
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

func TestConvertTo(t *testing.T) {
	for _, testCase := range testcasesConvert {
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

			realResult, err := ConvertTo(perm, goal)
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
