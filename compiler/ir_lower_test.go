package compiler

import "testing"

func TestLowerToIRClassifiesTopLevel(t *testing.T) {
	prog := &Program{Statements: []Statement{
		&ServerStatement{},
		&InitStatement{},
		&FnStatement{Name: "x"},
		&RouteStatement{Method: "GET", Path: "/"},
		&EveryStatement{Interval: 5},
	}}

	ir := LowerToIR(prog)
	if len(ir.TopLevel) != 5 {
		t.Fatalf("expected 5 top-level nodes, got %d", len(ir.TopLevel))
	}

	kinds := []IRTopLevelKind{}
	for _, n := range ir.TopLevel {
		kinds = append(kinds, n.Kind)
	}

	expected := []IRTopLevelKind{
		IRTopLevelServer,
		IRTopLevelInit,
		IRTopLevelFunction,
		IRTopLevelRoute,
		IRTopLevelEvery,
	}
	for i := range expected {
		if kinds[i] != expected[i] {
			t.Fatalf("node %d: expected %s got %s", i, expected[i], kinds[i])
		}
	}
}

func TestValidateIRDetectsDisconnectOnNonSSERoute(t *testing.T) {
	prog := &Program{Statements: []Statement{
		&RouteStatement{
			Token:           Token{Line: 3, Column: 7},
			Method:          "GET",
			Path:            "/bad",
			Body:            &BlockStatement{},
			DisconnectBlock: &BlockStatement{},
		},
	}}
	ir := LowerToIR(prog)
	errs := ValidateIR(ir)
	if len(errs) == 0 {
		t.Fatalf("expected validation error for disconnect on non-SSE route")
	}
}
