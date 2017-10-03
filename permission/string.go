// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package permission

import (
	"fmt"
	"strings"
)

// String renders the base permission in its Canonical form.
func (perm BasePermission) string(seen map[Permission]bool) string {
	var result string
	if perm&Owned != 0 {
		result = "o"
	}

	switch perm &^ Owned {
	case Mutable:
		return result + "m"
	case LinearValue:
		return result + "l"
	case Value:
		return result + "v"
	case ReadOnly:
		return result + "r"
	case Any &^ Owned:
		return "a" // special case: any implies owned.
	case None:
		return result + "n"
	default:
		if perm&Read != 0 {
			result += "r"
		}
		if perm&Write != 0 {
			result += "w"
		}
		if perm&ExclRead != 0 {
			result += "R"
		}
		if perm&ExclWrite != 0 {
			result += "W"
		}
		return result
	}
}

func (perm BasePermission) String() string {
	return perm.string(make(map[Permission]bool))
}

func (perm *SlicePermission) string(seen map[Permission]bool) string {
	if seen[perm] {
		return "<seen>"
	}

	seen[perm] = true

	return fmt.Sprintf("%s []%s", perm.BasePermission.string(seen), perm.ElementPermission.string(seen))
}

func (perm *SlicePermission) String() string {
	return perm.string(make(map[Permission]bool))
}

func (perm *ArrayPermission) string(seen map[Permission]bool) string {
	if seen[perm] {
		return "<seen>"
	}

	seen[perm] = true

	return fmt.Sprintf("%s [_]%s", perm.BasePermission.string(seen), perm.ElementPermission.string(seen))
}

func (perm *ArrayPermission) String() string {
	return perm.string(make(map[Permission]bool))
}

func (perm *ChanPermission) string(seen map[Permission]bool) string {
	if seen[perm] {
		return "<seen>"
	}

	seen[perm] = true

	return fmt.Sprintf("%s chan %s", perm.BasePermission.string(seen), perm.ElementPermission.string(seen))
}

func (perm *ChanPermission) String() string {
	return perm.string(make(map[Permission]bool))
}

func (perm *PointerPermission) string(seen map[Permission]bool) string {
	if seen[perm] {
		return "<seen>"
	}

	seen[perm] = true

	return fmt.Sprintf("%s *%s", perm.BasePermission.string(seen), perm.Target.string(seen))
}

func (perm *PointerPermission) String() string {
	return perm.string(make(map[Permission]bool))
}

func (perm *MapPermission) string(seen map[Permission]bool) string {
	if seen[perm] {
		return "<seen>"
	}

	seen[perm] = true

	return fmt.Sprintf("%s map[%s]%s", perm.BasePermission.string(seen), perm.KeyPermission.string(seen), perm.ValuePermission.string(seen))
}

func (perm *MapPermission) String() string {
	return perm.string(make(map[Permission]bool))
}

func (perm *FuncPermission) string(seen map[Permission]bool) string {
	if seen[perm] {
		return "<seen>"
	}

	doTuple := func(list []Permission) string {
		if list == nil {
			return "<nil>"
		}
		if len(list) == 0 {
			return "()"
		}
		var stringList []string

		for _, r := range list {
			stringList = append(stringList, r.string(seen))
		}

		return " (" + strings.Join(stringList, ", ") + ")"
	}

	seen[perm] = true

	result := ""
	result += doTuple(perm.Receivers)
	result += " " + perm.BasePermission.string(seen)
	result += " func"

	if perm.Name != "" {
		result += " " + perm.Name
	}
	result += doTuple(perm.Params)
	result += doTuple(perm.Results)

	return result
}

func (perm *FuncPermission) String() string {
	return perm.string(make(map[Permission]bool))
}

func (perm *InterfacePermission) string(seen map[Permission]bool) string {
	if seen[perm] {
		return "<seen>"
	}

	seen[perm] = true

	methods := make([]string, len(perm.Methods))
	for i, m := range perm.Methods {
		methods[i] = m.string(seen)
	}

	return fmt.Sprintf("%s interface { %s }", perm.BasePermission.string(seen), strings.Join(methods, "; "))
}

func (perm *InterfacePermission) String() string {
	return perm.string(make(map[Permission]bool))
}

func (perm *WildcardPermission) string(seen map[Permission]bool) string {
	return "_"
}

func (perm *WildcardPermission) String() string {
	return perm.string(make(map[Permission]bool))
}

func (perm *StructPermission) string(seen map[Permission]bool) string {
	if seen[perm] {
		return "<seen>"
	}

	seen[perm] = true

	fields := make([]string, len(perm.Fields))
	for i, m := range perm.Fields {
		fields[i] = m.string(seen)
	}

	return fmt.Sprintf("%s struct { %s }", perm.BasePermission.string(seen), strings.Join(fields, "; "))
}

func (perm *StructPermission) String() string {
	return perm.string(make(map[Permission]bool))
}

func (perm *TuplePermission) string(seen map[Permission]bool) string {
	if seen[perm] {
		return "<seen>"
	}

	seen[perm] = true

	elements := make([]string, len(perm.Elements))
	for i, m := range perm.Elements {
		elements[i] = m.string(seen)
	}

	return fmt.Sprintf("%s tuple (%s)", perm.BasePermission.string(seen), strings.Join(elements, "; "))
}

func (perm *TuplePermission) String() string {
	return perm.string(make(map[Permission]bool))
}
