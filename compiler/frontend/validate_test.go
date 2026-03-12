package frontend

import (
	"strings"
	"testing"

	front "httpdsl/compiler"
)

func TestValidateTopLevelAllowsSingleServer(t *testing.T) {
	program := &front.Program{
		Statements: []front.Statement{
			&front.ServerStatement{Token: front.Token{Line: 1, Column: 1}},
			&front.InitStatement{Token: front.Token{Line: 5, Column: 1}},
			&front.RouteStatement{Token: front.Token{Line: 9, Column: 1}, Method: "GET", Path: "/"},
		},
	}
	if err := ValidateTopLevel(program); err != nil {
		t.Fatalf("expected valid top-level program, got error: %v", err)
	}
}

func TestValidateTopLevelRejectsDuplicateServer(t *testing.T) {
	program := &front.Program{
		Statements: []front.Statement{
			&front.ServerStatement{Token: front.Token{Line: 1, Column: 1}},
			&front.ServerStatement{Token: front.Token{Line: 12, Column: 3}},
		},
	}
	err := ValidateTopLevel(program)
	if err == nil {
		t.Fatalf("expected duplicate server validation error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "duplicate server block") {
		t.Fatalf("expected duplicate server message, got: %s", msg)
	}
	if !strings.Contains(msg, "line 12, col 3") {
		t.Fatalf("expected duplicate location in error, got: %s", msg)
	}
	if !strings.Contains(msg, "line 1, col 1") {
		t.Fatalf("expected original server location in error, got: %s", msg)
	}
}

