package main

import (
	"fmt"
	"httpdsl/compiler"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: httpdsl <file.httpdsl or directory>")
		fmt.Println("  Run a single file or all .httpdsl files in a directory")
		os.Exit(1)
	}

	target := os.Args[1]

	// Gather source files
	var sources []string
	info, err := os.Stat(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	if info.IsDir() {
		// All .httpdsl files in directory
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

	// Parse all files into a single program
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

	// Execute
	interp := compiler.NewInterpreter()
	interp.Execute(program)

	// Start the server
	if err := interp.Server.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %s\n", err)
		os.Exit(1)
	}
}
