package compiler

// GenerateIRCode currently delegates to the legacy backend while the IR emitter
// is implemented incrementally. This keeps backend selection and parity harness
// wiring stable across migration milestones.
func GenerateIRCode(program *Program) (string, error) {
	return GenerateNativeCode(program)
}
