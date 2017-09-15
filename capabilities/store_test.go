// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package capabilities

import (
	"fmt"
	"go/ast"
	"strings"
	"testing"

	"github.com/julian-klode/lingolang/permission"
)

func TestStore_equal(t *testing.T) {
	var equalStores = []struct{ A, B Store }{
		{nil, nil},
		{nil, NewStore()},
		{nil, NewStore().BeginBlock().EndBlock()},
		{NewStore(), NewStore()},
		{NewStore().BeginBlock(), NewStore().BeginBlock()},
		{NewStore().BeginBlock().EndBlock(), NewStore().BeginBlock().EndBlock()},
	}

	for i, test := range equalStores {
		if !test.A.Equal(test.B) {
			t.Errorf("test %d: %s does not equal %s", i, test.A, test.B)
		}
		if !test.B.Equal(test.A) {
			t.Errorf("test %d: %s does not equal %s", i, test.A, test.B)
		}
	}
}

func TestStore_BeginEndBlock(t *testing.T) {
	st := NewStore()
	if len(st) != 0 {
		t.Fatalf("Store is not empty: %v", st)
	}

	block := st.BeginBlock()
	if block.Equal(st) {
		t.Fatalf("block is same as Store")
	}
	block2 := block.BeginBlock()
	if block2.Equal(block) {
		t.Fatalf("block2 is same as block")
	}
	if len(block) != 1 {
		t.Fatalf("block has %d markers, expected %d", len(block), 1)
	}
	if len(block2) != 2 {
		t.Fatalf("block2 has %d markers, expected %d", len(block2), 2)
	}
	unblock2 := block2.EndBlock()
	if !unblock2.Equal(block) {
		t.Fatalf("unblock2 is not same as block")
	}
	unblock := block.EndBlock()
	if !unblock.Equal(st) {
		t.Fatalf("unblocking is different from initial Store: %v vs %v", unblock, st)
	}
}

func TestStore_getset(t *testing.T) {
	st := NewStore()
	block := st.BeginBlock()
	unblock := block.EndBlock()
	a := ast.NewIdent("a")
	b := ast.NewIdent("b")

	block = block.Define(a, permission.Mutable)
	if !unblock.Equal(st) {
		t.Fatalf("unblocking is different from initial Store after definition in block: %v vs %v", unblock, st)
	}

	if block.GetEffective(a) != permission.Mutable {
		t.Fatalf("%v has effective perm %v expected %v", a, block.GetEffective(a), permission.Mutable)
	}
	if block.GetMaximum(a) != permission.Mutable {
		t.Fatalf("%v has max perm %v expected %v", a, block.GetMaximum(a), permission.Mutable)
	}
	if block.GetEffective(b) != nil {
		t.Fatalf("%v has effective perm %v expected %v", b, block.GetEffective(b), nil)
	}
	if block.GetMaximum(b) != nil {
		t.Fatalf("%v has max perm %v expected %v", b, block.GetMaximum(b), nil)
	}
	block, err := block.SetMaximum(a, permission.ReadOnly)
	if err != nil {
		t.Fatalf("setting maximum produced error %v", err)
	}
	if block.GetEffective(a) != permission.ReadOnly {
		t.Fatalf("%v has effective perm %v expected %v", a, block.GetEffective(a), permission.ReadOnly)
	}
	if block.GetMaximum(a) != permission.ReadOnly {
		t.Fatalf("%v has max perm %v expected %v", a, block.GetMaximum(a), permission.ReadOnly)
	}
}

