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
			*front.HelpStatement,
			*front.ServerStatement:
			continue
		default:
			return fmt.Errorf("unexpected top-level statement — use init {} for startup code")
		}
	}
	return nil
}
