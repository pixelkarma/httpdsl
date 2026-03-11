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
	if _, err := EmitIRPreview(ir); err != nil {
		return "", fmt.Errorf("ir preview emission failed: %w", err)
	}

	// Backend bridge now routes through the dedicated IR Go emitter path.
	return GenerateGoFromIR(ir)
}
