package main

import (
	"fmt"
	"httpdsl/compiler"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

const version = "0.1.0"

// ANSI colors
const (
	colorReset  = "\033[0m"
	colorCyan   = "\033[36m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorDim    = "\033[2m"
	colorBold   = "\033[1m"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: httpdsl <command> [path]")
		fmt.Println("Commands:")
		fmt.Println("  run [path]    Compile, run, and watch for changes (requires Go)")
		fmt.Println("  build [path]  Compile to native binary (requires Go)")
		fmt.Println("  emit [path]   Emit generated Go source to stdout")
		fmt.Println("")
		fmt.Println("If no path given, looks for app.httpdsl in current directory")
		fmt.Println("and recursively includes all .httpdsl files.")
		os.Exit(1)
	}

	cmd := os.Args[1]
	var target string

	switch cmd {
	case "emit":
		if len(os.Args) >= 3 {
			target = os.Args[2]
		} else {
			target = resolveDefault()
		}
		program := parseTarget(target)
		backend, err := compiler.BackendFromEnv()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Backend selection error: %s\n", err)
			os.Exit(1)
		}
		src, err := compiler.GenerateCode(program, backend)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Code generation error: %s\n", err)
			os.Exit(1)
		}
		fmt.Print(src)
	case "build":
		if len(os.Args) >= 3 {
			target = os.Args[2]
		} else {
			target = resolveDefault()
		}
		program := parseTarget(target)
		doBuild(program, target)
	case "run":
		if len(os.Args) >= 3 {
			target = os.Args[2]
		} else {
			target = resolveDefault()
		}
		doRunWatch(target)
	default:
		target = cmd
		doRunWatch(target)
	}
}

// resolveDefault finds the project root by looking for app.httpdsl
func resolveDefault() string {
	if _, err := os.Stat("app.httpdsl"); err == nil {
		return "."
	}
	fmt.Fprintln(os.Stderr, "No app.httpdsl found in current directory.")
	fmt.Fprintln(os.Stderr, "Create app.httpdsl or specify a path: httpdsl build <path>")
	os.Exit(1)
	return ""
}

func parseTarget(target string) *compiler.Program {
	var sources []string
	info, err := os.Stat(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}

	if info.IsDir() {
		appFile := filepath.Join(target, "app.httpdsl")
		if _, err := os.Stat(appFile); err != nil {
			fmt.Fprintf(os.Stderr, "No app.httpdsl found in %s\n", target)
			fmt.Fprintln(os.Stderr, "Create app.httpdsl with your server {} block.")
			os.Exit(1)
		}
		sources = append(sources, appFile)
		filepath.Walk(target, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if fi.IsDir() {
				return nil
			}
			if !strings.HasSuffix(fi.Name(), ".httpdsl") {
				return nil
			}
			abs, _ := filepath.Abs(path)
			appAbs, _ := filepath.Abs(appFile)
			if abs != appAbs {
				sources = append(sources, path)
			}
			return nil
		})
	} else {
		dir := filepath.Dir(target)
		base := filepath.Base(target)
		if base == "app.httpdsl" {
			return parseTarget(dir)
		}
		sources = []string{target}
	}

	if len(sources) == 0 {
		fmt.Fprintf(os.Stderr, "No .httpdsl files found\n")
		os.Exit(1)
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
		if errs := validateTopLevelStatements(fileProgram); len(errs) > 0 {
			fmt.Fprintf(os.Stderr, "Validation errors in %s:\n", src)
			for _, e := range errs {
				fmt.Fprintf(os.Stderr, "  %s\n", e)
			}
			os.Exit(1)
		}
		program.Statements = append(program.Statements, fileProgram.Statements...)
	}
	return program
}

