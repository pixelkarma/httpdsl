package main

import (
	"fmt"
	"httpdsl/compiler"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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
	src, err := compiler.GenerateNativeCode(program)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Code generation error: %s\n", err)
		os.Exit(1)
	}

	// Create temp build directory
	buildDir, err := os.MkdirTemp("", "httpdsl-build-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating build dir: %s\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(buildDir)

	// Write generated source
	srcPath := filepath.Join(buildDir, "main.go")
	if err := os.WriteFile(srcPath, []byte(src), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing source: %s\n", err)
		os.Exit(1)
	}

	// Write go.mod
	goMod := "module httpdsl-native\n\ngo 1.24.0\n"
	// Check if bcrypt is needed
	if strings.Contains(src, "golang.org/x/crypto/bcrypt") {
		goMod += "\nrequire golang.org/x/crypto v0.36.0\n"
	}
	if err := os.WriteFile(filepath.Join(buildDir, "go.mod"), []byte(goMod), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing go.mod: %s\n", err)
		os.Exit(1)
	}

	// Determine output name
	base := strings.TrimSuffix(filepath.Base(target), ".httpdsl")
	if base == "" || base == "." {
		base = "server"
	}
	// Output in current directory with -native suffix to avoid collisions
	outputPath, _ := filepath.Abs(base + "-native")

	// Save generated source for inspection next to the binary
	debugSrc := outputPath + ".gen.go"
	os.WriteFile(debugSrc, []byte(src), 0644)
	fmt.Printf("Generated source: %s\n", debugSrc)

	// Run go build
	fmt.Printf("Building native binary...\n")
	buildCmd := exec.Command("go", "build", "-o", outputPath, ".")
	buildCmd.Dir = buildDir
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "\nBuild failed. Generated source saved at: %s\n", debugSrc)
		os.Exit(1)
	}

	fmt.Printf("Native binary: %s\n", outputPath)
}
