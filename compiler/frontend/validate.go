package frontend

import (
	"fmt"

	front "httpdsl/compiler"
)

// ValidateTopLevel enforces the language rule that only declarations are allowed at top level.
func ValidateTopLevel(program *front.Program) error {
	if program == nil {
		return fmt.Errorf("nil program")
	}
	seenServer := false
	serverLine, serverCol := 0, 0
	for _, stmt := range program.Statements {
		switch stmt.(type) {
		case *front.RouteStatement,
			*front.GroupStatement,
			*front.FnStatement,
			*front.BeforeStatement,
			*front.AfterStatement,
			*front.ErrorStatement,
			*front.EveryStatement,
			*front.InitStatement,
			*front.ShutdownStatement,
			*front.HelpStatement:
			continue
		case *front.ServerStatement:
			line, col := statementLocation(stmt)
			if seenServer {
				return fmt.Errorf(
					"line %d, col %d: duplicate server block; first server block defined at line %d, col %d (only one server {} block is allowed per project)",
					line, col, serverLine, serverCol,
				)
			}
			seenServer = true
			serverLine, serverCol = line, col
			continue
		default:
			line, col := statementLocation(stmt)
			return fmt.Errorf("line %d, col %d: unexpected top-level statement — use init {} for startup code", line, col)
		}
	}
	return nil
}

func statementLocation(stmt front.Statement) (int, int) {
	switch s := stmt.(type) {
	case *front.RouteStatement:
		return s.Token.Line, s.Token.Column
	case *front.FnStatement:
		return s.Token.Line, s.Token.Column
	case *front.ServerStatement:
		return s.Token.Line, s.Token.Column
	case *front.GroupStatement:
		return s.Token.Line, s.Token.Column
	case *front.BeforeStatement:
		return s.Token.Line, s.Token.Column
	case *front.AfterStatement:
		return s.Token.Line, s.Token.Column
	case *front.InitStatement:
		return s.Token.Line, s.Token.Column
	case *front.ShutdownStatement:
		return s.Token.Line, s.Token.Column
	case *front.HelpStatement:
		return s.Token.Line, s.Token.Column
	case *front.ErrorStatement:
		return s.Token.Line, s.Token.Column
	case *front.EveryStatement:
		return s.Token.Line, s.Token.Column
	case *front.AssignStatement:
		return s.Token.Line, s.Token.Column
	case *front.CompoundAssignStatement:
		return s.Token.Line, s.Token.Column
	case *front.IndexAssignStatement:
		return s.Token.Line, s.Token.Column
	case *front.ExpressionStatement:
		return s.Token.Line, s.Token.Column
	case *front.ObjectDestructureStatement:
		return s.Token.Line, s.Token.Column
	case *front.ArrayDestructureStatement:
		return s.Token.Line, s.Token.Column
	default:
		return 0, 0
	}
}
