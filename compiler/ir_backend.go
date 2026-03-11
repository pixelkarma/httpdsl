package compiler

import (
	"fmt"
	"strings"
)

// GenerateIRCode compiles through the IR pipeline and emits Go via the dedicated
// IR emitter seam.
func GenerateIRCode(program *Program) (string, error) {
	ir := LowerToIR(program)
	if errs := ValidateIR(ir); len(errs) > 0 {
		return "", fmt.Errorf("ir validation failed:\n  %s", strings.Join(errs, "\n  "))
	}
	if _, err := EmitIRPreview(ir); err != nil {
		return "", fmt.Errorf("ir preview emission failed: %w", err)
	}

	// Backend bridge now routes through the dedicated IR Go emitter path.
	return GenerateGoFromIR(ir)
}