// parseTargetSoft is like parseTarget but returns an error instead of exiting
func parseTargetSoft(target string) (*compiler.Program, error) {
	var sources []string
	info, err := os.Stat(target)
	if err != nil {
		return nil, err
	}

	if info.IsDir() {
		appFile := filepath.Join(target, "app.httpdsl")
		if _, err := os.Stat(appFile); err != nil {
			return nil, fmt.Errorf("no app.httpdsl found in %s", target)
		}
		sources = append(sources, appFile)
		filepath.Walk(target, func(path string, fi os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if fi.IsDir() {
				return nil
			}
			if !strings.HasSuffix(fi.Name(), ".httpdsl") {
				return nil
			}
			abs, _ := filepath.Abs(path)
			appAbs, _ := filepath.Abs(appFile)
			if abs != appAbs {
				sources = append(sources, path)
			}
			return nil
		})
	} else {
		base := filepath.Base(target)
		if base == "app.httpdsl" {
			return parseTargetSoft(filepath.Dir(target))
		}
		sources = []string{target}
	}

	if len(sources) == 0 {
		return nil, fmt.Errorf("no .httpdsl files found")
	}

	program := &compiler.Program{}
	for _, src := range sources {
		data, err := os.ReadFile(src)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", src, err)
		}
		l := compiler.NewLexer(string(data))
		p := compiler.NewParser(l)
		fileProgram := p.ParseProgram()
		if errs := p.Errors(); len(errs) > 0 {
			var msgs []string
			for _, e := range errs {
				msgs = append(msgs, e)
			}
			return nil, fmt.Errorf("parse errors in %s:\n  %s", src, strings.Join(msgs, "\n  "))
		}
		if errs := validateTopLevelStatements(fileProgram); len(errs) > 0 {
			return nil, fmt.Errorf("validation errors in %s:\n  %s", src, strings.Join(errs, "\n  "))
		}
		program.Statements = append(program.Statements, fileProgram.Statements...)
	}
	return program, nil
}

func validateTopLevelStatements(program *compiler.Program) []string {
	var errs []string
	for _, stmt := range program.Statements {
		switch stmt.(type) {
		case *compiler.RouteStatement,
			*compiler.FnStatement,
			*compiler.ServerStatement,
			*compiler.GroupStatement,
			*compiler.BeforeStatement,
			*compiler.AfterStatement,
			*compiler.InitStatement,
			*compiler.ShutdownStatement,
			*compiler.HelpStatement,
			*compiler.ErrorStatement,
			*compiler.EveryStatement:
			continue
		default:
			line, col := statementLocation(stmt)
			switch stmt.(type) {
			case *compiler.AssignStatement,
				*compiler.CompoundAssignStatement,
				*compiler.IndexAssignStatement,
				*compiler.ObjectDestructureStatement,
				*compiler.ArrayDestructureStatement:
				errs = append(errs, fmt.Sprintf("line %d, col %d: top-level assignments are not allowed; move this into init {}", line, col))
			default:
				errs = append(errs, fmt.Sprintf("line %d, col %d: invalid top-level statement; only blocks are allowed", line, col))
			}
		}
	}
	return errs
}

func statementLocation(stmt compiler.Statement) (int, int) {
	switch s := stmt.(type) {
	case *compiler.RouteStatement:
		return s.Token.Line, s.Token.Column
	case *compiler.FnStatement:
		return s.Token.Line, s.Token.Column
	case *compiler.ServerStatement:
		return s.Token.Line, s.Token.Column
	case *compiler.GroupStatement:
		return s.Token.Line, s.Token.Column
	case *compiler.BeforeStatement:
		return s.Token.Line, s.Token.Column
	case *compiler.AfterStatement:
		return s.Token.Line, s.Token.Column
	case *compiler.InitStatement:
		return s.Token.Line, s.Token.Column
	case *compiler.ShutdownStatement:
		return s.Token.Line, s.Token.Column
	case *compiler.HelpStatement:
		return s.Token.Line, s.Token.Column
	case *compiler.ErrorStatement:
		return s.Token.Line, s.Token.Column
	case *compiler.EveryStatement:
		return s.Token.Line, s.Token.Column
	case *compiler.AssignStatement:
		return s.Token.Line, s.Token.Column
	case *compiler.CompoundAssignStatement:
		return s.Token.Line, s.Token.Column
	case *compiler.IndexAssignStatement:
		return s.Token.Line, s.Token.Column
	case *compiler.ExpressionStatement:
		return s.Token.Line, s.Token.Column
	case *compiler.ObjectDestructureStatement:
		return s.Token.Line, s.Token.Column
	case *compiler.ArrayDestructureStatement:
		return s.Token.Line, s.Token.Column
	default:
		return 0, 0
	}
}

// extractPort reads the port from a parsed program's server block
func extractPort(program *compiler.Program) int {
	for _, stmt := range program.Statements {
		if ss, ok := stmt.(*compiler.ServerStatement); ok {
			if pe, ok := ss.Settings["port"]; ok {
				if lit, ok := pe.(*compiler.IntegerLiteral); ok {
					return int(lit.Value)
				}
			}
		}
	}
	return 8080
}

// extractSSL checks if SSL is configured
func extractSSL(program *compiler.Program) bool {
	for _, stmt := range program.Statements {
		if ss, ok := stmt.(*compiler.ServerStatement); ok {
			_, hasCert := ss.Settings["ssl_cert"]
			_, hasKey := ss.Settings["ssl_key"]
			if hasCert && hasKey {
				return true
			}
		}
	}
	return false
}