func TestStore_SetEffectiveSetMaximum(t *testing.T) {
	block := NewStore()
	a := ast.NewIdent("a")
	b := ast.NewIdent("b")
	block = block.Define(a, permission.Mutable)
	block = block.Define(b, permission.ReadOnly)

	block, err := block.SetEffective(a, permission.None)
	if err != nil {
		t.Fatalf("setting effective produced error %v", err)
	}
	if block.GetEffective(a) != permission.None {
		t.Fatalf("%v has effective perm %v expected %v", a, block.GetEffective(a), permission.None)
	}
	if block.GetMaximum(a) != permission.Mutable {
		t.Fatalf("%v has max perm %v expected %v", a, block.GetMaximum(a), permission.Mutable)
	}
	block, err = block.SetEffective(b, permission.Mutable)
	if err != nil {
		t.Fatalf("setting effective produced error %v", err)
	}
	if block.GetEffective(b) != permission.ReadOnly {
		t.Fatalf("%v has effective perm %v expected %v", b, block.GetEffective(b), permission.ReadOnly)
	}
	if block.GetMaximum(b) != permission.ReadOnly {
		t.Fatalf("%v has max perm %v expected %v", b, block.GetMaximum(b), permission.ReadOnly)
	}

	// Check that setting an invalid maximum does not change value
	block1, err := block.SetMaximum(b, &permission.InterfacePermission{BasePermission: permission.ReadOnly})
	if block1 != nil {
		t.Errorf("setting max produced a block")
	}
	if err == nil {
		t.Errorf("setting max produced no error despite max being other shape")
	} else if !strings.Contains(err.Error(), "restrict effective") {
		t.Errorf("received error %s when setting iface max with primitive eff, expected restrictment error", err)
	}
	if block.GetMaximum(b) != permission.ReadOnly {
		t.Fatalf("%v has max perm %v expected %v", b, block.GetMaximum(b), permission.ReadOnly)
	}
	if block.GetEffective(b) != permission.ReadOnly {
		t.Fatalf("%v has effective perm %v expected %v", b, block.GetEffective(b), permission.ReadOnly)
	}
	if block.GetEffective(b) != permission.ReadOnly {
		t.Fatalf("%v has effective perm %v expected %v", b, block.GetEffective(b), permission.ReadOnly)
	}
	if block.GetMaximum(b) != permission.ReadOnly {
		t.Fatalf("%v has max perm %v expected %v", b, block.GetMaximum(b), permission.ReadOnly)
	}

	// Check that setting effective with a different type of max does not change
	// anything
	complexPerm := &permission.InterfacePermission{BasePermission: permission.ReadOnly}
	block[0].max = complexPerm
	block1, err = block.SetEffective(b, permission.Mutable)
	if block1 != nil {
		t.Errorf("setting effective produced a result despite max being other shape")
	}
	if err == nil {
		t.Errorf("setting effective produced no error despite max being other shape")
	} else if !strings.Contains(err.Error(), "restrict effective") {
		t.Errorf("received error %s when setting primitive eff with iface max, expected restrictment error", err)
	}
	if block.GetEffective(b) != permission.ReadOnly {
		t.Fatalf("%v has effective perm %v expected %v", b, block.GetEffective(b), permission.ReadOnly)
	}
	if block.GetMaximum(b) != complexPerm {
		t.Fatalf("%v has max perm %v expected %v", b, block.GetMaximum(b), complexPerm)
	}

}

func TestStore_panic(t *testing.T) {
	shouldPanic := func(name string, exp string, fun func()) {
		defer func() {
			e := recover()
			if !strings.Contains(fmt.Sprint(e), exp) {
				t.Errorf("%s: panicked with %s, expected to contain %s", name, e, exp)
			}
		}()
		fun()
	}
	st := NewStore()
	shouldPanic("setMaximum", "nonexisting", func() { st.SetMaximum(ast.NewIdent("a"), permission.Mutable) })
	shouldPanic("setEffective", "nonexisting", func() { st.SetEffective(ast.NewIdent("a"), permission.Mutable) })
	shouldPanic("EndBlock without block", "Not inside a block", func() { NewStore().EndBlock() })
}

func TestStore_Merge(t *testing.T) {
	// Test case 1: Succesful merge
	st1 := make(Store, 1)
	st1[0].ident = ast.NewIdent("a")
	st1[0].eff = &permission.InterfacePermission{BasePermission: permission.ReadOnly}
	st1[0].max = &permission.InterfacePermission{BasePermission: permission.Mutable}
	st11, err := st1.Merge(st1)
	if err != nil {
		t.Errorf("Could not merge %v with itself: %s", st1, err)
	} else if !st11.Equal(st1) {
		t.Errorf("st11 = %v does not equal st1 = %v", st11, st1)
	}
	// Test case 2 a) Incompatible effective permissions
	st2a := make(Store, 1)
	st2a[0].ident = st1[0].ident
	st2a[0].eff = permission.ReadOnly
	st2a[0].max = permission.Mutable
	st12a, err := st1.Merge(st2a)
	if err == nil {
		t.Errorf("Could merge st1=%v with st2a=%v: %v", st1, st2a, st12a)
	} else if !strings.Contains(err.Error(), "effective") {
		t.Errorf("Unexpected error for st12a: %s, expected effective", err)
	}

	// Test case 2 b) Incompatible effective permissions
	st2b := make(Store, 1)
	st2b[0].ident = st1[0].ident
	st2b[0].eff = st1[0].eff
	st2b[0].max = permission.Mutable
	st12b, err := st1.Merge(st2b)
	if err == nil {
		t.Errorf("Could merge st1=%v with st2b=%v: %v", st1, st2b, st12b)
	} else if !strings.Contains(err.Error(), "maximum") {
		t.Errorf("Unexpected error for st12b: %s, expected maximum", err)
	}

	// Test case 3: Different identifiers
	st3 := make(Store, 1)
	st3[0].ident = ast.NewIdent("b")
	st3[0].eff = permission.ReadOnly
	st3[0].max = permission.Mutable
	st13, err := st1.Merge(st3)
	if err == nil {
		t.Errorf("Could merge st1=%v with st3=%v: %v", st1, st3, st13)
	} else if !strings.Contains(err.Error(), "Different ident") {
		t.Errorf("Unexpected error for st13: %s, expected Different ident", err)
	}
}
