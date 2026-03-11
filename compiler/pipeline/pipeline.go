package pipeline

import (
	"fmt"
	front "httpdsl/compiler"
	"httpdsl/compiler/backend/goemit"
	"httpdsl/compiler/frontend"
	"httpdsl/compiler/ir"
)

// GenerateCode compiles a validated frontend program through the current backend pipeline.
func GenerateCode(program *front.Program) (string, error) {
	if err := frontend.ValidateTopLevel(program); err != nil {
		return "", err
	}
	irProgram := ir.Lower(program)
	if errs := ir.Validate(irProgram); len(errs) > 0 {
		return "", fmt.Errorf("ir validation failed:\n  %s", joinLines(errs))
	}
	if _, err := ir.EmitPreview(irProgram); err != nil {
		return "", fmt.Errorf("ir preview emission failed: %w", err)
	}
	return goemit.GenerateGoFromIR(program, irProgram.Features.DBDrivers)
}

// DetectDBDrivers returns driver usage from the frontend program.
func DetectDBDrivers(program *front.Program) map[string]bool {
	irProgram := ir.Lower(program)
	out := make(map[string]bool, len(irProgram.Features.DBDrivers))
	for k, v := range irProgram.Features.DBDrivers {
		out[k] = v
	}
	return out
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	out := lines[0]
	for i := 1; i < len(lines); i++ {
		out += "\n  " + lines[i]
	}
	return out
}
