package compiler

import (
	"fmt"
	"strings"
)

// GenerateIRCode currently delegates to the legacy backend while the IR emitter
// is implemented incrementally. This keeps backend selection and parity harness
// wiring stable across migration milestones.
func GenerateIRCode(program *Program) (string, error) {
	ir := LowerToIR(program)
	if errs := ValidateIR(ir); len(errs) > 0 {
		return "", fmt.Errorf("ir validation failed:\n  %s", strings.Join(errs, "\n  "))
	}

	// Temporary backend bridge: reuse proven legacy Go emitter while IR backend
	// matures. The rewrite now has an explicit AST->IR->backend pipeline shape.
	return GenerateNativeCode(ir.LegacyAST)
}