func printBanner(port int, ssl bool, buildTime time.Duration, watchDir string) {
	protocol := "http"
	if ssl {
		protocol = "https"
	}
	fmt.Println()
	fmt.Printf("  %shttpdsl%s %sv%s%s\n", colorBold+colorCyan, colorReset, colorDim, version, colorReset)
	fmt.Println()
	fmt.Printf("  %s➜%s  %sServer:%s   %s%s://localhost:%d/%s\n", colorGreen, colorReset, colorBold, colorReset, colorCyan, protocol, port, colorReset)
	fmt.Printf("  %s➜%s  %sBuilt in:%s  %s\n", colorGreen, colorReset, colorBold, colorReset, buildTime.Round(time.Millisecond))
	if watchDir != "" {
		fmt.Printf("  %s➜%s  %sWatching:%s  %s\n", colorGreen, colorReset, colorBold, colorReset, watchDir)
	}
	fmt.Println()
}

func printRebuild(changedFiles []string, watchDir string) {
	fmt.Printf("  %s[watch]%s %d file%s changed:\n", colorYellow, colorReset, len(changedFiles), plural(len(changedFiles)))
	for _, f := range changedFiles {
		rel, err := filepath.Rel(watchDir, f)
		if err != nil {
			rel = f
		}
		fmt.Printf("    %smodified%s  %s\n", colorDim, colorReset, rel)
	}
	fmt.Println()
	fmt.Printf("  %s➜%s  Rebuilding...\n", colorYellow, colorReset)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// doRunWatch compiles, runs, watches for changes, and restarts
func doRunWatch(target string) {
	// Determine watch directory
	watchDir := target
	info, err := os.Stat(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
	if !info.IsDir() {
		watchDir = filepath.Dir(target)
	}
	watchDir, _ = filepath.Abs(watchDir)

	// Temp directory for the binary
	tmpDir, err := os.MkdirTemp("", "httpdsl-run-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)
	binPath := filepath.Join(tmpDir, "server")

	// Initial build
	program, buildTime, buildErr := buildSoft(target, binPath)
	if buildErr != nil {
		fmt.Fprintf(os.Stderr, "\n  %sBuild error:%s %s\n\n", colorRed, colorReset, buildErr)
		fmt.Printf("  %s[watch]%s waiting for changes...\n\n", colorYellow, colorReset)
	} else {
		port := extractPort(program)
		ssl := extractSSL(program)
		printBanner(port, ssl, buildTime, watchDir)
	}

	// Start the server process
	var proc *os.Process
	var procMu sync.Mutex

	startServer := func() {
		procMu.Lock()
		defer procMu.Unlock()
		cmd := exec.Command(binPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Start(); err != nil {
			fmt.Fprintf(os.Stderr, "  %sError starting server:%s %s\n", colorRed, colorReset, err)
			return
		}
		proc = cmd.Process
		go cmd.Wait() // reap the child
	}

	stopServer := func() {
		procMu.Lock()
		defer procMu.Unlock()
		if proc != nil {
			proc.Signal(syscall.SIGTERM)
			// Wait briefly for graceful shutdown
			done := make(chan struct{})
			go func() {
				proc.Wait()
				close(done)
			}()
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				proc.Kill()
			}
			proc = nil
		}
	}

	if buildErr == nil {
		startServer()
	}

	// Set up file watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating watcher: %s\n", err)
		os.Exit(1)
	}
	defer watcher.Close()

	// Recursively add directories to watch
	addWatchDirs(watcher, watchDir, tmpDir)

	// Handle Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Debounce timer and changed file tracking
	var debounceTimer *time.Timer
	var changedFiles []string
	var changeMu sync.Mutex
	var building sync.Mutex
	var rebuildPending bool

	var rebuild func()
	rebuild = func() {
		// If already building, mark pending and return — the current build
		// will check for pending changes when it finishes.
		if !building.TryLock() {
			changeMu.Lock()
			rebuildPending = true
			changeMu.Unlock()
			return
		}
		defer building.Unlock()

		for {
			changeMu.Lock()
			files := changedFiles
			changedFiles = nil
			rebuildPending = false
			changeMu.Unlock()

			if len(files) == 0 {
				return
			}

			// Deduplicate
			seen := make(map[string]bool)
			var unique []string
			for _, f := range files {
				if !seen[f] {
					seen[f] = true
					unique = append(unique, f)
				}
			}

			printRebuild(unique, watchDir)

			stopServer()

			p, bt, err := buildSoft(target, binPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "\n  %sBuild error:%s %s\n\n", colorRed, colorReset, err)
				fmt.Printf("  %s[watch]%s waiting for changes...\n\n", colorYellow, colorReset)
			} else {
				port := extractPort(p)
				ssl := extractSSL(p)
				protocol := "http"
				if ssl {
					protocol = "https"
				}

				fmt.Printf("  %s➜%s  %sServer:%s   %s%s://localhost:%d/%s\n", colorGreen, colorReset, colorBold, colorReset, colorCyan, protocol, port, colorReset)
				fmt.Printf("  %s➜%s  %sBuilt in:%s  %s\n\n", colorGreen, colorReset, colorBold, colorReset, bt.Round(time.Millisecond))

				startServer()

				// Re-add any new directories that may have appeared
				addWatchDirs(watcher, watchDir, tmpDir)
			}

			// Check if more changes arrived while we were building
			changeMu.Lock()
			pending := rebuildPending || len(changedFiles) > 0
			changeMu.Unlock()
			if !pending {
				return
			}
			// Loop around to pick up the new changes
		}
	}

	for {
		select {
		case <-sigCh:
			fmt.Printf("\n  %s[watch]%s shutting down...\n", colorYellow, colorReset)
			stopServer()
			return

		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			// Only care about writes, creates, removes, renames
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}
			// Skip hidden files and temp files
			base := filepath.Base(event.Name)
			if strings.HasPrefix(base, ".") || strings.HasSuffix(base, "~") || strings.HasSuffix(base, ".swp") {
				continue
			}

			// If a new directory was created, watch it
			if event.Op&fsnotify.Create != 0 {
				if fi, err := os.Stat(event.Name); err == nil && fi.IsDir() {
					addWatchDirs(watcher, event.Name, tmpDir)
				}
			}

			changeMu.Lock()
			changedFiles = append(changedFiles, event.Name)
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(500*time.Millisecond, rebuild)
			changeMu.Unlock()

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			fmt.Fprintf(os.Stderr, "  %s[watch] error:%s %s\n", colorRed, colorReset, err)
		}
	}
}

