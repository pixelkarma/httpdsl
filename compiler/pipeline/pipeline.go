package pipeline

import (
	"fmt"
	front "httpdsl/compiler"
	"httpdsl/compiler/backend/goemit"
	"httpdsl/compiler/frontend"
	"httpdsl/compiler/ir"
	"strings"
)

type GeneratedFile struct {
	Name    string
	Content string
}

type GeneratedPackage struct {
	Files []GeneratedFile
}

// CombinedSource returns all generated files concatenated in deterministic order.
func (p *GeneratedPackage) CombinedSource() string {
	if p == nil || len(p.Files) == 0 {
		return ""
	}
	parts := make([]string, 0, len(p.Files))
	for _, f := range p.Files {
		parts = append(parts, f.Content)
	}
	return strings.Join(parts, "\n\n")
}

// GenerateCode compiles a validated frontend program through the current backend pipeline.
func GenerateCode(program *front.Program) (string, error) {
	core, err := generateCore(program)
	if err != nil {
		return "", err
	}
	// Keep `emit` output as a single, buildable source file.
	return core + "\n\nfunc main() {\n\trunGeneratedMain()\n}\n", nil
}

// GeneratePackage compiles a frontend program into multiple Go source files.
func GeneratePackage(program *front.Program) (*GeneratedPackage, error) {
	core, err := generateCore(program)
	if err != nil {
		return nil, err
	}
	return &GeneratedPackage{
		Files: []GeneratedFile{
			{Name: "gen_core.go", Content: core},
			{Name: "main.go", Content: "package main\n\nfunc main() {\n\trunGeneratedMain()\n}\n"},
		},
	}, nil
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

func generateCore(program *front.Program) (string, error) {
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
