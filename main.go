package main

import (
	"fmt"
	"httpdsl/compiler"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: httpdsl <command> <file.httpdsl or directory>")
		fmt.Println("Commands:")
		fmt.Println("  run <file>    Compile and run (requires Go)")
		fmt.Println("  build <file>  Compile to native binary (requires Go)")
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
		doBuild(program, target, "")
	case "run":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Usage: httpdsl run <file.httpdsl>")
			os.Exit(1)
		}
		target = os.Args[2]
		program := parseFiles(target)
		doRun(program, target)
	default:
		target = cmd
		program := parseFiles(target)
		doRun(program, target)
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

func doRun(program *compiler.Program, target string) {
	// Build to temp, then exec
	tmpDir, err := os.MkdirTemp("", "httpdsl-run-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	binPath := filepath.Join(tmpDir, "server")
	doBuild(program, target, binPath)

	// Run the binary, forwarding signals
	proc := exec.Command(binPath)
	proc.Stdout = os.Stdout
	proc.Stderr = os.Stderr
	proc.Stdin = os.Stdin

	if err := proc.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting server: %s\n", err)
		os.Exit(1)
	}

	// Forward SIGINT/SIGTERM
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		proc.Process.Signal(syscall.SIGTERM)
	}()

	if err := proc.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
	}
}

func doBuild(program *compiler.Program, target string, outputPath string) {
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
	if err := os.WriteFile(filepath.Join(buildDir, "main.go"), []byte(src), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing source: %s\n", err)
		os.Exit(1)
	}

	// Write go.mod
	goMod := "module httpdsl-app\n\ngo 1.24.0\n"
	if strings.Contains(src, "golang.org/x/crypto/bcrypt") {
		goMod += "\nrequire golang.org/x/crypto v0.48.0\n"
	}
	if err := os.WriteFile(filepath.Join(buildDir, "go.mod"), []byte(goMod), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing go.mod: %s\n", err)
		os.Exit(1)
	}

	// Determine output path
	if outputPath == "" {
		base := strings.TrimSuffix(filepath.Base(target), ".httpdsl")
		if base == "" || base == "." {
			base = "server"
		}
		outputPath, _ = filepath.Abs(base)
		if info, err := os.Stat(outputPath); err == nil && info.IsDir() {
			outputPath = outputPath + "-server"
		}
	}

	// Build
	buildCmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", outputPath, ".")
	buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	buildCmd.Dir = buildDir
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		// Save source for debugging
		debugPath := outputPath + ".go"
		os.WriteFile(debugPath, []byte(src), 0644)
		fmt.Fprintf(os.Stderr, "Build failed. Source saved: %s\n", debugPath)
		os.Exit(1)
	}

	// Only print when building to a named output (not temp for run)
	if !strings.Contains(outputPath, "httpdsl-run-") {
		fi, _ := os.Stat(outputPath)
		fmt.Printf("Built %s → %s (%s)\n", target, outputPath, humanSize(fi.Size()))
	}
}

func humanSize(b int64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	if b < 1024*1024 {
		return fmt.Sprintf("%.1f KB", float64(b)/1024)
	}
	return fmt.Sprintf("%.1f MB", float64(b)/(1024*1024))
}