// addWatchDirs recursively adds directories to the watcher, skipping hidden dirs and tmpDir
func addWatchDirs(watcher *fsnotify.Watcher, root string, tmpDir string) {
	filepath.Walk(root, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !fi.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		// Skip hidden directories, node_modules, and temp build dir
		if strings.HasPrefix(base, ".") && path != root {
			return filepath.SkipDir
		}
		if base == "node_modules" || base == "vendor" {
			return filepath.SkipDir
		}
		abs, _ := filepath.Abs(path)
		tmpAbs, _ := filepath.Abs(tmpDir)
		if strings.HasPrefix(abs, tmpAbs) {
			return filepath.SkipDir
		}
		watcher.Add(path)
		return nil
	})
}

// buildSoft compiles the target, returning the program, build duration, and any error
func buildSoft(target string, binPath string) (*compiler.Program, time.Duration, error) {
	start := time.Now()

	program, err := parseTargetSoft(target)
	if err != nil {
		return nil, 0, err
	}

	backend, err := compiler.BackendFromEnv()
	if err != nil {
		return nil, 0, err
	}

	src, err := compiler.GenerateCode(program, backend)
	if err != nil {
		return nil, 0, fmt.Errorf("code generation: %w", err)
	}

	buildDir, err := os.MkdirTemp("", "httpdsl-build-*")
	if err != nil {
		return nil, 0, fmt.Errorf("creating build dir: %w", err)
	}
	defer os.RemoveAll(buildDir)

	if err := os.WriteFile(filepath.Join(buildDir, "main.go"), []byte(src), 0644); err != nil {
		return nil, 0, fmt.Errorf("writing source: %w", err)
	}

	goMod := "module httpdsl-app\n\ngo 1.24.0\n"
	var requires []string
	if strings.Contains(src, "golang.org/x/crypto/") {
		requires = append(requires, "golang.org/x/crypto v0.48.0")
	}
	drivers := compiler.DetectDBDrivers(program)
	if drivers["sqlite"] {
		requires = append(requires, "modernc.org/sqlite v1.46.1")
	}
	if drivers["postgres"] {
		requires = append(requires, "github.com/jackc/pgx/v5 v5.8.0")
	}
	if drivers["mysql"] {
		requires = append(requires, "github.com/go-sql-driver/mysql v1.9.3")
	}
	if drivers["mongo"] {
		requires = append(requires, "go.mongodb.org/mongo-driver/v2 v2.5.0")
	}
	if len(requires) > 0 {
		goMod += "\nrequire (\n"
		for _, r := range requires {
			goMod += "\t" + r + "\n"
		}
		goMod += ")\n"
	}
	if err := os.WriteFile(filepath.Join(buildDir, "go.mod"), []byte(goMod), 0644); err != nil {
		return nil, 0, fmt.Errorf("writing go.mod: %w", err)
	}
	if len(requires) > 0 {
		tidyCmd := exec.Command("go", "mod", "tidy")
		tidyCmd.Dir = buildDir
		if out, err := tidyCmd.CombinedOutput(); err != nil {
			return nil, 0, fmt.Errorf("go mod tidy: %s\n%s", err, out)
		}
	}

	buildCmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", binPath, ".")
	buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	buildCmd.Dir = buildDir
	if out, err := buildCmd.CombinedOutput(); err != nil {
		// Save source for debugging
		debugPath := filepath.Join(os.TempDir(), "httpdsl-debug.go")
		os.WriteFile(debugPath, []byte(src), 0644)
		return nil, 0, fmt.Errorf("build failed (source saved: %s):\n%s", debugPath, out)
	}

	return program, time.Since(start), nil
}

