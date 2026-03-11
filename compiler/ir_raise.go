package compiler

// RaiseFromIR reconstructs an AST program from IR top-level statements.
// During migration this is intentionally a structural round-trip so backend
// logic can consume an IR-produced view of the program.
func RaiseFromIR(ir *IRProgram) *Program {
	if ir == nil {
		return &Program{}
	}
	out := &Program{Statements: make([]Statement, 0, len(ir.TopLevel))}
	for _, node := range ir.TopLevel {
		if node.Statement != nil {
			out.Statements = append(out.Statements, node.Statement)
		}
	}
	return out
}
