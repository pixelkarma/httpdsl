package main

import (
	"fmt"
	"httpdsl/compiler"
	"os"
)

func main() {
	// Read our own executable to find the appended program data
	exePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error finding executable: %s\n", err)
		os.Exit(1)
	}

	bin, err := os.ReadFile(exePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading executable: %s\n", err)
		os.Exit(1)
	}

	programData, err := compiler.UnpackProgramData(bin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\nThis binary has no embedded httpdsl program.\n", err)
		os.Exit(1)
	}

	program, err := compiler.DeserializeProgram(programData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error deserializing program: %s\n", err)
		os.Exit(1)
	}

	// Execute using the interpreter + bytecode VM (same as normal httpdsl run)
	interp := compiler.NewInterpreter()
	interp.Execute(program)

	if err := interp.Server.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %s\n", err)
		os.Exit(1)
	}
}
