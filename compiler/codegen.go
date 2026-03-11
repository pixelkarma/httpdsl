package compiler

import (
	"fmt"
	"os"
	"strings"
)

// Backend selects which code generation pipeline to use.
type Backend string

const (
	BackendLegacy Backend = "legacy"
	BackendIR     Backend = "ir"
)

// ParseBackend parses a backend name.
func ParseBackend(value string) (Backend, error) {
	v := strings.TrimSpace(strings.ToLower(value))
	if v == "" {
		return BackendLegacy, nil
	}
	switch Backend(v) {
	case BackendLegacy, BackendIR:
		return Backend(v), nil
	default:
		return "", fmt.Errorf("unknown backend %q (supported: legacy, ir)", value)
	}
}

// BackendFromEnv reads HTTPDSL_BACKEND and falls back to legacy.
func BackendFromEnv() (Backend, error) {
	return ParseBackend(os.Getenv("HTTPDSL_BACKEND"))
}

// GenerateCode compiles the program to Go source using the requested backend.
func GenerateCode(program *Program, backend Backend) (string, error) {
	switch backend {
	case BackendLegacy:
		return GenerateNativeCode(program)
	case BackendIR:
		return GenerateIRCode(program)
	default:
		return "", fmt.Errorf("unsupported backend %q", backend)
	}
}