func doBuild(program *compiler.Program, target string, outputOverride ...string) {
	backend, err := compiler.BackendFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Backend selection error: %s\n", err)
		os.Exit(1)
	}

	src, err := compiler.GenerateCode(program, backend)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Code generation error: %s\n", err)
		os.Exit(1)
	}

	buildDir, err := os.MkdirTemp("", "httpdsl-build-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating build dir: %s\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(buildDir)

	if err := os.WriteFile(filepath.Join(buildDir, "main.go"), []byte(src), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing source: %s\n", err)
		os.Exit(1)
	}

	goMod := "module httpdsl-app\n\ngo 1.24.0\n"
	var requires []string
	if strings.Contains(src, "golang.org/x/crypto/") {
		requires = append(requires, "golang.org/x/crypto v0.48.0")
	}
	drivers := compiler.DetectDBDrivers(program)
	if drivers["sqlite"] {
		requires = append(requires, "modernc.org/sqlite v1.46.1")
	}
	if drivers["postgres"] {
		requires = append(requires, "github.com/jackc/pgx/v5 v5.8.0")
	}
	if drivers["mysql"] {
		requires = append(requires, "github.com/go-sql-driver/mysql v1.9.3")
	}
	if drivers["mongo"] {
		requires = append(requires, "go.mongodb.org/mongo-driver/v2 v2.5.0")
	}
	if len(requires) > 0 {
		goMod += "\nrequire (\n"
		for _, r := range requires {
			goMod += "\t" + r + "\n"
		}
		goMod += ")\n"
	}
	if err := os.WriteFile(filepath.Join(buildDir, "go.mod"), []byte(goMod), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing go.mod: %s\n", err)
		os.Exit(1)
	}
	if len(requires) > 0 {
		tidyCmd := exec.Command("go", "mod", "tidy")
		tidyCmd.Dir = buildDir
		tidyCmd.Stderr = os.Stderr
		if err := tidyCmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "go mod tidy failed: %s\n", err)
			os.Exit(1)
		}
	}

	var outputPath string
	if len(outputOverride) > 0 && outputOverride[0] != "" {
		outputPath = outputOverride[0]
	} else {
		base := strings.TrimSuffix(filepath.Base(target), ".httpdsl")
		if base == "" || base == "." {
			wd, _ := filepath.Abs(target)
			base = filepath.Base(wd)
		}
		if base == "app" {
			wd, _ := filepath.Abs(filepath.Dir(target))
			base = filepath.Base(wd)
		}
		outputPath, _ = filepath.Abs(base)
		if info, err := os.Stat(outputPath); err == nil && info.IsDir() {
			outputPath = outputPath + "-server"
		}
	}

	buildCmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", outputPath, ".")
	buildCmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	buildCmd.Dir = buildDir
	buildCmd.Stderr = os.Stderr
	if err := buildCmd.Run(); err != nil {
		debugPath := filepath.Join(os.TempDir(), filepath.Base(outputPath)+".go")
		os.WriteFile(debugPath, []byte(src), 0644)
		fmt.Fprintf(os.Stderr, "Build failed. Source saved: %s\n", debugPath)
		os.Exit(1)
	}

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
