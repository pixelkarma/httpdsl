package ir

import (
	"testing"

	front "httpdsl/compiler"
)

func TestLowerClassifiesTopLevel(t *testing.T) {
	prog := &front.Program{Statements: []front.Statement{
		&front.ServerStatement{},
		&front.InitStatement{},
		&front.FnStatement{Name: "x"},
		&front.RouteStatement{Method: "GET", Path: "/"},
		&front.EveryStatement{Interval: 5},
	}}

	ir := Lower(prog)
	if len(ir.TopLevel) != 5 {
		t.Fatalf("expected 5 top-level nodes, got %d", len(ir.TopLevel))
	}

	kinds := []TopLevelKind{}
	for _, n := range ir.TopLevel {
		kinds = append(kinds, n.Kind)
	}

	expected := []TopLevelKind{
		TopLevelServer,
		TopLevelInit,
		TopLevelFunction,
		TopLevelRoute,
		TopLevelEvery,
	}
	for i := range expected {
		if kinds[i] != expected[i] {
			t.Fatalf("node %d: expected %s got %s", i, expected[i], kinds[i])
		}
	}
}

func TestValidateDetectsDisconnectOnNonSSERoute(t *testing.T) {
	prog := &front.Program{Statements: []front.Statement{
		&front.RouteStatement{
			Token:           front.Token{Line: 3, Column: 7},
			Method:          "GET",
			Path:            "/bad",
			Body:            &front.BlockStatement{},
			DisconnectBlock: &front.BlockStatement{},
		},
	}}
	ir := Lower(prog)
	errs := Validate(ir)
	if len(errs) == 0 {
		t.Fatalf("expected validation error for disconnect on non-SSE route")
	}
}

func TestValidateDetectsUnknownTopLevelKind(t *testing.T) {
	ir := &Program{TopLevel: []TopLevelNode{{Kind: TopLevelUnknown, Line: 2, Column: 5}}}
	errs := Validate(ir)
	if len(errs) == 0 {
		t.Fatalf("expected validation error for unknown top-level node")
	}
}
