// (C) 2017 Julian Andres Klode <jak@jak-linux.org>
// Licensed under the 2-Clause BSD license, see LICENSE for more information.

package permission

import (
	"errors"
	"strings"
	"testing"
)

func TestTokenTypeString(t *testing.T) {
	var foo TokenType = -1
	s := foo.String()

	if !strings.Contains(s, "unknown token type") {
		t.Errorf("Unknown token type produced %s expected error", s)
	}
}

func TestScannerError(t *testing.T) {
	err := scannerError{error: errors.New("testerror"), offset: 42}
	s := err.Error()

	if !strings.Contains(s, "testerror") {
		t.Errorf("expected %s to contain testerror", s)
	}
	if !strings.Contains(s, "42") {
		t.Errorf("expected %s to contain 42", s)
	}
}
