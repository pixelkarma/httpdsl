package main

import (
	_ "embed"
	"fmt"
	"httpdsl/compiler"
	"os"
	"path/filepath"
	"strings"
)

//go:embed runtime.bin
var runtimeBin []byte

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: httpdsl <command> <file.httpdsl or directory>")
		fmt.Println("Commands:")
		fmt.Println("  run <file>    Run with bytecode VM (default)")
		fmt.Println("  build <file>  Compile to native binary")
		os.Exit(1)
	}

	cmd := os.Args[1]
	var target string

	switch cmd {
	case "build":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: httpdsl build <file.httpdsl>")
			os.Exit(1)
		}
		target = os.Args[2]
		program := parseFiles(target)
		doBuild(program, target)
	case "run":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: httpdsl run <file.httpdsl>")
			os.Exit(1)
		}
		target = os.Args[2]
		program := parseFiles(target)
		doRun(program)
	default:
		// Backward compat: httpdsl <file>
		target = cmd
		program := parseFiles(target)
		doRun(program)
	}
}

func parseFiles(target string) *compiler.Program {
	var sources []string
	info, err := os.Stat(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	if info.IsDir() {
		entries, _ := os.ReadDir(target)
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".httpdsl") {
				sources = append(sources, filepath.Join(target, e.Name()))
			}
		}
		if len(sources) == 0 {
			fmt.Fprintf(os.Stderr, "No .httpdsl files found in %s\n", target)
			os.Exit(1)
		}
	} else {
		sources = []string{target}
	}

	program := &compiler.Program{}
	for _, src := range sources {
		data, err := os.ReadFile(src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading %s: %s\n", src, err)
			os.Exit(1)
		}
		l := compiler.NewLexer(string(data))
		p := compiler.NewParser(l)
		fileProgram := p.ParseProgram()
		if errs := p.Errors(); len(errs) > 0 {
			fmt.Fprintf(os.Stderr, "Parse errors in %s:\n", src)
			for _, e := range errs {
				fmt.Fprintf(os.Stderr, "  %s\n", e)
			}
			os.Exit(1)
		}
		program.Statements = append(program.Statements, fileProgram.Statements...)
	}
	return program
}

func doRun(program *compiler.Program) {
	interp := compiler.NewInterpreter()
	interp.Execute(program)
	if err := interp.Server.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %s\n", err)
		os.Exit(1)
	}
}

func doBuild(program *compiler.Program, target string) {
	// Serialize the program
	programData, err := compiler.SerializeProgram(program)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Serialization error: %s\n", err)
		os.Exit(1)
	}

	// Check that we have an embedded runtime
	if len(runtimeBin) == 0 {
		fmt.Fprintln(os.Stderr, "Error: no embedded runtime. Build httpdsl with 'make' first.")
		os.Exit(1)
	}

	// Pack: runtime binary + serialized program
	output := compiler.PackBinary(runtimeBin, programData)

	// Determine output name
	base := strings.TrimSuffix(filepath.Base(target), ".httpdsl")
	if base == "" || base == "." {
		base = "server"
	}
	outputPath, _ := filepath.Abs(base)
	// Avoid collisions with existing directories
	if info, err := os.Stat(outputPath); err == nil && info.IsDir() {
		outputPath = outputPath + "-server"
	}

	// Write the output binary
	if err := os.WriteFile(outputPath, output, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing binary: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("Compiled %s → %s (%d bytes)\n", target, outputPath, len(output))
	fmt.Printf("  Runtime: %d bytes, Program: %d bytes\n", len(runtimeBin), len(programData))
}
