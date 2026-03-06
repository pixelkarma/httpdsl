package compiler

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type NativeCompiler struct {
	b            strings.Builder
	indent       int
	port         int
	routes       []*RouteStatement
	groups       []*GroupStatement
	functions    []*FnStatement
	usedBuiltins map[string]bool
	usedImports  map[string]bool
	needsBcrypt      bool
	needsArgon2      bool
	dbDrivers    map[string]bool // "sqlite", "postgres", "mysql", "mongo"
	tmpCounter   int
	typeEnv      *TypeEnv // current function's type info
	fnTypes      map[string]*TypeEnv // per-function type info
	corsOrigins    string // CORS: allowed origins ("*" or comma-separated)
	corsMethods    string // CORS: allowed methods
	corsHeaders    string // CORS: allowed headers
	errorHandlers  []*ErrorStatement
	inRouteHandler bool // true when emitting code inside a route/error handler
	throttleRPS    int  // per-IP requests/second limit; 0 = disabled
	defaultTimeout int  // server-level timeout in seconds; 0 = no timeout
	gzipEnabled    bool // gzip compression
	globalBefore   []*BlockStatement
	globalAfter    []*BlockStatement
	routeBeforeMap map[*RouteStatement][]*BlockStatement // group before blocks per route
	routeAfterMap  map[*RouteStatement][]*BlockStatement // group after blocks per route
	staticMounts   []staticMount // static file serving
	everyBlocks    []*EveryStatement
	initBlocks     []*BlockStatement
	shutdownBlocks []*BlockStatement
	globalVars     map[string]bool // variables declared in init blocks
	sessionEnabled bool
	sessionCookie  string // cookie name, default "sid"
	sessionExpires int    // seconds, default 86400 (24h)
	sessionSecret  string     // HMAC signing secret (literal)
	sessionSecretExpr Expression // secret expression (e.g., env("..."))
	csrfEnabled    bool
	csrfSafeOrigins []string
	templatesDir   string              // path to templates directory
	templateFiles  map[string]string   // name -> content (embedded at compile time)
	hasSSE         bool                // whether any SSE routes exist
	hasCron        bool                // whether any cron expressions are used
	hasExec        bool                // whether exec() builtin is used
}

type staticMount struct {
	Prefix string // URL prefix, e.g. "/assets"
	Dir    string // filesystem directory, e.g. "./public"
}

// DetectDBDrivers returns which database drivers are used in the program
func DetectDBDrivers(program *Program) map[string]bool {
	c := &NativeCompiler{dbDrivers: make(map[string]bool)}
	c.detectDBDrivers(program)
	return c.dbDrivers
}

func GenerateNativeCode(program *Program) (string, error) {
	c := &NativeCompiler{
		port:           8080,
		usedBuiltins:   make(map[string]bool),
		usedImports:    make(map[string]bool),
		dbDrivers:      make(map[string]bool),
		routeBeforeMap: make(map[*RouteStatement][]*BlockStatement),
		routeAfterMap:  make(map[*RouteStatement][]*BlockStatement),
		globalVars:     make(map[string]bool),
	}
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *RouteStatement:
			c.routes = append(c.routes, s)
			c.scanBlock(s.Body)
			if s.Method == "SSE" { c.hasSSE = true }
		case *GroupStatement:
			for _, b := range s.Before { c.scanBlock(b) }
			for _, a := range s.After { c.scanBlock(a) }
			for _, route := range s.Routes {
				c.routes = append(c.routes, route)
				c.scanBlock(route.Body)
				if len(s.Before) > 0 {
					c.routeBeforeMap[route] = s.Before
				}
				if len(s.After) > 0 {
					c.routeAfterMap[route] = s.After
				}
			}
		case *FnStatement:
			c.functions = append(c.functions, s)
			c.scanBlock(s.Body)
		case *BeforeStatement:
			c.globalBefore = append(c.globalBefore, s.Body)
			c.scanBlock(s.Body)
		case *AfterStatement:
			c.globalAfter = append(c.globalAfter, s.Body)
			c.scanBlock(s.Body)
		case *ErrorStatement:
			c.errorHandlers = append(c.errorHandlers, s)
			c.scanBlock(s.Body)
		case *EveryStatement:
			c.everyBlocks = append(c.everyBlocks, s)
			if s.CronExpr != "" { c.hasCron = true }
			c.scanBlock(s.Body)
		case *InitStatement:
			c.initBlocks = append(c.initBlocks, s.Body)
			c.scanBlock(s.Body)
			// Collect variable names assigned in init → package-level globals
			for name := range c.collectVars(s.Body) {
				c.globalVars[name] = true
			}
		case *ShutdownStatement:
			c.shutdownBlocks = append(c.shutdownBlocks, s.Body)
			c.scanBlock(s.Body)
		case *ServerStatement:
			if pe, ok := s.Settings["port"]; ok {
				if lit, ok := pe.(*IntegerLiteral); ok {
					c.port = int(lit.Value)
				}
			}
			if te, ok := s.Settings["throttle_requests_per_second"]; ok {
				if lit, ok := te.(*IntegerLiteral); ok {
					c.throttleRPS = int(lit.Value)
				}
			}
			if te, ok := s.Settings["timeout"]; ok {
				if lit, ok := te.(*IntegerLiteral); ok {
					c.defaultTimeout = int(lit.Value)
				}
			}
			if gz, ok := s.Settings["gzip"]; ok {
				if bl, ok := gz.(*BooleanLiteral); ok && bl.Value {
					c.gzipEnabled = true
					c.usedImports["compress/gzip"] = true
				}
			}
			for _, sm := range s.StaticMounts {
				c.staticMounts = append(c.staticMounts, staticMount{Prefix: sm.Prefix, Dir: sm.Dir})
				c.usedImports["path/filepath"] = true
			}
			// CORS config
			if cors, ok := s.Settings["cors"]; ok {
				if h, ok := cors.(*HashLiteral); ok {
					for _, p := range h.Pairs {
						key := ""
						if sl, ok := p.Key.(*StringLiteral); ok { key = sl.Value }
						val := ""
						if sv, ok := p.Value.(*StringLiteral); ok { val = sv.Value }
						switch key {
						case "origins": c.corsOrigins = val
						case "methods": c.corsMethods = val
						case "headers": c.corsHeaders = val
						}
					}
				}
			}
			// Templates config
			if tpl, ok := s.Settings["templates"]; ok {
				if sv, ok := tpl.(*StringLiteral); ok {
					c.templatesDir = sv.Value
				}
			}
			// Session config
			if sess, ok := s.Settings["session"]; ok {
				c.sessionEnabled = true
				c.sessionCookie = "sid"
				c.sessionExpires = 86400 // 24h default
				if h, ok := sess.(*HashLiteral); ok {
					for _, p := range h.Pairs {
						key := ""
						if sl, ok := p.Key.(*StringLiteral); ok { key = sl.Value }
						switch key {
						case "cookie":
							if sv, ok := p.Value.(*StringLiteral); ok { c.sessionCookie = sv.Value }
						case "expires":
							if iv, ok := p.Value.(*IntegerLiteral); ok { c.sessionExpires = int(iv.Value) }
						case "secret":
							if sv, ok := p.Value.(*StringLiteral); ok {
								c.sessionSecret = sv.Value
							} else {
								c.sessionSecretExpr = p.Value
							}
						case "csrf":
							if id, ok := p.Value.(*Identifier); ok && id.Value == "true" {
								c.csrfEnabled = true
							} else if bl, ok := p.Value.(*BooleanLiteral); ok && bl.Value {
								c.csrfEnabled = true
							}
						case "csrf_safe_origins":
							if arr, ok := p.Value.(*ArrayLiteral); ok {
								for _, el := range arr.Elements {
									if sv, ok := el.(*StringLiteral); ok {
										c.csrfSafeOrigins = append(c.csrfSafeOrigins, sv.Value)
									}
								}
							}
						}
					}
				}
			}
		}
	}
	c.usedImports["context"] = true
	c.usedImports["encoding/json"] = true
	c.usedImports["fmt"] = true
	c.usedImports["io"] = true
	c.usedImports["net/http"] = true
	c.usedImports["strconv"] = true
	c.usedImports["strings"] = true
	c.usedImports["net/url"] = true
	c.usedImports["bytes"] = true
	c.usedImports["os"] = true
	c.usedImports["sync"] = true
	c.usedImports["time"] = true
	// Always needed for always-emitted builtins
	c.usedImports["sort"] = true
	c.usedImports["regexp"] = true
	c.usedImports["math"] = true
	c.usedImports["math/rand"] = true
	c.usedImports["crypto/rand"] = true
	c.usedImports["crypto/sha256"] = true
	c.usedImports["crypto/sha512"] = true
	c.usedImports["crypto/md5"] = true
	c.usedImports["crypto/hmac"] = true
	c.usedImports["encoding/hex"] = true
	c.usedImports["encoding/base64"] = true
	c.usedImports["hash"] = true
	// Import tracking for builtins
	if c.usedBuiltins["env"] {
		c.usedImports["os"] = true
	}
	if c.usedBuiltins["sleep"] || c.usedBuiltins["now"] || c.usedBuiltins["now_ms"] || c.usedBuiltins["date"] || c.usedBuiltins["date_format"] || c.usedBuiltins["date_parse"] || c.usedBuiltins["strtotime"] || c.usedBuiltins["cuid2"] {
		c.usedImports["time"] = true
	}
	if c.usedBuiltins["abs"] || c.usedBuiltins["ceil"] || c.usedBuiltins["floor"] || c.usedBuiltins["round"] {
		c.usedImports["math"] = true
	}
	if c.usedBuiltins["rand"] {
		c.usedImports["math/rand"] = true
	}
	if c.usedBuiltins["uuid"] || c.usedBuiltins["cuid2"] {
		c.usedImports["crypto/rand"] = true
	}
	if c.usedBuiltins["hash"] || c.usedBuiltins["hmac_hash"] || c.usedBuiltins["cuid2"] {
		c.usedImports["crypto/sha256"] = true
	}
	if c.usedBuiltins["hash"] {
		c.usedImports["crypto/sha512"] = true
		c.usedImports["crypto/md5"] = true
	}
	if c.usedBuiltins["hmac_hash"] {
		c.usedImports["crypto/hmac"] = true
		c.usedImports["crypto/sha512"] = true
		c.usedImports["hash"] = true
	}
	if c.usedBuiltins["hash"] || c.usedBuiltins["hmac_hash"] {
		c.usedImports["encoding/hex"] = true
	}
	if c.usedBuiltins["base64_encode"] || c.usedBuiltins["base64_decode"] {
		c.usedImports["encoding/base64"] = true
	}
	if c.usedBuiltins["url_encode"] || c.usedBuiltins["url_decode"] {
		c.usedImports["net/url"] = true
	}
	if c.usedBuiltins["bcrypt_hash"] || c.usedBuiltins["bcrypt_verify"] ||
		c.usedBuiltins["hash_password"] || c.usedBuiltins["verify_password"] {
		c.needsBcrypt = true
	}
	if c.usedBuiltins["hash_password"] || c.usedBuiltins["verify_password"] {
		c.needsArgon2 = true
		c.usedImports["crypto/rand"] = true
		c.usedImports["crypto/subtle"] = true
		c.usedImports["encoding/base64"] = true
		c.usedImports["strings"] = true
		c.usedImports["strconv"] = true
		c.usedImports["fmt"] = true
	}
	if c.usedBuiltins["validate"] || c.usedBuiltins["is_email"] || c.usedBuiltins["is_url"] || c.usedBuiltins["is_uuid"] || c.usedBuiltins["is_numeric"] {
		c.usedImports["regexp"] = true
		c.usedImports["strconv"] = true
		c.usedImports["strings"] = true
	}

	if c.usedBuiltins["file"] {
		c.usedImports["os"] = true
	}
	if c.usedBuiltins["server_stats"] {
		c.usedImports["runtime"] = true
	}
	if c.usedBuiltins["store"] || len(c.shutdownBlocks) > 0 || c.sessionEnabled {
		c.usedImports["os/signal"] = true
		c.usedImports["syscall"] = true
	}
	if c.sessionEnabled && len(c.dbDrivers) == 0 {
		c.usedImports["database/sql"] = true
	}
	if c.usedBuiltins["strtotime"] {
		c.usedImports["strconv"] = true
	}
	if c.usedBuiltins["jwt"] {
		c.usedImports["crypto/hmac"] = true
		c.usedImports["crypto/sha256"] = true
		c.usedImports["crypto/sha512"] = true
		c.usedImports["encoding/base64"] = true
		c.usedImports["hash"] = true
	}
	if c.hasExec {
		c.usedImports["os/exec"] = true
	}

	// Detect database drivers from db.open() calls
	c.detectDBDrivers(program)
	if len(c.dbDrivers) > 0 {
		if c.dbDrivers["sqlite"] || c.dbDrivers["postgres"] || c.dbDrivers["mysql"] {
			c.usedImports["database/sql"] = true
		}
	}

	// Type inference pass
	c.fnTypes = make(map[string]*TypeEnv)
	for _, fn := range c.functions {
		env := InferFunctionTypes(fn.Params, fn.Body, nil)
		c.fnTypes[fn.Name] = env
	}

	// Load template files at compile time
	if c.templatesDir != "" {
		if err := c.loadTemplateFiles(); err != nil {
			return "", fmt.Errorf("templates: %w", err)
		}
		if len(c.templateFiles) > 0 {
			c.usedImports["html/template"] = true
		}
	}

	c.emitHeader()
	c.emitGlobalVars()
	c.emitRuntime()
	c.emitBuiltinFuncs()
	c.emitCronRuntime()
	c.emitDBRuntime()
	c.emitSessionRuntime()
	c.emitTemplateRuntime()
	c.emitSSERuntime()
	c.emitUserFunctions()
	c.emitMain()
	return c.b.String(), nil
}

func (c *NativeCompiler) tmp() string {
	c.tmpCounter++
	return fmt.Sprintf("_t%d", c.tmpCounter)
}

// --- scanning ---
func (c *NativeCompiler) scanBlock(block *BlockStatement) {
	for _, s := range block.Statements {
		c.scanStmt(s)
	}
}

func (c *NativeCompiler) scanStmt(stmt Statement) {
	switch s := stmt.(type) {
	case *ExpressionStatement:
		c.scanExpr(s.Expression)
	case *AssignStatement:
		for _, v := range s.Values { c.scanExpr(v) }
	case *CompoundAssignStatement:
		c.scanExpr(s.Value)
	case *IndexAssignStatement:
		c.scanExpr(s.Left); c.scanExpr(s.Index); c.scanExpr(s.Value)
	case *ReturnStatement:
		for _, v := range s.Values { c.scanExpr(v) }
	case *TryCatchStatement:
		c.scanBlock(s.Try)
		c.scanBlock(s.Catch)
	case *ThrowStatement:
		c.scanExpr(s.Value)
	case *IfStatement:
		c.scanExpr(s.Condition)
		c.scanBlock(s.Consequence)
		if s.Alternative != nil {
			switch alt := s.Alternative.(type) {
			case *BlockStatement: c.scanBlock(alt)
			case *IfStatement: c.scanStmt(alt)
			}
		}
	case *SwitchStatement:
		c.scanExpr(s.Subject)
		for _, cs := range s.Cases {
			for _, v := range cs.Values { c.scanExpr(v) }
			c.scanBlock(cs.Body)
		}
		if s.Default != nil { c.scanBlock(s.Default) }
	case *WhileStatement:
		c.scanExpr(s.Condition); c.scanBlock(s.Body)
	case *EachStatement:
		c.scanExpr(s.Iterable); c.scanBlock(s.Body)
	case *BlockStatement:
		c.scanBlock(s)
	case *FnStatement:
		c.scanBlock(s.Body)
	case *ObjectDestructureStatement:
		c.scanExpr(s.Value)
	case *ArrayDestructureStatement:
		c.scanExpr(s.Value)
	}
}

func (c *NativeCompiler) scanExpr(expr Expression) {
	if expr == nil { return }
	switch e := expr.(type) {
	case *Identifier:
		c.usedBuiltins[e.Value] = true
	case *CallExpression:
		c.scanExpr(e.Function)
		for _, a := range e.Arguments { c.scanExpr(a) }
		// Track exec() as top-level call (not db.exec())
		if id, ok := e.Function.(*Identifier); ok && id.Value == "exec" {
			c.hasExec = true
		}
	case *InfixExpression:
		c.scanExpr(e.Left); c.scanExpr(e.Right)
	case *TernaryExpression:
		c.scanExpr(e.Condition); c.scanExpr(e.Consequence); c.scanExpr(e.Alternative)
	case *PrefixExpression:
		c.scanExpr(e.Right)
	case *IndexExpression:
		c.scanExpr(e.Left); c.scanExpr(e.Index)
	case *DotExpression:
		c.scanExpr(e.Left)
		if id, ok := e.Left.(*Identifier); ok {
			c.usedBuiltins[id.Value] = true
		}
	case *ArrayLiteral:
		for _, el := range e.Elements { c.scanExpr(el) }
	case *HashLiteral:
		for _, p := range e.Pairs { c.scanExpr(p.Key); c.scanExpr(p.Value) }
	case *FunctionLiteral:
		c.scanBlock(e.Body)
	case *AsyncExpression:
		c.scanExpr(e.Expression)
	}
}

// --- emit helpers ---
func (c *NativeCompiler) ln(s string) {
	for i := 0; i < c.indent; i++ { c.b.WriteByte('\t') }
	c.b.WriteString(s)
	c.b.WriteByte('\n')
}

func (c *NativeCompiler) lnf(f string, a ...interface{}) {
	for i := 0; i < c.indent; i++ { c.b.WriteByte('\t') }
	fmt.Fprintf(&c.b, f, a...)
	c.b.WriteByte('\n')
}

func (c *NativeCompiler) raw(s string) { c.b.WriteString(s) }

// detectDBDrivers scans all db.open() calls for the driver string literal
func (c *NativeCompiler) detectDBDrivers(program *Program) {
	for _, stmt := range program.Statements {
		c.detectDBInStmt(stmt)
	}
}

func (c *NativeCompiler) detectDBInStmt(stmt Statement) {
	if stmt == nil { return }
	switch s := stmt.(type) {
	case *AssignStatement:
		for _, v := range s.Values { c.detectDBInExpr(v) }
	case *ExpressionStatement:
		c.detectDBInExpr(s.Expression)
	case *IfStatement:
		c.detectDBInBlock(s.Consequence)
		if alt, ok := s.Alternative.(*BlockStatement); ok { c.detectDBInBlock(alt) }
		if alt, ok := s.Alternative.(*IfStatement); ok { c.detectDBInStmt(alt) }
	case *SwitchStatement:
		for _, cs := range s.Cases { c.detectDBInBlock(cs.Body) }
		if s.Default != nil { c.detectDBInBlock(s.Default) }
	case *WhileStatement:
		c.detectDBInBlock(s.Body)
	case *EachStatement:
		c.detectDBInBlock(s.Body)
	case *BlockStatement:
		c.detectDBInBlock(s)
	case *RouteStatement:
		c.detectDBInBlock(s.Body)
		if s.ElseBlock != nil { c.detectDBInBlock(s.ElseBlock) }
	case *GroupStatement:
		for _, r := range s.Routes { c.detectDBInStmt(r) }
		for _, b := range s.Before { c.detectDBInBlock(b) }
		for _, a := range s.After { c.detectDBInBlock(a) }
	case *BeforeStatement:
		c.detectDBInBlock(s.Body)
	case *AfterStatement:
		c.detectDBInBlock(s.Body)
	case *EveryStatement:
		c.detectDBInBlock(s.Body)
	case *InitStatement:
		c.detectDBInBlock(s.Body)
	case *ShutdownStatement:
		c.detectDBInBlock(s.Body)
	case *FnStatement:
		c.detectDBInBlock(s.Body)
	case *TryCatchStatement:
		c.detectDBInBlock(s.Try)
		c.detectDBInBlock(s.Catch)
	case *ReturnStatement:
		for _, v := range s.Values { c.detectDBInExpr(v) }
	case *ObjectDestructureStatement:
		c.detectDBInExpr(s.Value)
	case *ArrayDestructureStatement:
		c.detectDBInExpr(s.Value)
	}
}

func (c *NativeCompiler) detectDBInBlock(block *BlockStatement) {
	if block == nil { return }
	for _, stmt := range block.Statements { c.detectDBInStmt(stmt) }
}

func (c *NativeCompiler) detectDBInExpr(expr Expression) {
	if expr == nil { return }
	switch e := expr.(type) {
	case *CallExpression:
		if dot, ok := e.Function.(*DotExpression); ok {
			if ident, ok := dot.Left.(*Identifier); ok && ident.Value == "db" && dot.Field == "open" {
				if len(e.Arguments) >= 1 {
					if lit, ok := e.Arguments[0].(*StringLiteral); ok {
						c.dbDrivers[lit.Value] = true
					}
				}
			}
		}
		for _, a := range e.Arguments { c.detectDBInExpr(a) }
	case *InfixExpression:
		c.detectDBInExpr(e.Left); c.detectDBInExpr(e.Right)
	case *TernaryExpression:
		c.detectDBInExpr(e.Condition); c.detectDBInExpr(e.Consequence); c.detectDBInExpr(e.Alternative)
	case *PrefixExpression:
		c.detectDBInExpr(e.Right)
	case *DotExpression:
		c.detectDBInExpr(e.Left)
	case *IndexExpression:
		c.detectDBInExpr(e.Left); c.detectDBInExpr(e.Index)
	case *ArrayLiteral:
		for _, el := range e.Elements { c.detectDBInExpr(el) }
	case *HashLiteral:
		for _, p := range e.Pairs { c.detectDBInExpr(p.Key); c.detectDBInExpr(p.Value) }
	case *AsyncExpression:
		c.detectDBInExpr(e.Expression)
	}
}

func (c *NativeCompiler) emitHeader() {
	c.ln("// Code generated by httpdsl native compiler. DO NOT EDIT.")
	c.ln("package main")
	c.ln("")
	c.ln("import (")
	c.indent++
	stdlib := []string{"bytes", "compress/gzip", "context", "crypto/hmac", "crypto/md5", "crypto/rand",
		"crypto/sha256", "crypto/sha512", "crypto/subtle", "database/sql", "encoding/base64", "encoding/hex", "encoding/json",
		"html/template",
		"fmt", "hash", "io", "math", "math/rand", "net/http", "net/url",
		"os", "os/exec", "os/signal", "path/filepath", "regexp", "runtime", "sort", "strconv", "strings", "sync", "syscall", "time"}
	for _, imp := range stdlib {
		if c.usedImports[imp] {
			switch imp {
			case "math/rand":
				c.lnf("mrand %q", imp)
			case "crypto/rand":
				c.lnf("crand %q", imp)
			case "os/exec":
				c.lnf("osexec %q", imp)
			default:
				c.lnf("%q", imp)
			}
		}
	}
	if c.needsBcrypt || c.needsArgon2 {
		c.ln("")
	}
	if c.needsBcrypt {
		c.lnf("%q", "golang.org/x/crypto/bcrypt")
	}
	if c.needsArgon2 {
		c.lnf("%q", "golang.org/x/crypto/argon2")
	}
	if c.dbDrivers["sqlite"] {
		c.lnf("_ %q", "modernc.org/sqlite")
	}
	if c.dbDrivers["postgres"] {
		c.lnf("_ %q", "github.com/jackc/pgx/v5/stdlib")
	}
	if c.dbDrivers["mysql"] {
		c.lnf("_ %q", "github.com/go-sql-driver/mysql")
	}
	if c.dbDrivers["mongo"] {
		c.ln("")
		c.lnf("%q", "go.mongodb.org/mongo-driver/v2/bson")
		c.lnf("%q", "go.mongodb.org/mongo-driver/v2/mongo")
		c.lnf("%q", "go.mongodb.org/mongo-driver/v2/mongo/options")
	}
	c.indent--
	c.ln(")")
	// Blank identifier for mongo primitive if needed
	if c.dbDrivers["mongo"] {
		c.ln("")
		c.ln("var _ = bson.M{}")
	}
	c.ln("")
}

func (c *NativeCompiler) emitStoreSync() {
	// JSON file sync — always emitted when store is used
	c.raw(`
func storeSyncJSON(filePath string, flushSec int64) {
	// Load existing data from JSON file
	if raw, err := os.ReadFile(filePath); err == nil && len(raw) > 0 {
		var data map[string]interface{}
		if err := json.Unmarshal(raw, &data); err == nil {
			globalStore.mu.Lock()
			for k, v := range data {
				globalStore.data[k] = storeEntry{value: goToValue(v)}
			}
			globalStore.mu.Unlock()
		} else {
			fmt.Fprintf(os.Stderr, "store.sync: failed to parse %s: %v\n", filePath, err)
		}
	}
	globalStore.mu.Lock()
	globalStore.synced = true
	globalStore.mu.Unlock()

	flush := func() {
		globalStore.mu.Lock()
		if len(globalStore.dirtyKeys) == 0 && len(globalStore.deletedKeys) == 0 {
			globalStore.mu.Unlock()
			return
		}
		globalStore.dirtyKeys = make(map[string]bool)
		globalStore.deletedKeys = make(map[string]bool)
		// Snapshot entire store for full-file write
		snapshot := make(map[string]interface{}, len(globalStore.data))
		for k, e := range globalStore.data {
			if !storeExpired(e) {
				snapshot[k] = valueToGo(e.value)
			}
		}
		globalStore.mu.Unlock()

		jBytes, err := json.MarshalIndent(snapshot, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "store.sync json marshal error: %v\n", err)
			return
		}
		if err := os.WriteFile(filePath, jBytes, 0644); err != nil {
			fmt.Fprintf(os.Stderr, "store.sync json write error: %v\n", err)
		}
	}

	go func() {
		ticker := time.NewTicker(time.Duration(flushSec) * time.Second)
		defer ticker.Stop()
		for range ticker.C { flush() }
	}()

	_storeFlush = flush
}
`)

	// SQL DB sync — only emitted when DB drivers are present
	if len(c.dbDrivers) > 0 {
		c.raw(`
func storeSyncDB(args ...Value) {
	dbVal, ok := args[0].(*dslDB)
	if !ok { throw(Value("store.sync: first argument must be a database connection")); return }
	tableName := valueToString(args[1])
	flushSec := int64(5)
	if len(args) >= 3 { flushSec = toInt64(args[2]); if flushSec < 1 { flushSec = 1 } }

	createSQL := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (key TEXT PRIMARY KEY, value TEXT NOT NULL)", tableName)
	if _, err := dbVal.db.Exec(createSQL); err != nil {
		throw(Value("store.sync: create table: " + err.Error())); return
	}

	rows, err := dbVal.db.Query(fmt.Sprintf("SELECT key, value FROM %s", tableName))
	if err != nil { throw(Value("store.sync: load: " + err.Error())); return }
	defer rows.Close()
	globalStore.mu.Lock()
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil { continue }
		var parsed interface{}
		if err := json.Unmarshal([]byte(v), &parsed); err == nil {
			globalStore.data[k] = storeEntry{value: goToValue(parsed)}
		} else {
			globalStore.data[k] = storeEntry{value: Value(v)}
		}
	}
	globalStore.synced = true
	globalStore.mu.Unlock()

	var upsertSQL string
	switch dbVal.driver {
	case "postgres":
		upsertSQL = fmt.Sprintf("INSERT INTO %s (key, value) VALUES ($1, $2) ON CONFLICT (key) DO UPDATE SET value = $2", tableName)
	case "mysql":
`)
		c.ln(`		upsertSQL = fmt.Sprintf("REPLACE INTO %s (` + "`" + `key` + "`" + `, ` + "`" + `value` + "`" + `) VALUES (?, ?)", tableName)`)
		c.raw(`	default:
		upsertSQL = fmt.Sprintf("INSERT OR REPLACE INTO %s (key, value) VALUES (?, ?)", tableName)
	}
	deleteSQL := fmt.Sprintf("DELETE FROM %s WHERE key = ?", tableName)
	if dbVal.driver == "postgres" {
		deleteSQL = fmt.Sprintf("DELETE FROM %s WHERE key = $1", tableName)
	}

	flush := func() {
		globalStore.mu.Lock()
		if len(globalStore.dirtyKeys) == 0 && len(globalStore.deletedKeys) == 0 {
			globalStore.mu.Unlock()
			return
		}
		dirty := make(map[string]Value, len(globalStore.dirtyKeys))
		for k := range globalStore.dirtyKeys {
			if e, ok := globalStore.data[k]; ok { dirty[k] = e.value }
		}
		deleted := make(map[string]bool, len(globalStore.deletedKeys))
		for k := range globalStore.deletedKeys { deleted[k] = true }
		globalStore.dirtyKeys = make(map[string]bool)
		globalStore.deletedKeys = make(map[string]bool)
		globalStore.mu.Unlock()

		tx, err := dbVal.db.Begin()
		if err != nil {
			fmt.Fprintf(os.Stderr, "store.sync flush error: %v\n", err)
			globalStore.mu.Lock()
			for k := range dirty { globalStore.dirtyKeys[k] = true }
			for k := range deleted { globalStore.deletedKeys[k] = true }
			globalStore.mu.Unlock()
			return
		}
		for k, v := range dirty {
			jBytes, _ := json.Marshal(valueToGo(v))
			if _, err := tx.Exec(upsertSQL, k, string(jBytes)); err != nil {
				fmt.Fprintf(os.Stderr, "store.sync upsert error [%s]: %v\n", k, err)
			}
		}
		for k := range deleted {
			if _, err := tx.Exec(deleteSQL, k); err != nil {
				fmt.Fprintf(os.Stderr, "store.sync delete error [%s]: %v\n", k, err)
			}
		}
		if err := tx.Commit(); err != nil {
			fmt.Fprintf(os.Stderr, "store.sync commit error: %v\n", err)
		}
	}

	go func() {
		ticker := time.NewTicker(time.Duration(flushSec) * time.Second)
		defer ticker.Stop()
		for range ticker.C { flush() }
	}()

	_storeFlush = flush
}
`)
	}

	// Dispatcher: detect string (JSON file) vs *dslDB (SQL)
	c.raw(`
func storeSync(args ...Value) Value {
	if len(args) == 0 { throw(Value("store.sync requires a file path or database connection")); return null }
	// JSON file mode: store.sync("path.json") or store.sync("path.json", flushSec)
	if path, ok := args[0].(string); ok {
		flushSec := int64(5)
		if len(args) >= 2 { flushSec = toInt64(args[1]); if flushSec < 1 { flushSec = 1 } }
		storeSyncJSON(path, flushSec)
		return null
	}
`)
	if len(c.dbDrivers) > 0 {
		c.raw(`	// SQL DB mode: store.sync(db, table) or store.sync(db, table, flushSec)
	if len(args) < 2 { throw(Value("store.sync: database mode requires db and table name")); return null }
	storeSyncDB(args...)
	return null
`)
	} else {
		c.raw(`	throw(Value("store.sync: argument must be a file path string or database connection"))
	return null
`)
	}
	c.raw(`}
`)
}

func (c *NativeCompiler) emitGlobalVars() {
	hasGlobals := len(c.globalVars) > 0 || c.usedBuiltins["server_stats"]
	if !hasGlobals {
		return
	}
	c.ln("// ===== Global Variables =====")
	// Sort for deterministic output
	names := make([]string, 0, len(c.globalVars))
	for name := range c.globalVars {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		c.lnf("var %s Value = null", safeIdent(name))
	}
	if c.usedBuiltins["server_stats"] {
		c.ln("var _startTime = time.Now()")
	}
	c.ln("")
}

func (c *NativeCompiler) emitRuntime() {
	c.raw(`// ===== Runtime =====
type Value = interface{}
type nullType struct{}
var null Value = &nullType{}
type multiReturn struct{ Values []Value }

// Async/futures
type dslFuture struct {
	ch       chan Value
	resolved bool
	value    Value
	mu       sync.Mutex
}

func (f *dslFuture) resolve() Value {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.resolved {
		f.value = <-f.ch
		f.resolved = true
	}
	return f.value
}

func resolveValue(v Value) Value {
	if f, ok := v.(*dslFuture); ok {
		return f.resolve()
	}
	return v
}

type ctxKey int
const paramsKey ctxKey = 0

func getParams(r *http.Request) map[string]Value {
	if v := r.Context().Value(paramsKey); v != nil {
		return v.(map[string]Value)
	}
	return map[string]Value{}
}

func isTruthy(v Value) bool {
	v = resolveValue(v)
	switch val := v.(type) {
	case bool: return val
	case int64: return val != 0
	case float64: return val != 0
	case string: return val != ""
	case *nullType: return false
	case nil: return false
	case []Value: return len(val) > 0
	case map[string]Value: return len(val) > 0
	default: return true
	}
}

func logicalOr(a, b Value) Value {
	if isTruthy(a) { return a }
	return b
}

func logicalAnd(a, b Value) Value {
	if !isTruthy(a) { return a }
	return b
}

func nullCoalesce(a, b Value) Value {
	if a == nil || isNull(a) { return b }
	return a
}

func ternary(cond, a, b Value) Value {
	if isTruthy(cond) { return a }
	return b
}

func isNull(v Value) bool {
	v = resolveValue(v)
	if v == nil { return true }
	_, ok := v.(*nullType)
	return ok
}

func valueToString(v Value) string {
	v = resolveValue(v)
	switch val := v.(type) {
	case string: return val
	case int64: return strconv.FormatInt(val, 10)
	case float64: return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		if val { return "true" }
		return "false"
	case *nullType, nil: return "null"
	default:
		b, _ := json.Marshal(valueToGo(v))
		return string(b)
	}
}

func valueToGo(v Value) interface{} {
	v = resolveValue(v)
	switch val := v.(type) {
	case string: return val
	case int64: return val
	case float64: return val
	case bool: return val
	case *nullType, nil: return nil
	case []Value:
		r := make([]interface{}, len(val))
		for i, x := range val { r[i] = valueToGo(x) }
		return r
	case map[string]Value:
		r := make(map[string]interface{})
		for k, x := range val { r[k] = valueToGo(x) }
		return r
	default: return nil
	}
}

func goToValue(v interface{}) Value {
	switch val := v.(type) {
	case float64:
		if val == float64(int64(val)) { return int64(val) }
		return val
	case string: return val
	case bool: return val
	case nil: return null
	case []interface{}:
		r := make([]Value, len(val))
		for i, x := range val { r[i] = goToValue(x) }
		return r
	case map[string]interface{}:
		r := make(map[string]Value)
		for k, x := range val { r[k] = goToValue(x) }
		return r
	default: return null
	}
}

func addValues(a, b Value) Value {
	a = resolveValue(a); b = resolveValue(b)
	if ai, ok := a.(int64); ok {
		if bi, ok := b.(int64); ok { return ai + bi }
		if bf, ok := b.(float64); ok { return float64(ai) + bf }
	}
	if af, ok := a.(float64); ok {
		if bf, ok := b.(float64); ok { return af + bf }
		if bi, ok := b.(int64); ok { return af + float64(bi) }
	}
	if as, ok := a.(string); ok { return as + valueToString(b) }
	return valueToString(a) + valueToString(b)
}

func subtractValues(a, b Value) Value {
	a = resolveValue(a); b = resolveValue(b)
	if ai, ok := a.(int64); ok {
		if bi, ok := b.(int64); ok { return ai - bi }
		if bf, ok := b.(float64); ok { return float64(ai) - bf }
	}
	if af, ok := a.(float64); ok {
		if bf, ok := b.(float64); ok { return af - bf }
		if bi, ok := b.(int64); ok { return af - float64(bi) }
	}
	return int64(0)
}

func multiplyValues(a, b Value) Value {
	a = resolveValue(a); b = resolveValue(b)
	if ai, ok := a.(int64); ok {
		if bi, ok := b.(int64); ok { return ai * bi }
		if bf, ok := b.(float64); ok { return float64(ai) * bf }
	}
	if af, ok := a.(float64); ok {
		if bf, ok := b.(float64); ok { return af * bf }
		if bi, ok := b.(int64); ok { return af * float64(bi) }
	}
	return int64(0)
}

func divideValues(a, b Value) Value {
	a = resolveValue(a); b = resolveValue(b)
	if ai, ok := a.(int64); ok {
		if bi, ok := b.(int64); ok {
			if bi == 0 { return int64(0) }
			return ai / bi
		}
		if bf, ok := b.(float64); ok {
			if bf == 0 { return float64(0) }
			return float64(ai) / bf
		}
	}
	if af, ok := a.(float64); ok {
		if bf, ok := b.(float64); ok {
			if bf == 0 { return float64(0) }
			return af / bf
		}
		if bi, ok := b.(int64); ok {
			if bi == 0 { return float64(0) }
			return af / float64(bi)
		}
	}
	return int64(0)
}

func modValues(a, b Value) Value {
	if ai, ok := a.(int64); ok {
		if bi, ok := b.(int64); ok && bi != 0 { return ai % bi }
	}
	return int64(0)
}

func valuesEqual(a, b Value) bool {
	a = resolveValue(a)
	b = resolveValue(b)
	switch av := a.(type) {
	case int64:
		if bv, ok := b.(int64); ok { return av == bv }
		if bv, ok := b.(float64); ok { return float64(av) == bv }
		if bv, ok := b.(string); ok { return fmt.Sprintf("%d", av) == bv }
	case float64:
		if bv, ok := b.(float64); ok { return av == bv }
		if bv, ok := b.(int64); ok { return av == float64(bv) }
	case string:
		if bv, ok := b.(string); ok { return av == bv }
		if bv, ok := b.(int64); ok { return av == fmt.Sprintf("%d", bv) }
		if bv, ok := b.(float64); ok { return av == fmt.Sprintf("%g", bv) }
	case bool:
		if bv, ok := b.(bool); ok { return av == bv }
	case *nullType:
		_, ok := b.(*nullType); return ok || b == nil
	case nil:
		if _, ok := b.(*nullType); ok { return true }
		return b == nil
	case []Value:
		bv, ok := b.([]Value)
		if !ok { return false }
		if len(av) != len(bv) { return false }
		for i := range av {
			if !valuesEqual(av[i], bv[i]) { return false }
		}
		return true
	case map[string]Value:
		bv, ok := b.(map[string]Value)
		if !ok { return false }
		if len(av) != len(bv) { return false }
		for k, v := range av {
			bVal, exists := bv[k]
			if !exists || !valuesEqual(v, bVal) { return false }
		}
		return true
	}
	return false
}

func compareLess(a, b Value) bool {
	switch av := a.(type) {
	case int64:
		if bv, ok := b.(int64); ok { return av < bv }
		if bv, ok := b.(float64); ok { return float64(av) < bv }
	case float64:
		if bv, ok := b.(float64); ok { return av < bv }
		if bv, ok := b.(int64); ok { return av < float64(bv) }
	case string:
		if bv, ok := b.(string); ok { return av < bv }
	}
	return false
}

func toInt64(v Value) int64 {
	v = resolveValue(v)
	switch val := v.(type) {
	case int64: return val
	case float64: return int64(val)
	case string:
		if n, err := strconv.ParseInt(val, 10, 64); err == nil { return n }
		if f, err := strconv.ParseFloat(val, 64); err == nil { return int64(f) }
		return 0
	case bool:
		if val { return 1 }
		return 0
	default: return 0
	}
}

func toFloat64v(v Value) float64 {
	v = resolveValue(v)
	switch val := v.(type) {
	case float64: return val
	case int64: return float64(val)
	case string:
		if f, err := strconv.ParseFloat(val, 64); err == nil { return f }
		return 0
	case bool:
		if val { return 1 }
		return 0
	default: return 0
	}
}

func indexValue(obj, idx Value) Value {
	switch o := obj.(type) {
	case []Value:
		if i, ok := idx.(int64); ok && i >= 0 && int(i) < len(o) { return o[i] }
	case map[string]Value:
		if v, ok := o[valueToString(idx)]; ok { return v }
	case string:
		if i, ok := idx.(int64); ok && i >= 0 && int(i) < len(o) { return string(o[i]) }
	}
	return null
}

func dotValue(obj Value, field string) Value {
	obj = resolveValue(obj)
	if m, ok := obj.(map[string]Value); ok {
		if v, ok := m[field]; ok { return v }
	}
	return null
}

func setIndex(obj, idx, val Value) {
	obj = resolveValue(obj)
	switch o := obj.(type) {
	case map[string]Value: o[valueToString(idx)] = val
	case []Value:
		if i, ok := idx.(int64); ok && i >= 0 && int(i) < len(o) { o[i] = val }
	}
}

func appendValue(arr, val Value) Value {
	if a, ok := arr.([]Value); ok { return append(a, val) }
	return arr
}

// Router
type routeEntry struct {
	method   string
	segments []routeSeg
	handler  http.HandlerFunc
}
type routeSeg struct {
	literal  string
	param    string
	wildcard string // *param — matches rest of path
}
type staticMountEntry struct {
	prefix  string
	handler http.Handler
}

type dslRouter struct {
	routes        []routeEntry
	errorHandlers map[int]http.HandlerFunc
	limiter       *rateLimiter
	statics       []staticMountEntry
}

func (rt *dslRouter) add(method, pattern string, h http.HandlerFunc) {
	parts := strings.Split(strings.Trim(pattern, "/"), "/")
	segs := make([]routeSeg, len(parts))
	for i, p := range parts {
		if strings.HasPrefix(p, "*") {
			segs[i] = routeSeg{wildcard: p[1:]}
		} else if strings.HasPrefix(p, ":") {
			segs[i] = routeSeg{param: p[1:]}
		} else {
			segs[i] = routeSeg{literal: p}
		}
	}
	rt.routes = append(rt.routes, routeEntry{method: method, segments: segs, handler: h})
}

func (rt *dslRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Rate limiting — checked before anything else
	if rt.limiter != nil {
		ip := r.RemoteAddr
		if idx := strings.LastIndex(ip, ":"); idx != -1 { ip = ip[:idx] }
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ip = strings.TrimSpace(strings.Split(xff, ",")[0])
		}
		if !rt.limiter.allow(ip) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(429)
			w.Write([]byte("{\"error\":\"too many requests\"}"))
			return
		}
	}
	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	for _, route := range rt.routes {
		if route.method != r.Method { continue }
		// Check for wildcard routes (can match more segments)
		hasWildcard := len(route.segments) > 0 && route.segments[len(route.segments)-1].wildcard != ""
		if !hasWildcard && len(parts) != len(route.segments) { continue }
		if hasWildcard && len(parts) < len(route.segments)-1 { continue }
		ok := true
		for i, seg := range route.segments {
			if seg.wildcard != "" { break } // wildcard matches rest
			if i >= len(parts) { ok = false; break }
			if seg.param == "" && seg.literal != parts[i] { ok = false; break }
		}
		if !ok { continue }
		params := make(map[string]Value)
		for i, seg := range route.segments {
			if seg.wildcard != "" {
				params[seg.wildcard] = Value(strings.Join(parts[i:], "/"))
				break
			}
			if seg.param != "" { params[seg.param] = Value(parts[i]) }
		}
		ctx := context.WithValue(r.Context(), paramsKey, params)
		route.handler(w, r.WithContext(ctx))
		return
	}
	// Static file mounts
	for _, s := range rt.statics {
		if strings.HasPrefix(r.URL.Path, s.prefix) {
			s.handler.ServeHTTP(w, r)
			return
		}
	}
	if h, ok := rt.errorHandlers[404]; ok {
		h(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(404)
	w.Write([]byte("{\"error\":\"not found\"}"))
}

// Per-IP rate limiter (token bucket)
type ipBucket struct {
	tokens float64
	last   time.Time
}

type rateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*ipBucket
	rate    float64 // tokens per second
	burst   float64 // max tokens (= rate, so 1 second burst)
}

func newRateLimiter(rps int) *rateLimiter {
	rl := &rateLimiter{
		buckets: make(map[string]*ipBucket),
		rate:    float64(rps),
		burst:   float64(rps),
	}
	// Cleanup stale entries every 60s
	go func() {
		for {
			time.Sleep(60 * time.Second)
			rl.mu.Lock()
			now := time.Now()
			for ip, b := range rl.buckets {
				if now.Sub(b.last) > 5*time.Minute {
					delete(rl.buckets, ip)
				}
			}
			rl.mu.Unlock()
		}
	}()
	return rl
}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	b, ok := rl.buckets[ip]
	if !ok {
		rl.buckets[ip] = &ipBucket{tokens: rl.burst - 1, last: now}
		return true
	}
	elapsed := now.Sub(b.last).Seconds()
	b.tokens += elapsed * rl.rate
	if b.tokens > rl.burst { b.tokens = rl.burst }
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Throw support
type throwValue struct {
	value Value
}

func throw(v Value) {
	panic(&throwValue{value: v})
}

func callValue(fn Value, args ...Value) Value {
	fn = resolveValue(fn)
	if f, ok := fn.(func(...Value) Value); ok {
		return f(args...)
	}
	panic(&throwValue{value: "not a function"})
}

// Response writer
func writeResponse(_w http.ResponseWriter, resp Value) {
	rm, ok := resp.(map[string]Value)
	if !ok {
		_w.WriteHeader(500)
		return
	}
	// Apply response headers
	if hdrs, ok := rm["headers"].(map[string]Value); ok {
		for k, v := range hdrs {
			_w.Header().Set(k, valueToString(v))
		}
	}
	// Apply cookies
	if cookies, ok := rm["cookies"].(map[string]Value); ok {
		for name, v := range cookies {
			switch cv := v.(type) {
			case map[string]Value:
				c := &http.Cookie{Name: name}
				if val, ok := cv["value"]; ok { c.Value = valueToString(val) }
				if p, ok := cv["path"]; ok { c.Path = valueToString(p) } else { c.Path = "/" }
				if d, ok := cv["domain"]; ok { c.Domain = valueToString(d) }
				if ho, ok := cv["httpOnly"]; ok { if b, ok := ho.(bool); ok { c.HttpOnly = b } }
				if s, ok := cv["secure"]; ok { if b, ok := s.(bool); ok { c.Secure = b } }
				if ma, ok := cv["maxAge"]; ok { if n, ok := ma.(int64); ok { c.MaxAge = int(n) } }
				if ss, ok := cv["sameSite"]; ok {
					switch valueToString(ss) {
					case "lax": c.SameSite = http.SameSiteLaxMode
					case "strict": c.SameSite = http.SameSiteStrictMode
					case "none": c.SameSite = http.SameSiteNoneMode
					}
				}
				http.SetCookie(_w, c)
			case string:
				http.SetCookie(_w, &http.Cookie{Name: name, Value: cv, Path: "/"})
			}
		}
	}
	// Determine content type and write body
	status := 200
	if s, ok := rm["status"].(int64); ok { status = int(s) }
	respType := "json"
	if t, ok := rm["type"].(string); ok { respType = t }
	body := rm["body"]
	switch respType {
	case "json":
		if _w.Header().Get("Content-Type") == "" {
			_w.Header().Set("Content-Type", "application/json")
		}
		_w.WriteHeader(status)
		if body != nil { json.NewEncoder(_w).Encode(valueToGo(body)) }
	case "text":
		if _w.Header().Get("Content-Type") == "" {
			_w.Header().Set("Content-Type", "text/plain")
		}
		_w.WriteHeader(status)
		if body != nil { fmt.Fprint(_w, valueToString(body)) }
	case "html":
		if _w.Header().Get("Content-Type") == "" {
			_w.Header().Set("Content-Type", "text/html")
		}
		_w.WriteHeader(status)
		if body != nil { fmt.Fprint(_w, valueToString(body)) }
	default:
		_w.WriteHeader(status)
		if body != nil { fmt.Fprint(_w, valueToString(body)) }
	}
}

// Body parsers
func parseJSONBody(s string) Value {
	if s == "" { return null }
	var raw interface{}
	if err := json.Unmarshal([]byte(s), &raw); err != nil { return null }
	return goToValue(raw)
}

func parseFormBody(s string) Value {
	result := make(map[string]Value)
	for _, pair := range strings.Split(s, "&") {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			key, _ := url.QueryUnescape(parts[0])
			val, _ := url.QueryUnescape(parts[1])
			result[key] = Value(val)
		}
	}
	return result
}

func parseMultipartBody(r *http.Request) (Value, Value) {
	data := make(map[string]Value)
	files := make([]Value, 0)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		return data, Value(files)
	}
	if r.MultipartForm != nil {
		for k, v := range r.MultipartForm.Value {
			if len(v) == 1 { data[k] = Value(v[0]) } else {
				arr := make([]Value, len(v))
				for i, s := range v { arr[i] = Value(s) }
				data[k] = Value(arr)
			}
		}
		for k, fhs := range r.MultipartForm.File {
			for _, fh := range fhs {
				f, err := fh.Open()
				if err != nil { continue }
				fileData, _ := io.ReadAll(f)
				f.Close()
				files = append(files, Value(map[string]Value{
					"field": Value(k),
					"name":  Value(fh.Filename),
					"size":  Value(int64(fh.Size)),
					"type":  Value(fh.Header.Get("Content-Type")),
					"data":  Value(string(fileData)),
				}))
			}
		}
	}
	return data, Value(files)
}

// Suppress unused
var _ = fmt.Sprintf
var _ = strconv.Itoa
var _ = io.Discard
var _ = json.Marshal
var _ = context.Background
`)
	// emit store if needed
	if c.usedBuiltins["store"] {
		c.raw(`
// Concurrent store with optional persistence
type storeEntry struct {
	value   Value
	expires int64 // unix timestamp, 0 = no expiry
}

type concurrentStore struct {
	mu          sync.RWMutex
	data        map[string]storeEntry
	dirtyKeys   map[string]bool
	deletedKeys map[string]bool
	synced      bool
}
var globalStore = &concurrentStore{
	data:        make(map[string]storeEntry),
	dirtyKeys:   make(map[string]bool),
	deletedKeys: make(map[string]bool),
}
var _storeFlush func() // set by store.sync for graceful shutdown

func storeExpired(e storeEntry) bool {
	return e.expires > 0 && e.expires < time.Now().Unix()
}

func storeGet(key string, def Value) Value {
	globalStore.mu.RLock()
	e, ok := globalStore.data[key]
	globalStore.mu.RUnlock()
	if !ok { return def }
	if storeExpired(e) {
		globalStore.mu.Lock()
		delete(globalStore.data, key)
		if globalStore.synced { globalStore.deletedKeys[key] = true }
		globalStore.mu.Unlock()
		return def
	}
	return e.value
}
func storeSet(key string, val Value, ttl int64) Value {
	var expires int64
	if ttl > 0 { expires = time.Now().Unix() + ttl }
	globalStore.mu.Lock()
	globalStore.data[key] = storeEntry{value: val, expires: expires}
	globalStore.dirtyKeys[key] = true
	delete(globalStore.deletedKeys, key)
	globalStore.mu.Unlock()
	return val
}
func storeDelete(key string) {
	globalStore.mu.Lock()
	delete(globalStore.data, key)
	delete(globalStore.dirtyKeys, key)
	if globalStore.synced {
		globalStore.deletedKeys[key] = true
	}
	globalStore.mu.Unlock()
}
func storeHas(key string) bool {
	globalStore.mu.RLock()
	e, ok := globalStore.data[key]
	globalStore.mu.RUnlock()
	if !ok { return false }
	if storeExpired(e) {
		globalStore.mu.Lock()
		delete(globalStore.data, key)
		if globalStore.synced { globalStore.deletedKeys[key] = true }
		globalStore.mu.Unlock()
		return false
	}
	return true
}
func storeAll() map[string]Value {
	now := time.Now().Unix()
	globalStore.mu.RLock()
	r := make(map[string]Value, len(globalStore.data))
	for k, e := range globalStore.data {
		if e.expires == 0 || e.expires >= now {
			r[k] = e.value
		}
	}
	globalStore.mu.RUnlock()
	return r
}
func storeIncr(key string, amount int64, ttl int64) int64 {
	globalStore.mu.Lock()
	defer globalStore.mu.Unlock()
	e, exists := globalStore.data[key]
	var n int64
	if exists && !storeExpired(e) {
		if ci, ok := e.value.(int64); ok { n = ci + amount } else { n = amount }
	} else {
		n = amount
	}
	var expires int64
	if ttl > 0 {
		expires = time.Now().Unix() + ttl
	} else if exists && e.expires > 0 {
		expires = e.expires // preserve existing TTL
	}
	globalStore.data[key] = storeEntry{value: Value(n), expires: expires}
	globalStore.dirtyKeys[key] = true
	delete(globalStore.deletedKeys, key)
	return n
}

func init() {
	// Sweep expired store entries every 60 seconds
	go func() {
		for range time.NewTicker(60 * time.Second).C {
			now := time.Now().Unix()
			globalStore.mu.Lock()
			for k, e := range globalStore.data {
				if e.expires > 0 && e.expires < now {
					delete(globalStore.data, k)
					if globalStore.synced { globalStore.deletedKeys[k] = true }
				}
			}
			globalStore.mu.Unlock()
		}
	}()
}
`)
		c.emitStoreSync()
	}

	// Static file serving runtime
	if len(c.staticMounts) > 0 {
		c.raw(`
// noDirFS wraps http.Dir to prevent directory listings
type noDirFS struct {
	fs http.FileSystem
}

func (n noDirFS) Open(name string) (http.File, error) {
	f, err := n.fs.Open(name)
	if err != nil { return nil, err }
	stat, err := f.Stat()
	if err != nil { f.Close(); return nil, err }
	if stat.IsDir() {
		idx, err := n.fs.Open(filepath.Join(name, "index.html"))
		if err != nil { f.Close(); return nil, os.ErrNotExist }
		idx.Close()
	}
	return f, nil
}
`)
	}

	// Gzip middleware
	if c.gzipEnabled {
		c.raw(`
type gzipResponseWriter struct {
	http.ResponseWriter
	gz *gzip.Writer
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.gz.Write(b)
}

func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")
		w.Header().Del("Content-Length")
		gz := gzip.NewWriter(w)
		defer gz.Close()
		next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, gz: gz}, r)
	})
}
`)
	}
	c.ln("")
}

func (c *NativeCompiler) emitBuiltinFuncs() {
	c.raw(`// ===== Builtins =====
func builtin_print(args ...Value) Value {
	parts := make([]string, len(args))
	for i, a := range args { parts[i] = valueToString(a) }
	fmt.Println(strings.Join(parts, " "))
	return null
}

func builtin_env(args ...Value) Value {
	if len(args) == 0 { return null }
	v := os.Getenv(valueToString(args[0]))
	if v == "" { if len(args) >= 2 { return args[1] }; return null }
	return Value(v)
}

func builtin_sleep(args ...Value) Value {
	if len(args) == 0 { return null }
	ms := toInt64(args[0])
	time.Sleep(time.Duration(ms) * time.Millisecond)
	return null
}

func builtin_fetch(args ...Value) Value {
	if len(args) == 0 { throw(Value("fetch requires a URL")) }
	url := valueToString(args[0])
	method := "GET"
	var reqBody io.Reader
	reqHeaders := map[string]string{}
	timeout := 30 * time.Second

	if len(args) >= 2 {
		if opts, ok := args[1].(map[string]Value); ok {
			if m, ok := opts["method"]; ok { method = valueToString(m) }
			if h, ok := opts["headers"].(map[string]Value); ok {
				for k, v := range h { reqHeaders[k] = valueToString(v) }
			}
			if t, ok := opts["timeout"]; ok {
				timeout = time.Duration(toInt64(t)) * time.Millisecond
			}
			if b, ok := opts["body"]; ok {
				switch bv := b.(type) {
				case string:
					reqBody = strings.NewReader(bv)
				case map[string]Value, []Value:
					jb, _ := json.Marshal(valueToGo(bv))
					reqBody = bytes.NewReader(jb)
					if _, ok := reqHeaders["Content-Type"]; !ok {
						reqHeaders["Content-Type"] = "application/json"
					}
				default:
					reqBody = strings.NewReader(valueToString(bv))
				}
			}
		}
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil { throw(Value("fetch: " + err.Error())) }
	for k, v := range reqHeaders { req.Header.Set(k, v) }

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil { throw(Value("fetch: " + err.Error())) }
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil { throw(Value("fetch: " + err.Error())) }
	bodyStr := string(body)

	// Detect type from Content-Type
	ct := resp.Header.Get("Content-Type")
	respType := "text"
	var respBody Value = Value(bodyStr)
	if strings.Contains(ct, "application/json") {
		respType = "json"
		parsed := parseJSONBody(bodyStr)
		if parsed != null { respBody = parsed }
	} else if strings.Contains(ct, "text/html") {
		respType = "html"
	}

	// Response headers
	respHeaders := make(map[string]Value)
	for k, v := range resp.Header {
		if len(v) == 1 { respHeaders[strings.ToLower(k)] = Value(v[0]) } else {
			arr := make([]Value, len(v))
			for i, s := range v { arr[i] = Value(s) }
			respHeaders[strings.ToLower(k)] = Value(arr)
		}
	}

	// Cookies from Set-Cookie
	respCookies := make(map[string]Value)
	for _, c := range resp.Cookies() {
		respCookies[c.Name] = Value(c.Value)
	}

	return Value(map[string]Value{
		"status":  Value(int64(resp.StatusCode)),
		"type":    Value(respType),
		"body":    respBody,
		"headers": Value(respHeaders),
		"cookies": Value(respCookies),
	})
}

func builtin_await(args ...Value) Value {
	results := make([]Value, len(args))
	for i, a := range args {
		results[i] = resolveValue(a)
	}
	if len(results) == 1 { return results[0] }
	return &multiReturn{Values: results}
}

func builtin_race(args ...Value) Value {
	if len(args) == 0 { return null }
	if len(args) == 1 { return resolveValue(args[0]) }
	ch := make(chan Value, 1)
	for _, a := range args {
		go func(v Value) {
			resolved := resolveValue(v)
			select {
			case ch <- resolved:
			default:
			}
		}(a)
	}
	return <-ch
}

func builtin_len(args ...Value) Value {
	if len(args) == 0 { return int64(0) }
	switch v := args[0].(type) {
	case string: return int64(len(v))
	case []Value: return int64(len(v))
	case map[string]Value: return int64(len(v))
	default: return int64(0)
	}
}

func builtin_str(args ...Value) Value {
	if len(args) == 0 { return "" }
	return valueToString(args[0])
}

func builtin_int(args ...Value) Value {
	if len(args) == 0 { return int64(0) }
	if len(args) >= 2 {
		// int(str, base) — parse with specified base
		base := int(toInt64(args[1]))
		if n, err := strconv.ParseInt(valueToString(args[0]), base, 64); err == nil { return n }
		return int64(0)
	}
	return toInt64(args[0])
}

func builtin_float(args ...Value) Value {
	if len(args) == 0 { return float64(0) }
	return toFloat64v(args[0])
}

func builtin_bool(args ...Value) Value {
	if len(args) == 0 { return false }
	return isTruthy(args[0])
}

func builtin_type(args ...Value) Value {
	if len(args) == 0 { return "null" }
	switch args[0].(type) {
	case int64: return "int"
	case float64: return "float"
	case string: return "string"
	case bool: return "bool"
	case []Value: return "array"
	case map[string]Value: return "object"
	case *nullType, nil: return "null"
	default: return "unknown"
	}
}

func builtin_append(args ...Value) Value {
	if len(args) < 2 { return null }
	if a, ok := args[0].([]Value); ok {
		return append(a, args[1:]...)
	}
	return args[0]
}

func builtin_keys(args ...Value) Value {
	if len(args) == 0 { return []Value{} }
	if m, ok := args[0].(map[string]Value); ok {
		r := make([]Value, 0, len(m))
		for k := range m { r = append(r, k) }
		return r
	}
	return []Value{}
}

func builtin_values(args ...Value) Value {
	if len(args) == 0 { return []Value{} }
	if m, ok := args[0].(map[string]Value); ok {
		r := make([]Value, 0, len(m))
		for _, v := range m { r = append(r, v) }
		return r
	}
	return []Value{}
}

func builtin_contains(args ...Value) Value {
	if len(args) < 2 { return false }
	switch col := args[0].(type) {
	case string:
		return strings.Contains(col, valueToString(args[1]))
	case []Value:
		for _, v := range col {
			if valuesEqual(v, args[1]) { return true }
		}
		return false
	case map[string]Value:
		_, ok := col[valueToString(args[1])]
		return ok
	}
	return false
}

func builtin_trim(args ...Value) Value {
	if len(args) == 0 { return "" }
	return strings.TrimSpace(valueToString(args[0]))
}

func builtin_split(args ...Value) Value {
	if len(args) < 2 { return []Value{} }
	parts := strings.Split(valueToString(args[0]), valueToString(args[1]))
	r := make([]Value, len(parts))
	for i, p := range parts { r[i] = p }
	return r
}

func builtin_join(args ...Value) Value {
	if len(args) < 2 { return "" }
	if a, ok := args[0].([]Value); ok {
		parts := make([]string, len(a))
		for i, v := range a { parts[i] = valueToString(v) }
		return strings.Join(parts, valueToString(args[1]))
	}
	return ""
}

func builtin_upper(args ...Value) Value {
	if len(args) == 0 { return "" }
	return strings.ToUpper(valueToString(args[0]))
}

func builtin_lower(args ...Value) Value {
	if len(args) == 0 { return "" }
	return strings.ToLower(valueToString(args[0]))
}

func builtin_replace(args ...Value) Value {
	if len(args) < 3 { return "" }
	return strings.ReplaceAll(valueToString(args[0]), valueToString(args[1]), valueToString(args[2]))
}

func builtin_starts_with(args ...Value) Value {
	if len(args) < 2 { return false }
	return strings.HasPrefix(valueToString(args[0]), valueToString(args[1]))
}

func builtin_ends_with(args ...Value) Value {
	if len(args) < 2 { return false }
	return strings.HasSuffix(valueToString(args[0]), valueToString(args[1]))
}

func builtin_slice(args ...Value) Value {
	if len(args) < 2 { return null }
	start := toInt64(args[1])
	switch col := args[0].(type) {
	case string:
		end := int64(len(col))
		if len(args) >= 3 { end = toInt64(args[2]) }
		if start < 0 { start = int64(len(col)) + start }
		if end < 0 { end = int64(len(col)) + end }
		if start < 0 { start = 0 }
		if end > int64(len(col)) { end = int64(len(col)) }
		if start >= end { return "" }
		return col[start:end]
	case []Value:
		end := int64(len(col))
		if len(args) >= 3 { end = toInt64(args[2]) }
		if start < 0 { start = int64(len(col)) + start }
		if end < 0 { end = int64(len(col)) + end }
		if start < 0 { start = 0 }
		if end > int64(len(col)) { end = int64(len(col)) }
		if start >= end { return []Value{} }
		r := make([]Value, end-start)
		copy(r, col[start:end])
		return r
	}
	return null
}

func builtin_reverse(args ...Value) Value {
	if len(args) == 0 { return null }
	if a, ok := args[0].([]Value); ok {
		r := make([]Value, len(a))
		for i, v := range a { r[len(a)-1-i] = v }
		return r
	}
	if s, ok := args[0].(string); ok {
		runes := []rune(s)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 { runes[i], runes[j] = runes[j], runes[i] }
		return string(runes)
	}
	return null
}

func builtin_unique(args ...Value) Value {
	if len(args) == 0 { return []Value{} }
	if a, ok := args[0].([]Value); ok {
		seen := make(map[string]bool)
		r := make([]Value, 0)
		for _, v := range a {
			k := valueToString(v)
			if !seen[k] { seen[k] = true; r = append(r, v) }
		}
		return r
	}
	return []Value{}
}

func builtin_has(args ...Value) Value { return builtin_contains(args...) }
func builtin_includes(args ...Value) Value { return builtin_contains(args...) }

func builtin_merge(args ...Value) Value {
	r := make(map[string]Value)
	for _, a := range args {
		if m, ok := a.(map[string]Value); ok {
			for k, v := range m { r[k] = v }
		}
	}
	return r
}

func builtin_delete(args ...Value) Value {
	if len(args) < 2 { return null }
	if m, ok := args[0].(map[string]Value); ok {
		key := valueToString(args[1])
		delete(m, key)
		return m
	}
	if a, ok := args[0].([]Value); ok {
		idx := toInt64(args[1])
		if idx >= 0 && int(idx) < len(a) {
			return append(a[:idx], a[idx+1:]...)
		}
		return a
	}
	return null
}

func builtin_json_parse(args ...Value) Value {
	if len(args) == 0 { return null }
	s := valueToString(args[0])
	var raw interface{}
	if err := json.Unmarshal([]byte(s), &raw); err != nil { return null }
	return goToValue(raw)
}

func builtin_json_stringify(args ...Value) Value {
	if len(args) == 0 { return "" }
	data := valueToGo(args[0])
	b, _ := json.Marshal(data)
	return string(b)
}

func builtin_index_of(args ...Value) Value {
	if len(args) < 2 { return int64(-1) }
	switch col := args[0].(type) {
	case string:
		idx := strings.Index(col, valueToString(args[1]))
		return int64(idx)
	case []Value:
		for i, v := range col {
			if valuesEqual(v, args[1]) { return int64(i) }
		}
	}
	return int64(-1)
}

func builtin_repeat(args ...Value) Value {
	if len(args) < 2 { return "" }
	return strings.Repeat(valueToString(args[0]), int(toInt64(args[1])))
}

func builtin_flat(args ...Value) Value {
	if len(args) == 0 { return []Value{} }
	if a, ok := args[0].([]Value); ok {
		r := make([]Value, 0)
		for _, v := range a {
			if inner, ok := v.([]Value); ok {
				r = append(r, inner...)
			} else {
				r = append(r, v)
			}
		}
		return r
	}
	return []Value{}
}
`)

	if c.usedBuiltins["file"] {
		c.raw(`
// ===== File I/O =====
func builtin_file_read(args ...Value) Value {
	if len(args) == 0 { return null }
	data, err := os.ReadFile(valueToString(args[0]))
	if err != nil { return null }
	return Value(string(data))
}

func builtin_file_write(args ...Value) Value {
	if len(args) < 2 { throw(Value("file.write requires path and data")) }
	path := valueToString(args[0])
	data := valueToString(args[1])
	if len(args) >= 3 {
		perm := os.FileMode(toInt64(args[2]))
		if err := os.WriteFile(path, []byte(data), perm); err != nil {
			throw(Value("file.write: " + err.Error()))
		}
	} else {
		// Preserve existing permissions, default 0644 for new files
		perm := os.FileMode(0644)
		if info, err := os.Stat(path); err == nil {
			perm = info.Mode().Perm()
		}
		if err := os.WriteFile(path, []byte(data), perm); err != nil {
			throw(Value("file.write: " + err.Error()))
		}
	}
	return Value(true)
}

func builtin_file_append(args ...Value) Value {
	if len(args) < 2 { throw(Value("file.append requires path and data")) }
	path := valueToString(args[0])
	data := valueToString(args[1])
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil { throw(Value("file.append: " + err.Error())) }
	defer f.Close()
	if _, err := f.WriteString(data); err != nil {
		throw(Value("file.append: " + err.Error()))
	}
	return Value(true)
}

func builtin_file_read_json(args ...Value) Value {
	if len(args) == 0 { return null }
	data, err := os.ReadFile(valueToString(args[0]))
	if err != nil { return null }
	return parseJSONBody(string(data))
}

func builtin_file_write_json(args ...Value) Value {
	if len(args) < 2 { throw(Value("file.write_json requires path and data")) }
	path := valueToString(args[0])
	b, err := json.MarshalIndent(valueToGo(args[1]), "", "  ")
	if err != nil { throw(Value("file.write_json: " + err.Error())) }
	if len(args) >= 3 {
		perm := os.FileMode(toInt64(args[2]))
		if err := os.WriteFile(path, append(b, '\n'), perm); err != nil {
			throw(Value("file.write_json: " + err.Error()))
		}
	} else {
		perm := os.FileMode(0644)
		if info, err := os.Stat(path); err == nil {
			perm = info.Mode().Perm()
		}
		if err := os.WriteFile(path, append(b, '\n'), perm); err != nil {
			throw(Value("file.write_json: " + err.Error()))
		}
	}
	return Value(true)
}

func builtin_file_exists(args ...Value) Value {
	if len(args) == 0 { return Value(false) }
	_, err := os.Stat(valueToString(args[0]))
	return Value(err == nil)
}

func builtin_file_delete(args ...Value) Value {
	if len(args) == 0 { throw(Value("file.delete requires a path")) }
	if err := os.Remove(valueToString(args[0])); err != nil {
		throw(Value("file.delete: " + err.Error()))
	}
	return Value(true)
}

func builtin_file_list(args ...Value) Value {
	if len(args) == 0 { return null }
	entries, err := os.ReadDir(valueToString(args[0]))
	if err != nil { return null }
	result := make([]Value, len(entries))
	for i, e := range entries {
		info, _ := e.Info()
		size := int64(0)
		if info != nil { size = info.Size() }
		result[i] = Value(map[string]Value{
			"name":   Value(e.Name()),
			"is_dir": Value(e.IsDir()),
			"size":   Value(size),
		})
	}
	return Value(result)
}

func builtin_file_mkdir(args ...Value) Value {
	if len(args) == 0 { throw(Value("file.mkdir requires a path")) }
	perm := os.FileMode(0755)
	if len(args) >= 2 { perm = os.FileMode(toInt64(args[1])) }
	if err := os.MkdirAll(valueToString(args[0]), perm); err != nil {
		throw(Value("file.mkdir: " + err.Error()))
	}
	return Value(true)
}

func builtin_file_chmod(args ...Value) Value {
	if len(args) < 2 { throw(Value("file.chmod requires path and permissions")) }
	perm := os.FileMode(toInt64(args[1]))
	if err := os.Chmod(valueToString(args[0]), perm); err != nil {
		throw(Value("file.chmod: " + err.Error()))
	}
	return Value(true)
}
`)
	}

	// Always-emitted builtins
	c.raw(`
func builtin_sort(args ...Value) Value {
	if len(args) == 0 { return Value([]Value{}) }
	arr, ok := args[0].([]Value)
	if !ok { return args[0] }
	result := make([]Value, len(arr))
	copy(result, arr)
	desc := false
	if len(args) >= 2 { desc = valueToString(args[1]) == "desc" }
	sort.SliceStable(result, func(i, j int) bool {
		if desc { return compareLess(result[j], result[i]) }
		return compareLess(result[i], result[j])
	})
	return Value(result)
}

func builtin_sort_by(args ...Value) Value {
	if len(args) < 2 { return Value([]Value{}) }
	arr, ok := args[0].([]Value)
	if !ok { return args[0] }
	key := valueToString(args[1])
	result := make([]Value, len(arr))
	copy(result, arr)
	desc := false
	if len(args) >= 3 { desc = valueToString(args[2]) == "desc" }
	sort.SliceStable(result, func(i, j int) bool {
		a := dotValue(result[i], key)
		b := dotValue(result[j], key)
		if desc { return compareLess(b, a) }
		return compareLess(a, b)
	})
	return Value(result)
}

func builtin_regex_match(args ...Value) Value {
	if len(args) < 2 { return null }
	str := valueToString(args[0])
	pattern := valueToString(args[1])
	re, err := regexp.Compile(pattern)
	if err != nil { return null }
	matches := re.FindAllString(str, -1)
	if matches == nil { return null }
	result := make([]Value, len(matches))
	for i, m := range matches { result[i] = Value(m) }
	return Value(result)
}

func builtin_regex_replace(args ...Value) Value {
	if len(args) < 3 { return Value("") }
	str := valueToString(args[0])
	pattern := valueToString(args[1])
	repl := valueToString(args[2])
	re, err := regexp.Compile(pattern)
	if err != nil { return Value(str) }
	return Value(re.ReplaceAllString(str, repl))
}

func builtin_rand(args ...Value) Value {
	if len(args) == 0 { return Value(mrand.Float64()) }
	if len(args) == 1 { return Value(int64(mrand.Intn(int(toInt64(args[0]))))) }
	min := int(toInt64(args[0]))
	max := int(toInt64(args[1]))
	if max <= min { return Value(int64(min)) }
	return Value(int64(min + mrand.Intn(max-min)))
}

func builtin_uuid(args ...Value) Value {
	b := make([]byte, 16)
	crand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return Value(fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]))
}

var cuid2Counter uint64
var cuid2Mu sync.Mutex

func builtin_cuid2(args ...Value) Value {
	length := 24
	if len(args) > 0 { l := int(toInt64(args[0])); if l > 0 { length = l } }
	cuid2Mu.Lock()
	cuid2Counter++
	cnt := cuid2Counter
	cuid2Mu.Unlock()
	rb := make([]byte, 32)
	crand.Read(rb)
	ts := time.Now().UnixMilli()
	data := fmt.Sprintf("%d%d%x", ts, cnt, rb)
	hash := sha256.Sum256([]byte(data))
	const chars = "0123456789abcdefghijklmnopqrstuvwxyz"
	result := make([]byte, length)
	result[0] = chars[hash[0]%26+10] // first char always a letter
	for i := 1; i < length; i++ {
		result[i] = chars[hash[i%32]%36]
	}
	return Value(string(result))
}

func builtin_abs(args ...Value) Value {
	if len(args) == 0 { return Value(int64(0)) }
	v := args[0]
	if i, ok := v.(int64); ok { if i < 0 { return Value(-i) }; return v }
	if f, ok := v.(float64); ok { return Value(math.Abs(f)) }
	return Value(int64(0))
}

func builtin_ceil(args ...Value) Value {
	if len(args) == 0 { return Value(int64(0)) }
	return Value(int64(math.Ceil(toFloat64v(args[0]))))
}

func builtin_floor(args ...Value) Value {
	if len(args) == 0 { return Value(int64(0)) }
	return Value(int64(math.Floor(toFloat64v(args[0]))))
}

func builtin_round(args ...Value) Value {
	if len(args) == 0 { return Value(int64(0)) }
	v := toFloat64v(args[0])
	if len(args) >= 2 {
		dec := toInt64(args[1])
		mul := math.Pow(10, float64(dec))
		return Value(math.Round(v*mul) / mul)
	}
	return Value(int64(math.Round(v)))
}

func builtin_base64_encode(args ...Value) Value {
	if len(args) == 0 { return Value("") }
	return Value(base64.StdEncoding.EncodeToString([]byte(valueToString(args[0]))))
}

func builtin_base64_decode(args ...Value) Value {
	if len(args) == 0 { return Value("") }
	b, err := base64.StdEncoding.DecodeString(valueToString(args[0]))
	if err != nil { return null }
	return Value(string(b))
}

func builtin_url_encode(args ...Value) Value {
	if len(args) == 0 { return Value("") }
	return Value(url.QueryEscape(valueToString(args[0])))
}

func builtin_url_decode(args ...Value) Value {
	if len(args) == 0 { return Value("") }
	v, err := url.QueryUnescape(valueToString(args[0]))
	if err != nil { return Value(valueToString(args[0])) }
	return Value(v)
}

func builtin_hash(args ...Value) Value {
	if len(args) < 2 { return Value("") }
	algo := valueToString(args[0])
	data := []byte(valueToString(args[1]))
	switch algo {
	case "sha256":
		h := sha256.Sum256(data)
		return Value(hex.EncodeToString(h[:]))
	case "sha512":
		h := sha512.Sum512(data)
		return Value(hex.EncodeToString(h[:]))
	case "md5":
		h := md5.Sum(data)
		return Value(hex.EncodeToString(h[:]))
	default:
		return Value("")
	}
}

func builtin_hmac_hash(args ...Value) Value {
	if len(args) < 3 { return Value("") }
	algo := valueToString(args[0])
	key := []byte(valueToString(args[1]))
	data := []byte(valueToString(args[2]))
	var h hash.Hash
	switch algo {
	case "sha256":
		h = hmac.New(sha256.New, key)
	case "sha512":
		h = hmac.New(sha512.New, key)
	default:
		return Value("")
	}
	h.Write(data)
	return Value(hex.EncodeToString(h.Sum(nil)))
}

func dslLog(level string, r *http.Request, args ...Value) Value {
	parts := make([]string, len(args))
	for i, a := range args { parts[i] = valueToString(a) }
	msg := strings.Join(parts, " ")

	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	var sb strings.Builder
	sb.WriteString(now)
	if level != "" {
		sb.WriteString(" [")
		sb.WriteString(level)
		sb.WriteString("]")
	}
	if r != nil {
		sb.WriteString(" ")
		sb.WriteString(r.Method)
		sb.WriteString(" ")
		sb.WriteString(r.URL.Path)
	}
	sb.WriteString(" — ")
	sb.WriteString(msg)
	fmt.Fprintln(os.Stderr, sb.String())
	return null
}

func builtin_map(args ...Value) Value {
	if len(args) < 2 { return Value([]Value{}) }
	arr, ok := args[0].([]Value)
	if !ok { return Value([]Value{}) }
	fn := resolveValue(args[1])
	result := make([]Value, len(arr))
	if f, ok := fn.(func(...Value) Value); ok {
		for i, v := range arr { result[i] = f(v, Value(int64(i))) }
	} else {
		copy(result, arr)
	}
	return Value(result)
}

func builtin_filter(args ...Value) Value {
	if len(args) < 2 { return Value([]Value{}) }
	arr, ok := args[0].([]Value)
	if !ok { return Value([]Value{}) }
	fn := resolveValue(args[1])
	result := make([]Value, 0)
	if f, ok := fn.(func(...Value) Value); ok {
		for i, v := range arr {
			if isTruthy(f(v, Value(int64(i)))) { result = append(result, v) }
		}
	}
	return Value(result)
}

func builtin_reduce(args ...Value) Value {
	if len(args) < 3 { return null }
	arr, ok := args[0].([]Value)
	if !ok { return null }
	fn := resolveValue(args[1])
	acc := args[2]
	if f, ok := fn.(func(...Value) Value); ok {
		for _, v := range arr { acc = f(acc, v) }
	}
	return acc
}

func builtin_find(args ...Value) Value {
	if len(args) < 2 { return null }
	arr, ok := args[0].([]Value)
	if !ok { return null }
	fn := resolveValue(args[1])
	if f, ok := fn.(func(...Value) Value); ok {
		for _, v := range arr {
			if isTruthy(f(v)) { return v }
		}
	}
	return null
}

func builtin_some(args ...Value) Value {
	if len(args) < 2 { return Value(false) }
	arr, ok := args[0].([]Value)
	if !ok { return Value(false) }
	fn := resolveValue(args[1])
	if f, ok := fn.(func(...Value) Value); ok {
		for _, v := range arr {
			if isTruthy(f(v)) { return Value(true) }
		}
	}
	return Value(false)
}

func builtin_every(args ...Value) Value {
	if len(args) < 2 { return Value(true) }
	arr, ok := args[0].([]Value)
	if !ok { return Value(false) }
	if len(arr) == 0 { return Value(true) }
	fn := resolveValue(args[1])
	if f, ok := fn.(func(...Value) Value); ok {
		for _, v := range arr {
			if !isTruthy(f(v)) { return Value(false) }
		}
	}
	return Value(true)
}

func builtin_count(args ...Value) Value {
	if len(args) < 2 { return Value(int64(0)) }
	arr, ok := args[0].([]Value)
	if !ok { return Value(int64(0)) }
	fn := resolveValue(args[1])
	var n int64
	if f, ok := fn.(func(...Value) Value); ok {
		for _, v := range arr {
			if isTruthy(f(v)) { n++ }
		}
	}
	return Value(n)
}

func builtin_pluck(args ...Value) Value {
	if len(args) < 2 { return Value([]Value{}) }
	arr, ok := args[0].([]Value)
	if !ok { return Value([]Value{}) }
	key := valueToString(args[1])
	result := make([]Value, len(arr))
	for i, v := range arr {
		if m, ok := v.(map[string]Value); ok {
			result[i] = m[key]
			if result[i] == nil { result[i] = null }
		} else { result[i] = null }
	}
	return Value(result)
}

func builtin_group_by(args ...Value) Value {
	if len(args) < 2 { return Value(map[string]Value{}) }
	arr, ok := args[0].([]Value)
	if !ok { return Value(map[string]Value{}) }
	result := make(map[string]Value)
	key := valueToString(args[1])
	for _, v := range arr {
		var gk string
		if m, ok := v.(map[string]Value); ok {
			gk = valueToString(m[key])
		}
		if existing, ok := result[gk]; ok {
			result[gk] = Value(append(existing.([]Value), v))
		} else {
			result[gk] = Value([]Value{v})
		}
	}
	return Value(result)
}

func builtin_sum(args ...Value) Value {
	if len(args) == 0 { return Value(int64(0)) }
	arr, ok := args[0].([]Value)
	if !ok { return Value(int64(0)) }
	var total float64
	isFloat := false
	for _, v := range arr {
		switch n := v.(type) {
		case int64: total += float64(n)
		case float64: total += n; isFloat = true
		}
	}
	if isFloat { return Value(total) }
	return Value(int64(total))
}

func builtin_min(args ...Value) Value {
	if len(args) >= 2 {
		if compareLess(args[0], args[1]) { return args[0] }
		return args[1]
	}
	if len(args) == 1 {
		arr, ok := args[0].([]Value)
		if !ok || len(arr) == 0 { return null }
		min := arr[0]
		for _, v := range arr[1:] { if compareLess(v, min) { min = v } }
		return min
	}
	return null
}

func builtin_max(args ...Value) Value {
	if len(args) >= 2 {
		if compareLess(args[1], args[0]) { return args[0] }
		return args[1]
	}
	if len(args) == 1 {
		arr, ok := args[0].([]Value)
		if !ok || len(arr) == 0 { return null }
		max := arr[0]
		for _, v := range arr[1:] { if compareLess(max, v) { max = v } }
		return max
	}
	return null
}

func builtin_clamp(args ...Value) Value {
	if len(args) < 3 { return null }
	v := args[0]; lo := args[1]; hi := args[2]
	if compareLess(v, lo) { return lo }
	if compareLess(hi, v) { return hi }
	return v
}

func builtin_chunk(args ...Value) Value {
	if len(args) < 2 { return Value([]Value{}) }
	arr, ok := args[0].([]Value)
	if !ok { return Value([]Value{}) }
	size := int(toInt64(args[1]))
	if size <= 0 { return Value([]Value{}) }
	var result []Value
	for i := 0; i < len(arr); i += size {
		end := i + size
		if end > len(arr) { end = len(arr) }
		result = append(result, Value(arr[i:end]))
	}
	return Value(result)
}

func builtin_range(args ...Value) Value {
	if len(args) == 0 { return Value([]Value{}) }
	var start, end int64
	if len(args) == 1 {
		end = toInt64(args[0])
	} else {
		start = toInt64(args[0])
		end = toInt64(args[1])
	}
	step := int64(1)
	if len(args) >= 3 { step = toInt64(args[2]) }
	if step == 0 { return Value([]Value{}) }
	var result []Value
	if step > 0 {
		for i := start; i < end; i += step { result = append(result, Value(i)) }
	} else {
		for i := start; i > end; i += step { result = append(result, Value(i)) }
	}
	return Value(result)
}

func builtin_pad_left(args ...Value) Value {
	if len(args) < 2 { return Value("") }
	s := valueToString(args[0])
	width := int(toInt64(args[1]))
	pad := " "
	if len(args) >= 3 { pad = valueToString(args[2]) }
	for len(s) < width { s = pad + s }
	return Value(s[:width])
}

func builtin_pad_right(args ...Value) Value {
	if len(args) < 2 { return Value("") }
	s := valueToString(args[0])
	width := int(toInt64(args[1]))
	pad := " "
	if len(args) >= 3 { pad = valueToString(args[2]) }
	for len(s) < width { s = s + pad }
	return Value(s[:width])
}

func builtin_truncate(args ...Value) Value {
	if len(args) < 2 { return Value("") }
	s := valueToString(args[0])
	maxLen := int(toInt64(args[1]))
	suffix := "..."
	if len(args) >= 3 { suffix = valueToString(args[2]) }
	if len(s) <= maxLen { return Value(s) }
	if maxLen <= len(suffix) { return Value(suffix[:maxLen]) }
	return Value(s[:maxLen-len(suffix)] + suffix)
}

func builtin_capitalize(args ...Value) Value {
	if len(args) == 0 { return Value("") }
	s := valueToString(args[0])
	if len(s) == 0 { return Value(s) }
	return Value(strings.ToUpper(s[:1]) + s[1:])
}

func builtin_date(args ...Value) Value {
	var t time.Time
	if len(args) > 0 { t = time.Unix(toInt64(args[0]), 0) } else { t = time.Now() }
	return Value(map[string]Value{
		"year": Value(int64(t.Year())), "month": Value(int64(t.Month())),
		"day": Value(int64(t.Day())), "hour": Value(int64(t.Hour())),
		"minute": Value(int64(t.Minute())), "second": Value(int64(t.Second())),
		"weekday": Value(int64(t.Weekday())), "unix": Value(t.Unix()),
	})
}

func builtin_date_format(args ...Value) Value {
	if len(args) < 2 { return Value("") }
	t := time.Unix(toInt64(args[0]), 0)
	fmt_ := valueToString(args[1])
	return Value(t.Format(fmt_))
}

func builtin_date_parse(args ...Value) Value {
	if len(args) < 2 { return null }
	str := valueToString(args[0])
	fmt_ := valueToString(args[1])
	t, err := time.Parse(fmt_, str)
	if err != nil { return null }
	return Value(t.Unix())
}

func builtin_strtotime(args ...Value) Value {
	if len(args) == 0 { return null }
	input := valueToString(args[0])
	// Parse: "now", "now + 3 days", "1234567890 + 2 hours", "+3 days", "-2 hours"
	input = strings.TrimSpace(input)
	var baseTime time.Time
	rest := input
	if strings.HasPrefix(input, "now") {
		baseTime = time.Now()
		rest = strings.TrimSpace(input[3:])
	} else if input[0] == '+' || input[0] == '-' {
		baseTime = time.Now()
		rest = input
	} else {
		// Try to parse leading number as unix timestamp
		parts := strings.SplitN(input, " ", 2)
		if ts, err := strconv.ParseInt(parts[0], 10, 64); err == nil {
			baseTime = time.Unix(ts, 0)
			if len(parts) > 1 { rest = strings.TrimSpace(parts[1]) } else { rest = "" }
		} else {
			baseTime = time.Now()
		}
	}
	if rest == "" { return Value(baseTime.Unix()) }
	// Parse +/- N unit
	var sign int
	if rest[0] == '+' { sign = 1; rest = strings.TrimSpace(rest[1:]) } else if rest[0] == '-' { sign = -1; rest = strings.TrimSpace(rest[1:]) } else { return Value(baseTime.Unix()) }
	parts := strings.SplitN(rest, " ", 2)
	if len(parts) < 2 { return Value(baseTime.Unix()) }
	amt, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil { return Value(baseTime.Unix()) }
	n := int(amt) * sign
	unit := strings.TrimSuffix(strings.ToLower(parts[1]), "s") // "days" -> "day"
	switch unit {
	case "second": baseTime = baseTime.Add(time.Duration(n) * time.Second)
	case "minute": baseTime = baseTime.Add(time.Duration(n) * time.Minute)
	case "hour": baseTime = baseTime.Add(time.Duration(n) * time.Hour)
	case "day": baseTime = baseTime.AddDate(0, 0, n)
	case "week": baseTime = baseTime.AddDate(0, 0, n*7)
	case "month": baseTime = baseTime.AddDate(0, n, 0)
	case "year": baseTime = baseTime.AddDate(n, 0, 0)
	}
	return Value(baseTime.Unix())
}

type redirectSignal struct {
	url    string
	status int
}

func builtin_redirect(args ...Value) Value {
	if len(args) == 0 { throw(Value("redirect requires a URL")) }
	url := valueToString(args[0])
	status := 302
	if len(args) >= 2 { status = int(toInt64(args[1])) }
	panic(&redirectSignal{url: url, status: status})
}
`)
	if c.usedBuiltins["jwt"] {
		c.raw(`
func jwtBase64Encode(data []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}

func jwtBase64Decode(s string) ([]byte, error) {
	switch len(s) % 4 {
	case 2: s += "=="
	case 3: s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}

func jwtHmacHash(algo string, key, data []byte) []byte {
	var h func() hash.Hash
	switch algo {
	case "HS384": h = sha512.New384
	case "HS512": h = sha512.New
	default: h = sha256.New // HS256
	}
	mac := hmac.New(h, key)
	mac.Write(data)
	return mac.Sum(nil)
}

func builtin_jwt_sign(args ...Value) Value {
	if len(args) < 2 { throw(Value("jwt.sign requires payload and secret")); return null }
	payload := args[0]
	secret := valueToString(args[1])
	algo := "HS256"
	if len(args) >= 3 { algo = valueToString(args[2]) }
	headerJSON, _ := json.Marshal(map[string]string{"alg": algo, "typ": "JWT"})
	payloadJSON, _ := json.Marshal(valueToGo(payload))
	headerB64 := jwtBase64Encode(headerJSON)
	payloadB64 := jwtBase64Encode(payloadJSON)
	sigInput := headerB64 + "." + payloadB64
	sig := jwtHmacHash(algo, []byte(secret), []byte(sigInput))
	return Value(sigInput + "." + jwtBase64Encode(sig))
}

func builtin_jwt_verify(args ...Value) Value {
	if len(args) < 2 { throw(Value("jwt.verify requires token and secret")); return null }
	token := valueToString(args[0])
	secret := valueToString(args[1])
	algo := "HS256"
	if len(args) >= 3 { algo = valueToString(args[2]) }
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 { throw(Value("jwt: invalid token")); return null }
	sigInput := parts[0] + "." + parts[1]
	expectedSig := jwtHmacHash(algo, []byte(secret), []byte(sigInput))
	actualSig, err := jwtBase64Decode(parts[2])
	if err != nil || !hmac.Equal(expectedSig, actualSig) { throw(Value("jwt: invalid signature")); return null }
	payloadJSON, err := jwtBase64Decode(parts[1])
	if err != nil { throw(Value("jwt: invalid payload")); return null }
	var raw interface{}
	if err := json.Unmarshal(payloadJSON, &raw); err != nil { throw(Value("jwt: invalid JSON")); return null }
	payload := goToValue(raw)
	// Check exp claim
	if m, ok := payload.(map[string]Value); ok {
		if exp, ok := m["exp"]; ok {
			if toInt64(exp) > 0 && toInt64(exp) < time.Now().Unix() {
				throw(Value("jwt: token expired"))
			}
		}
	}
	return payload
}
`)
	}

	// Password hashing (bcrypt + argon2id)
	if c.usedBuiltins["hash_password"] || c.usedBuiltins["verify_password"] {
		c.raw(`
func builtin_hash_password(args ...Value) Value {
	if len(args) < 1 { throw(Value("hash_password requires a password")) }
	password := valueToString(args[0])
	algo := "bcrypt"
	if len(args) >= 2 { algo = valueToString(args[1]) }
	var opts map[string]Value
	if len(args) >= 3 {
		if m, ok := args[2].(map[string]Value); ok { opts = m }
	}
	switch algo {
	case "bcrypt":
		cost := 12
		if opts != nil {
			if v, ok := opts["cost"]; ok { cost = int(toInt64(v)) }
		}
		if cost < 4 { cost = 4 }
		if cost > 31 { cost = 31 }
		hashed, err := bcrypt.GenerateFromPassword([]byte(password), cost)
		if err != nil { throw(Value("hash_password: " + err.Error())) }
		return Value(string(hashed))
	case "argon2":
		memory := uint32(65536)
		iterations := uint32(3)
		parallelism := uint8(4)
		keyLength := uint32(32)
		if opts != nil {
			if v, ok := opts["memory"]; ok { memory = uint32(toInt64(v)) }
			if v, ok := opts["iterations"]; ok { iterations = uint32(toInt64(v)) }
			if v, ok := opts["parallelism"]; ok { parallelism = uint8(toInt64(v)) }
			if v, ok := opts["key_length"]; ok { keyLength = uint32(toInt64(v)) }
		}
		salt := make([]byte, 16)
		if _, err := crand.Read(salt); err != nil { throw(Value("hash_password: " + err.Error())) }
		key := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, keyLength)
		b64Salt := base64.RawStdEncoding.EncodeToString(salt)
		b64Key := base64.RawStdEncoding.EncodeToString(key)
		return Value(fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, memory, iterations, parallelism, b64Salt, b64Key))
	default:
		throw(Value("hash_password: unknown algorithm: " + algo + " (use bcrypt or argon2)"))
	}
	return null
}

func builtin_verify_password(args ...Value) Value {
	if len(args) < 2 { throw(Value("verify_password requires password and hash")) }
	password := valueToString(args[0])
	hashed := valueToString(args[1])
	if strings.HasPrefix(hashed, "$2a$") || strings.HasPrefix(hashed, "$2b$") || strings.HasPrefix(hashed, "$2y$") {
		err := bcrypt.CompareHashAndPassword([]byte(hashed), []byte(password))
		return Value(err == nil)
	}
	if strings.HasPrefix(hashed, "$argon2id$") {
		parts := strings.Split(hashed, "$")
		if len(parts) != 6 { return Value(false) }
		var memory uint32
		var iterations uint32
		var parallelism uint8
		_, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism)
		if err != nil { return Value(false) }
		salt, err := base64.RawStdEncoding.DecodeString(parts[4])
		if err != nil { return Value(false) }
		expectedKey, err := base64.RawStdEncoding.DecodeString(parts[5])
		if err != nil { return Value(false) }
		keyLength := uint32(len(expectedKey))
		actualKey := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, keyLength)
		return Value(subtle.ConstantTimeCompare(actualKey, expectedKey) == 1)
	}
	return Value(false)
}
`)
	}

	// Validation
	if c.usedBuiltins["validate"] || c.usedBuiltins["is_email"] || c.usedBuiltins["is_url"] || c.usedBuiltins["is_uuid"] || c.usedBuiltins["is_numeric"] {
		c.raw(`
var reEmail = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+/=?^_{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")
var reUUID = regexp.MustCompile("^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$")

func builtin_is_email(args ...Value) Value {
	if len(args) == 0 { return Value(false) }
	return Value(reEmail.MatchString(valueToString(args[0])))
}

func builtin_is_url(args ...Value) Value {
	if len(args) == 0 { return Value(false) }
	s := valueToString(args[0])
	return Value(strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://"))
}

func builtin_is_uuid(args ...Value) Value {
	if len(args) == 0 { return Value(false) }
	return Value(reUUID.MatchString(valueToString(args[0])))
}

func builtin_is_numeric(args ...Value) Value {
	if len(args) == 0 { return Value(false) }
	s := valueToString(args[0])
	_, errI := strconv.ParseInt(s, 10, 64)
	if errI == nil { return Value(true) }
	_, errF := strconv.ParseFloat(s, 64)
	return Value(errF == nil)
}

func builtin_validate(args ...Value) Value {
	if len(args) < 2 { throw(Value("validate requires data and schema")) }
	data, ok := args[0].(map[string]Value)
	if !ok { throw(Value("validate: first argument must be an object")) }
	schema, ok := args[1].(map[string]Value)
	if !ok { throw(Value("validate: second argument must be a schema object")) }

	errs := make(map[string]Value)
	for field, rulesVal := range schema {
		rulesStr := valueToString(rulesVal)
		rules := strings.Split(rulesStr, "|")
		val, exists := data[field]

		for _, rule := range rules {
			rule = strings.TrimSpace(rule)
			if rule == "" { continue }

			// Parse rule:param
			ruleName := rule
			ruleParam := ""
			if idx := strings.Index(rule, ":"); idx >= 0 {
				ruleName = rule[:idx]
				ruleParam = rule[idx+1:]
			}

			failed := ""
			switch ruleName {
			case "required":
				if !exists || val == nil || val == Value("") {
					failed = "required"
				}
			case "string":
				if exists && val != nil {
					if _, ok := val.(string); !ok {
						failed = "string"
					}
				}
			case "int":
				if exists && val != nil {
					if _, ok := val.(int64); !ok {
						if _, ok := val.(float64); ok {
							v := val.(float64)
							if v != float64(int64(v)) { failed = "int" }
						} else { failed = "int" }
					}
				}
			case "number":
				if exists && val != nil {
					switch val.(type) {
					case int64, float64:
					default: failed = "number"
					}
				}
			case "bool":
				if exists && val != nil {
					if _, ok := val.(bool); !ok { failed = "bool" }
				}
			case "array":
				if exists && val != nil {
					if _, ok := val.([]Value); !ok { failed = "array" }
				}
			case "object":
				if exists && val != nil {
					if _, ok := val.(map[string]Value); !ok { failed = "object" }
				}
			case "email":
				if exists && val != nil {
					if !reEmail.MatchString(valueToString(val)) { failed = "email" }
				}
			case "url":
				if exists && val != nil {
					s := valueToString(val)
					if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") { failed = "url" }
				}
			case "uuid":
				if exists && val != nil {
					if !reUUID.MatchString(valueToString(val)) { failed = "uuid" }
				}
			case "min":
				if exists && val != nil && ruleParam != "" {
					n, _ := strconv.ParseFloat(ruleParam, 64)
					switch v := val.(type) {
					case string:
						if float64(len(v)) < n { failed = rule }
					case int64:
						if float64(v) < n { failed = rule }
					case float64:
						if v < n { failed = rule }
					case []Value:
						if float64(len(v)) < n { failed = rule }
					}
				}
			case "max":
				if exists && val != nil && ruleParam != "" {
					n, _ := strconv.ParseFloat(ruleParam, 64)
					switch v := val.(type) {
					case string:
						if float64(len(v)) > n { failed = rule }
					case int64:
						if float64(v) > n { failed = rule }
					case float64:
						if v > n { failed = rule }
					case []Value:
						if float64(len(v)) > n { failed = rule }
					}
				}
			case "between":
				if exists && val != nil && ruleParam != "" {
					parts := strings.SplitN(ruleParam, ",", 2)
					if len(parts) == 2 {
						lo, _ := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
						hi, _ := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
						var fv float64
						switch v := val.(type) {
						case string: fv = float64(len(v))
						case int64: fv = float64(v)
						case float64: fv = v
						case []Value: fv = float64(len(v))
						}
						if fv < lo || fv > hi { failed = rule }
					}
				}
			case "in":
				if exists && val != nil && ruleParam != "" {
					opts := strings.Split(ruleParam, ",")
					sv := valueToString(val)
					found := false
					for _, o := range opts {
						if strings.TrimSpace(o) == sv { found = true; break }
					}
					if !found { failed = rule }
				}
			case "regex":
				if exists && val != nil && ruleParam != "" {
					re, err := regexp.Compile(ruleParam)
					if err == nil && !re.MatchString(valueToString(val)) { failed = rule }
				}
			}

			if failed != "" {
				errs[field] = Value(failed)
				break
			}
		}
	}
	if len(errs) == 0 { return null }
	return Value(errs)
}
`)
	}

	if c.usedBuiltins["server_stats"] {
		c.raw(`func builtin_server_stats() Value {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return Value(map[string]Value{
		"mem_alloc":       Value(int64(m.Alloc)),
		"mem_alloc_mb":    Value(math.Round(float64(m.Alloc) / 1024 / 1024 * 100) / 100),
		"mem_total":       Value(int64(m.TotalAlloc)),
		"mem_total_mb":    Value(math.Round(float64(m.TotalAlloc) / 1024 / 1024 * 100) / 100),
		"mem_sys":         Value(int64(m.Sys)),
		"mem_sys_mb":      Value(math.Round(float64(m.Sys) / 1024 / 1024 * 100) / 100),
		"mem_heap_inuse":  Value(int64(m.HeapInuse)),
		"mem_heap_idle":   Value(int64(m.HeapIdle)),
		"mem_stack":       Value(int64(m.StackInuse)),
		"gc_count":        Value(int64(m.NumGC)),
		"gc_pause_ms":     Value(math.Round(float64(m.PauseTotalNs) / 1e6 * 100) / 100),
		"goroutines":      Value(int64(runtime.NumGoroutine())),
		"cpus":            Value(int64(runtime.NumCPU())),
		"uptime":          Value(int64(time.Since(_startTime).Seconds())),
		"uptime_human":    Value(time.Since(_startTime).Round(time.Second).String()),
	})
}
`)
	}

	// exec() — shell command execution
	if c.hasExec {
		c.raw(`
func builtin_exec(args ...Value) Value {
	if len(args) == 0 { throw(Value("exec requires a command string")); return null }
	cmdStr := valueToString(args[0])
	timeoutSec := int64(30)
	if len(args) >= 2 { timeoutSec = toInt64(args[1]); if timeoutSec < 1 { timeoutSec = 1 } }

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSec)*time.Second)
	defer cancel()

	cmd := osexec.CommandContext(ctx, "sh", "-c", cmdStr)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	status := int64(0)
	if err != nil {
		if exitErr, ok := err.(*osexec.ExitError); ok {
			status = int64(exitErr.ExitCode())
		} else if ctx.Err() == context.DeadlineExceeded {
			status = int64(-1)
		} else {
			status = int64(-1)
		}
	}

	return Value(map[string]Value{
		"stdout": Value(stdout.String()),
		"stderr": Value(stderr.String()),
		"status": Value(status),
		"ok":     Value(status == 0),
	})
}
`)
	}

	c.ln("")
}

func (c *NativeCompiler) emitCronRuntime() {
	if !c.hasCron {
		return
	}
	c.raw(`
// ===== Cron Runtime =====
type cronSchedule struct {
	minutes []int
	hours   []int
	days    []int
	months  []int
	weekdays []int
}

func parseCron(expr string) cronSchedule {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		panic("cron: expected 5 fields (min hour dom month dow), got " + expr)
	}
	return cronSchedule{
		minutes:  parseCronField(fields[0], 0, 59),
		hours:    parseCronField(fields[1], 0, 23),
		days:     parseCronField(fields[2], 1, 31),
		months:   parseCronField(fields[3], 1, 12),
		weekdays: parseCronField(fields[4], 0, 6),
	}
}

func parseCronField(field string, min, max int) []int {
	var result []int
	for _, part := range strings.Split(field, ",") {
		step := 1
		if idx := strings.Index(part, "/"); idx >= 0 {
			step, _ = strconv.Atoi(part[idx+1:])
			part = part[:idx]
		}
		if part == "*" {
			for i := min; i <= max; i += step {
				result = append(result, i)
			}
		} else if idx := strings.Index(part, "-"); idx >= 0 {
			lo, _ := strconv.Atoi(part[:idx])
			hi, _ := strconv.Atoi(part[idx+1:])
			for i := lo; i <= hi; i += step {
				result = append(result, i)
			}
		} else {
			n, _ := strconv.Atoi(part)
			result = append(result, n)
		}
	}
	return result
}

func (cs cronSchedule) matches(t time.Time) bool {
	return containsInt(cs.minutes, t.Minute()) &&
		containsInt(cs.hours, t.Hour()) &&
		containsInt(cs.days, t.Day()) &&
		containsInt(cs.months, int(t.Month())) &&
		containsInt(cs.weekdays, int(t.Weekday()))
}

func containsInt(s []int, v int) bool {
	for _, n := range s { if n == v { return true } }
	return false
}

func cronRun(expr string, fn func()) {
	sched := parseCron(expr)
	go func() {
		// Align to next minute boundary
		now := time.Now()
		next := now.Truncate(time.Minute).Add(time.Minute)
		time.Sleep(next.Sub(now))
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		// Check immediately at first aligned minute
		if sched.matches(time.Now()) { fn() }
		for t := range ticker.C {
			if sched.matches(t) { fn() }
		}
	}()
}
`)
}

func (c *NativeCompiler) emitDBRuntime() {
	if len(c.dbDrivers) == 0 {
		// Emit stub dslDB type if sessions need it
		if c.sessionEnabled {
			c.raw(`
type dslDB struct {
	db     *sql.DB
	driver string
}
`)
		}
		return
	}
	needSQL := c.dbDrivers["sqlite"] || c.dbDrivers["postgres"] || c.dbDrivers["mysql"]
	needMongo := c.dbDrivers["mongo"]

	// Unified db.open dispatcher
	c.raw(`
// ===== Database Runtime =====
func dslDBOpen(args ...Value) Value {
	if len(args) < 2 { throw(Value("db.open requires driver and connection string")) }
	driver := valueToString(args[0])
	connStr := valueToString(args[1])
	switch driver {
`)
	if needSQL {
		c.raw(`	case "sqlite", "postgres", "mysql":
		return dslSQLOpen(driver, connStr)
`)
	}
	if needMongo {
		c.raw(`	case "mongo":
		return dslMongoOpen(Value(connStr))
`)
	}
	c.raw(`	default:
		throw(Value("db.open: unknown driver: " + driver))
	}
	return null
}
`)

	// Close dispatcher
	c.raw(`
func dslDBClose(args ...Value) Value {
	if len(args) < 1 { throw(Value("close requires db")) }
	switch d := args[0].(type) {
`)
	if needSQL {
		c.raw(`	case *dslDB:
		if err := d.db.Close(); err != nil { throw(Value("close: " + err.Error())) }
`)
	}
	if needMongo {
		c.raw(`	case *dslMongoDB:
		if err := d.client.Disconnect(context.TODO()); err != nil { throw(Value("close: " + err.Error())) }
`)
	}
	c.raw(`	default:
		throw(Value("close: not a database connection"))
	}
	return null
}
`)

	if needSQL {
		c.emitSQLRuntime()
	}
	if needMongo {
		c.emitMongoRuntime()
	}
}

func (c *NativeCompiler) emitSQLRuntime() {
	c.raw(`
type dslDB struct {
	db     *sql.DB
	driver string
}

func dslSQLOpen(driver, connStr string) Value {
	sqlDriver := driver
	switch driver {
	case "sqlite": sqlDriver = "sqlite"
	case "postgres": sqlDriver = "pgx"
	case "mysql": sqlDriver = "mysql"
	}
	db, err := sql.Open(sqlDriver, connStr)
	if err != nil { throw(Value("db.open: " + err.Error())) }
	if err := db.Ping(); err != nil { db.Close(); throw(Value("db.open: " + err.Error())) }
	return Value(&dslDB{db: db, driver: driver})
}

func dbParams(v Value) []interface{} {
	if v == nil || v == null { return nil }
	arr, ok := v.([]Value)
	if !ok { return nil }
	result := make([]interface{}, len(arr))
	for i, val := range arr {
		if val == nil || val == null { result[i] = nil } else {
			switch v := val.(type) {
			case int64: result[i] = v
			case float64: result[i] = v
			case bool: result[i] = v
			case string: result[i] = v
			default: result[i] = valueToString(val)
			}
		}
	}
	return result
}

func dslDBExec(args ...Value) Value {
	if len(args) < 2 { throw(Value("exec requires db and query")) }
	db, ok := args[0].(*dslDB)
	if !ok { throw(Value("exec: first argument must be a SQL database")) }
	query := valueToString(args[1])
	var params []interface{}
	if len(args) >= 3 { params = dbParams(args[2]) }
	result, err := db.db.Exec(query, params...)
	if err != nil { throw(Value("exec: " + err.Error())) }
	ra, _ := result.RowsAffected()
	li, _ := result.LastInsertId()
	return Value(map[string]Value{"rows_affected": Value(ra), "last_insert_id": Value(li)})
}

func dslDBQuery(args ...Value) Value {
	if len(args) < 2 { throw(Value("query requires db and query string")) }
	db, ok := args[0].(*dslDB)
	if !ok { throw(Value("query: first argument must be a SQL database")) }
	query := valueToString(args[1])
	var params []interface{}
	if len(args) >= 3 { params = dbParams(args[2]) }
	rows, err := db.db.Query(query, params...)
	if err != nil { throw(Value("query: " + err.Error())) }
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil { throw(Value("query: " + err.Error())) }
	var results []Value
	for rows.Next() {
		vals := make([]interface{}, len(columns))
		ptrs := make([]interface{}, len(columns))
		for i := range vals { ptrs[i] = &vals[i] }
		if err := rows.Scan(ptrs...); err != nil { throw(Value("query: " + err.Error())) }
		row := make(map[string]Value)
		for i, col := range columns {
			switch v := vals[i].(type) {
			case nil: row[col] = null
			case int64: row[col] = Value(v)
			case float64: row[col] = Value(v)
			case bool: row[col] = Value(v)
			case string: row[col] = Value(v)
			case []byte: row[col] = Value(string(v))
			default: row[col] = Value(fmt.Sprintf("%v", v))
			}
		}
		results = append(results, Value(row))
	}
	if err := rows.Err(); err != nil { throw(Value("query: " + err.Error())) }
	if results == nil { results = []Value{} }
	return Value(results)
}

func dslDBQueryOne(args ...Value) Value {
	result := dslDBQuery(args...)
	if arr, ok := result.([]Value); ok && len(arr) > 0 { return arr[0] }
	return null
}

func dslDBQueryValue(args ...Value) Value {
	result := dslDBQuery(args...)
	if arr, ok := result.([]Value); ok && len(arr) > 0 {
		if row, ok := arr[0].(map[string]Value); ok {
			for _, v := range row { return v }
		}
	}
	return null
}
`)
}

func (c *NativeCompiler) emitMongoRuntime() {
	c.raw(`
type dslMongoDB struct {
	client *mongo.Client
	db     *mongo.Database
}

func valueToBson(v Value) interface{} {
	if v == nil || v == null { return nil }
	switch val := v.(type) {
	case map[string]Value:
		m := bson.M{}
		for k, v := range val { m[k] = valueToBson(v) }
		return m
	case []Value:
		a := bson.A{}
		for _, item := range val { a = append(a, valueToBson(item)) }
		return a
	default: return val
	}
}

func bsonToValue(v interface{}) Value {
	if v == nil { return null }
	switch val := v.(type) {
	case bson.M:
		m := make(map[string]Value)
		for k, v := range val { m[k] = bsonToValue(v) }
		return Value(m)
	case bson.D:
		m := make(map[string]Value)
		for _, e := range val { m[e.Key] = bsonToValue(e.Value) }
		return Value(m)
	case bson.A:
		a := make([]Value, len(val))
		for i, item := range val { a[i] = bsonToValue(item) }
		return Value(a)
	case []interface{}:
		a := make([]Value, len(val))
		for i, item := range val { a[i] = bsonToValue(item) }
		return Value(a)
	case bson.ObjectID: return Value(val.Hex())
	case int32: return Value(int64(val))
	case int64: return Value(val)
	case float64: return Value(val)
	case float32: return Value(float64(val))
	case string: return Value(val)
	case bool: return Value(val)
	default: return Value(fmt.Sprintf("%v", val))
	}
}

func dslMongoOpen(connStr Value) Value {
	uri := valueToString(connStr)
	dbName := ""
	if idx := strings.LastIndex(uri, "/"); idx >= 0 && idx < len(uri)-1 {
		rest := uri[idx+1:]
		if q := strings.Index(rest, "?"); q >= 0 { dbName = rest[:q] } else { dbName = rest }
	}
	if dbName == "" { throw(Value("db.open: database name not found in MongoDB URI")) }
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil { throw(Value("db.open: " + err.Error())) }
	ctx := context.TODO()
	if err := client.Ping(ctx, nil); err != nil {
		client.Disconnect(ctx)
		throw(Value("db.open: " + err.Error()))
	}
	return Value(&dslMongoDB{client: client, db: client.Database(dbName)})
}

func dslMongoInsert(args ...Value) Value {
	if len(args) < 3 { throw(Value("insert requires db, collection, document")) }
	db, ok := args[0].(*dslMongoDB); if !ok { throw(Value("insert: not a MongoDB connection")) }
	coll := db.db.Collection(valueToString(args[1]))
	result, err := coll.InsertOne(context.TODO(), valueToBson(args[2]))
	if err != nil { throw(Value("insert: " + err.Error())) }
	id := fmt.Sprintf("%v", result.InsertedID)
	if oid, ok := result.InsertedID.(bson.ObjectID); ok { id = oid.Hex() }
	return Value(map[string]Value{"inserted_id": Value(id)})
}

func dslMongoInsertMany(args ...Value) Value {
	if len(args) < 3 { throw(Value("insert_many requires db, collection, documents")) }
	db, ok := args[0].(*dslMongoDB); if !ok { throw(Value("insert_many: not a MongoDB connection")) }
	docs, ok := args[2].([]Value); if !ok { throw(Value("insert_many: documents must be an array")) }
	bsonDocs := make([]interface{}, len(docs))
	for i, d := range docs { bsonDocs[i] = valueToBson(d) }
	coll := db.db.Collection(valueToString(args[1]))
	result, err := coll.InsertMany(context.TODO(), bsonDocs)
	if err != nil { throw(Value("insert_many: " + err.Error())) }
	ids := make([]Value, len(result.InsertedIDs))
	for i, id := range result.InsertedIDs {
		if oid, ok := id.(bson.ObjectID); ok { ids[i] = Value(oid.Hex()) } else { ids[i] = Value(fmt.Sprintf("%v", id)) }
	}
	return Value(map[string]Value{"inserted_ids": Value(ids)})
}

func dslMongoFind(args ...Value) Value {
	if len(args) < 3 { throw(Value("find requires db, collection, filter")) }
	db, ok := args[0].(*dslMongoDB); if !ok { throw(Value("find: not a MongoDB connection")) }
	coll := db.db.Collection(valueToString(args[1]))
	ctx := context.TODO()
	findOpts := options.Find()
	if len(args) >= 4 {
		if opts, ok := args[3].(map[string]Value); ok {
			if v, ok := opts["limit"]; ok { findOpts.SetLimit(toInt64(v)) }
			if v, ok := opts["skip"]; ok { findOpts.SetSkip(toInt64(v)) }
			if v, ok := opts["sort"]; ok { findOpts.SetSort(valueToBson(v)) }
		}
	}
	cursor, err := coll.Find(ctx, valueToBson(args[2]), findOpts)
	if err != nil { throw(Value("find: " + err.Error())) }
	defer cursor.Close(ctx)
	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil { throw(Value("find: " + err.Error())) }
	out := make([]Value, len(results))
	for i, r := range results { out[i] = bsonToValue(r) }
	return Value(out)
}

func dslMongoFindOne(args ...Value) Value {
	if len(args) < 3 { throw(Value("find_one requires db, collection, filter")) }
	db, ok := args[0].(*dslMongoDB); if !ok { throw(Value("find_one: not a MongoDB connection")) }
	coll := db.db.Collection(valueToString(args[1]))
	var result bson.M
	err := coll.FindOne(context.TODO(), valueToBson(args[2])).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments { return null }
		throw(Value("find_one: " + err.Error()))
	}
	return bsonToValue(result)
}

func dslMongoUpdate(args ...Value) Value {
	if len(args) < 4 { throw(Value("update requires db, collection, filter, update")) }
	db, ok := args[0].(*dslMongoDB); if !ok { throw(Value("update: not a MongoDB connection")) }
	coll := db.db.Collection(valueToString(args[1]))
	result, err := coll.UpdateMany(context.TODO(), valueToBson(args[2]), valueToBson(args[3]))
	if err != nil { throw(Value("update: " + err.Error())) }
	return Value(map[string]Value{"matched": Value(result.MatchedCount), "modified": Value(result.ModifiedCount)})
}

func dslMongoDelete(args ...Value) Value {
	if len(args) < 3 { throw(Value("delete requires db, collection, filter")) }
	db, ok := args[0].(*dslMongoDB); if !ok { throw(Value("delete: not a MongoDB connection")) }
	coll := db.db.Collection(valueToString(args[1]))
	result, err := coll.DeleteMany(context.TODO(), valueToBson(args[2]))
	if err != nil { throw(Value("delete: " + err.Error())) }
	return Value(map[string]Value{"deleted": Value(result.DeletedCount)})
}

func dslMongoCount(args ...Value) Value {
	if len(args) < 2 { throw(Value("count requires db, collection")) }
	db, ok := args[0].(*dslMongoDB); if !ok { throw(Value("count: not a MongoDB connection")) }
	coll := db.db.Collection(valueToString(args[1]))
	filter := bson.M{}
	if len(args) >= 3 { if f, ok := args[2].(map[string]Value); ok { filter = valueToBson(f).(bson.M) } }
	n, err := coll.CountDocuments(context.TODO(), filter)
	if err != nil { throw(Value("count: " + err.Error())) }
	return Value(n)
}
`)
}

func (c *NativeCompiler) emitSessionRuntime() {
	if !c.sessionEnabled {
		return
	}
	// Session secret expression
	secretExpr := fmt.Sprintf("%q", c.sessionSecret)
	if c.sessionSecretExpr != nil {
		secretExpr = fmt.Sprintf("valueToString(%s)", c.expr(c.sessionSecretExpr))
	}
	c.lnf("var _sessionSecret = %s", secretExpr)
	c.lnf("var _sessionExpires = int64(%d)", c.sessionExpires)
	c.lnf("var _sessionCookie = %q", c.sessionCookie)
	c.raw(`
// Session runtime
type sessionStore struct {
	mu          sync.RWMutex
	data        map[string]sessionEntry
	dirtyKeys   map[string]bool
	deletedKeys map[string]bool
	db          *dslDB
	tableName   string
}

type sessionEntry struct {
	data    map[string]Value
	expires int64
}

var _sessions = &sessionStore{
	data:        make(map[string]sessionEntry),
	dirtyKeys:   make(map[string]bool),
	deletedKeys: make(map[string]bool),
}

func sessionSign(id string) string {
	mac := hmac.New(sha256.New, []byte(_sessionSecret))
	mac.Write([]byte(id))
	sig := hex.EncodeToString(mac.Sum(nil))[:16]
	return id + "." + sig
}

func sessionVerify(cookie string) (string, bool) {
	parts := strings.SplitN(cookie, ".", 2)
	if len(parts) != 2 { return "", false }
	id := parts[0]
	expected := sessionSign(id)
	if !hmac.Equal([]byte(cookie), []byte(expected)) { return "", false }
	return id, true
}

func sessionLoad(r *http.Request) (string, map[string]Value, bool) {
	c, err := r.Cookie(_sessionCookie)
	if err != nil || c.Value == "" {
		return "", make(map[string]Value), false
	}
	id, ok := sessionVerify(c.Value)
	if !ok {
		return "", make(map[string]Value), false
	}
	_sessions.mu.RLock()
	entry, exists := _sessions.data[id]
	_sessions.mu.RUnlock()
	if !exists {
		return "", make(map[string]Value), false
	}
	if entry.expires > 0 && entry.expires < time.Now().Unix() {
		// Expired — clean up
		_sessions.mu.Lock()
		delete(_sessions.data, id)
		_sessions.deletedKeys[id] = true
		delete(_sessions.dirtyKeys, id)
		_sessions.mu.Unlock()
		return "", make(map[string]Value), false
	}
	// Return a copy
	copy := make(map[string]Value, len(entry.data))
	for k, v := range entry.data { copy[k] = v }
	return id, copy, true
}

func sessionSave(w http.ResponseWriter, id string, data map[string]Value, destroyed bool) {
	if destroyed {
		if id != "" {
			_sessions.mu.Lock()
			delete(_sessions.data, id)
			_sessions.deletedKeys[id] = true
			delete(_sessions.dirtyKeys, id)
			_sessions.mu.Unlock()
		}
		http.SetCookie(w, &http.Cookie{
			Name:     _sessionCookie,
			Value:    "",
			Path:     "/",
			MaxAge:   -1,
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
		})
		return
	}
	if len(data) == 0 { return }
	// Generate new session ID if needed
	if id == "" {
		b := make([]byte, 24)
		crand.Read(b)
		id = hex.EncodeToString(b)
	}
	_sessions.mu.Lock()
	_sessions.data[id] = sessionEntry{
		data:    data,
		expires: time.Now().Unix() + _sessionExpires,
	}
	_sessions.dirtyKeys[id] = true
	delete(_sessions.deletedKeys, id)
	_sessions.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     _sessionCookie,
		Value:    sessionSign(id),
		Path:     "/",
		MaxAge:   int(_sessionExpires),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func sessionFlush() {
	if _sessions.db == nil { return }
	_sessions.mu.Lock()
	if len(_sessions.dirtyKeys) == 0 && len(_sessions.deletedKeys) == 0 {
		_sessions.mu.Unlock()
		return
	}
	dirty := make(map[string]sessionEntry, len(_sessions.dirtyKeys))
	for k := range _sessions.dirtyKeys {
		if e, ok := _sessions.data[k]; ok { dirty[k] = e }
	}
	deleted := make(map[string]bool, len(_sessions.deletedKeys))
	for k := range _sessions.deletedKeys { deleted[k] = true }
	_sessions.dirtyKeys = make(map[string]bool)
	_sessions.deletedKeys = make(map[string]bool)
	_sessions.mu.Unlock()

	tx, err := _sessions.db.db.Begin()
	if err != nil {
		fmt.Fprintf(os.Stderr, "session flush error: %v\n", err)
		_sessions.mu.Lock()
		for k := range dirty { _sessions.dirtyKeys[k] = true }
		for k := range deleted { _sessions.deletedKeys[k] = true }
		_sessions.mu.Unlock()
		return
	}
	for k, e := range dirty {
		jBytes, _ := json.Marshal(valueToGo(Value(e.data)))
		switch _sessions.db.driver {
		case "postgres":
			tx.Exec(fmt.Sprintf("INSERT INTO %s (id, data, expires) VALUES ($1, $2, $3) ON CONFLICT (id) DO UPDATE SET data = $2, expires = $3", _sessions.tableName), k, string(jBytes), e.expires)
		case "mysql":
`)
	// MySQL with backtick-quoted table reference
	c.ln(`			tx.Exec("REPLACE INTO " + _sessions.tableName + " (id, data, expires) VALUES (?, ?, ?)", k, string(jBytes), e.expires)`)
	c.raw(`		default:
			tx.Exec(fmt.Sprintf("INSERT OR REPLACE INTO %s (id, data, expires) VALUES (?, ?, ?)", _sessions.tableName), k, string(jBytes), e.expires)
		}
	}
	for k := range deleted {
		switch _sessions.db.driver {
		case "postgres":
			tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE id = $1", _sessions.tableName), k)
		default:
			tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE id = ?", _sessions.tableName), k)
		}
	}
	tx.Commit()
}

func set_session_store(args ...Value) Value {
	if len(args) < 2 { throw(Value("set_session_store requires db and table name")); return null }
	dbVal, ok := args[0].(*dslDB)
	if !ok { throw(Value("set_session_store: first argument must be a database connection")); return null }
	tableName := valueToString(args[1])

	createSQL := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (id TEXT PRIMARY KEY, data TEXT NOT NULL, expires INTEGER NOT NULL)", tableName)
	if _, err := dbVal.db.Exec(createSQL); err != nil {
		throw(Value("set_session_store: create table: " + err.Error())); return null
	}

	// Load existing sessions
	rows, err := dbVal.db.Query(fmt.Sprintf("SELECT id, data, expires FROM %s", tableName))
	if err != nil { throw(Value("set_session_store: load: " + err.Error())); return null }
	defer rows.Close()
	now := time.Now().Unix()
	_sessions.mu.Lock()
	for rows.Next() {
		var id, data string
		var expires int64
		if err := rows.Scan(&id, &data, &expires); err != nil { continue }
		if expires > 0 && expires < now { continue } // skip expired
		var parsed interface{}
		if err := json.Unmarshal([]byte(data), &parsed); err != nil { continue }
		if m, ok := goToValue(parsed).(map[string]Value); ok {
			_sessions.data[id] = sessionEntry{data: m, expires: expires}
		}
	}
	_sessions.db = dbVal
	_sessions.tableName = tableName
	_sessions.mu.Unlock()

	// Flush goroutine
	flushSec := int64(5)
	if len(args) >= 3 { flushSec = toInt64(args[2]); if flushSec < 1 { flushSec = 1 } }
	go func() {
		ticker := time.NewTicker(time.Duration(flushSec) * time.Second)
		defer ticker.Stop()
		cleanupCounter := 0
		for range ticker.C {
			sessionFlush()
			cleanupCounter++
			if cleanupCounter >= 60 { // every ~5 minutes at 5s interval
				cleanupCounter = 0
				// Sweep expired sessions from memory
				now := time.Now().Unix()
				_sessions.mu.Lock()
				for id, e := range _sessions.data {
					if e.expires > 0 && e.expires < now {
						delete(_sessions.data, id)
						_sessions.deletedKeys[id] = true
					}
				}
				_sessions.mu.Unlock()
				// DB cleanup
				if _sessions.db != nil {
					switch _sessions.db.driver {
					case "postgres":
						_sessions.db.db.Exec(fmt.Sprintf("DELETE FROM %s WHERE expires < $1", _sessions.tableName), now)
					default:
						_sessions.db.db.Exec(fmt.Sprintf("DELETE FROM %s WHERE expires < ?", _sessions.tableName), now)
					}
				}
			}
		}
	}()

	return null
}
`)

	// CSRF runtime
	if c.csrfEnabled {
		c.emitCSRFRuntime()
	}
}

func (c *NativeCompiler) emitCSRFRuntime() {
	// Emit safe origins slice
	originsStr := "var _csrfSafeOrigins = []string{}"
	if len(c.csrfSafeOrigins) > 0 {
		parts := make([]string, len(c.csrfSafeOrigins))
		for i, o := range c.csrfSafeOrigins {
			parts[i] = fmt.Sprintf("%q", o)
		}
		originsStr = fmt.Sprintf("var _csrfSafeOrigins = []string{%s}", strings.Join(parts, ", "))
	}
	c.ln(originsStr)
	c.raw(`
func csrfToken(sessData map[string]Value) Value {
	if tok, ok := sessData["_csrf_token"]; ok {
		if s, ok := tok.(string); ok && s != "" {
			return Value(s)
		}
	}
	// Generate new token
	b := make([]byte, 32)
	crand.Read(b)
	tok := hex.EncodeToString(b)
	sessData["_csrf_token"] = Value(tok)
	return Value(tok)
}

func csrfField(sessData map[string]Value) Value {
	tok := valueToString(csrfToken(sessData))
	return Value(fmt.Sprintf(` + "`" + `<input type="hidden" name="_csrf" value="%s">` + "`" + `, tok))
}

func csrfValidate(r *http.Request, sessData map[string]Value, bodyStr string) bool {
	// Safe methods don't need CSRF
	switch r.Method {
	case "GET", "HEAD", "OPTIONS", "TRACE":
		return true
	}

	// Check safe origins
	if len(_csrfSafeOrigins) > 0 {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = r.Header.Get("Referer")
		}
		for _, safe := range _csrfSafeOrigins {
			if strings.HasPrefix(origin, safe) {
				return true
			}
		}
	}

	// Get expected token from session
	expected := ""
	if tok, ok := sessData["_csrf_token"]; ok {
		expected = valueToString(tok)
	}
	if expected == "" {
		return false
	}

	// Check token from header first
	submitted := r.Header.Get("X-CSRF-Token")
	if submitted == "" {
		submitted = r.Header.Get("X-XSRF-Token")
	}
	// Check URL query parameter
	if submitted == "" {
		submitted = r.URL.Query().Get("_csrf")
	}
	// Check form body (already read)
	if submitted == "" && bodyStr != "" {
		vals, err := url.ParseQuery(bodyStr)
		if err == nil {
			submitted = vals.Get("_csrf")
		}
		// Also check JSON body
		if submitted == "" {
			var jsonBody map[string]interface{}
			if json.Unmarshal([]byte(bodyStr), &jsonBody) == nil {
				if tok, ok := jsonBody["_csrf"]; ok {
					submitted = fmt.Sprintf("%v", tok)
				}
			}
		}
	}

	return hmac.Equal([]byte(submitted), []byte(expected))
}
`)
}

func (c *NativeCompiler) loadTemplateFiles() error {
	c.templateFiles = make(map[string]string)
	// Resolve templates dir relative to the source file directory
	baseDir := c.templatesDir
	return filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".gohtml" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		// Key is the relative path from templates dir
		rel, err := filepath.Rel(baseDir, path)
		if err != nil {
			return err
		}
		// Normalize to forward slashes
		rel = filepath.ToSlash(rel)
		c.templateFiles[rel] = string(content)
		return nil
	})
}

func (c *NativeCompiler) emitTemplateRuntime() {
	if len(c.templateFiles) == 0 {
		return
	}

	// Emit embedded template contents as constants
	c.ln("// ===== Template Runtime =====")
	c.ln("var _templateSources = map[string]string{")
	c.indent++
	// Sort keys for deterministic output
	keys := make([]string, 0, len(c.templateFiles))
	for k := range c.templateFiles {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, name := range keys {
		c.lnf("%q: %q,", name, c.templateFiles[name])
	}
	c.indent--
	c.ln("}")
	c.ln("")

	c.raw(`var _templates *template.Template

func _initTemplates() {
	funcMap := template.FuncMap{
		"safe": func(s interface{}) template.HTML {
			switch v := s.(type) {
			case string: return template.HTML(v)
			case template.HTML: return v
			default: return template.HTML(fmt.Sprint(v))
			}
		},`)
	if c.csrfEnabled {
		c.raw(`
		"csrf_field": func() template.HTML { return "" },
		"csrf_token": func() string { return "" },`)
	}
	c.raw(`
	}
	t := template.New("").Funcs(funcMap)
	for name, src := range _templateSources {
		template.Must(t.New(name).Parse(src))
	}
	_templates = t
}

func _render(templateName string, page Value, request Value`)
	if c.csrfEnabled {
		c.raw(`, sessData map[string]Value`)
	}
	c.raw(`) Value {
	data := map[string]interface{}{
		"Page":    valueToGo(page),
		"Request": valueToGo(request),
	}
	// Clone template and set csrf funcs with current session data
	t, err := _templates.Clone()
	if err != nil {
		throw(Value("render: clone templates: " + err.Error()))
		return null
	}`)
	if c.csrfEnabled {
		c.raw(`
	t.Funcs(template.FuncMap{
		"csrf_field": func() template.HTML {
			tok := valueToString(csrfToken(sessData))
			return template.HTML(fmt.Sprintf(` + "`" + `<input type="hidden" name="_csrf" value="%s">` + "`" + `, template.HTMLEscapeString(tok)))
		},
		"csrf_token": func() string {
			return valueToString(csrfToken(sessData))
		},
	})`)
	}
	c.raw(`
	var buf bytes.Buffer
	tmpl := t.Lookup(templateName)
	if tmpl == nil {
		throw(Value("render: template not found: " + templateName))
		return null
	}
	if err := tmpl.Execute(&buf, data); err != nil {
		throw(Value("render: " + err.Error()))
		return null
	}
	return Value(buf.String())
}
`)
}

func (c *NativeCompiler) emitSSERuntime() {
	if !c.hasSSE {
		return
	}
	c.raw(`
// ===== SSE Runtime =====
type sseClient struct {
	ch       chan sseEvent
	channels map[string]bool
}

type sseEvent struct {
	event string
	data  string
}

type sseHub struct {
	mu      sync.RWMutex
	clients map[*sseClient]bool
}

var _sseHub = &sseHub{
	clients: make(map[*sseClient]bool),
}

func (h *sseHub) register(client *sseClient) {
	h.mu.Lock()
	h.clients[client] = true
	h.mu.Unlock()
}

func (h *sseHub) unregister(client *sseClient) {
	h.mu.Lock()
	delete(h.clients, client)
	h.mu.Unlock()
	close(client.ch)
}

func (h *sseHub) broadcast(event string, data string, channel string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for client := range h.clients {
		if client.channels[channel] {
			select {
			case client.ch <- sseEvent{event: event, data: data}:
			default:
				// Drop if client buffer full
			}
		}
	}
}

func builtin_broadcast(args ...Value) Value {
	if len(args) < 2 { return null }
	event := valueToString(args[0])
	data := ""
	if args[1] != nil {
		jBytes, err := json.Marshal(valueToGo(args[1]))
		if err == nil { data = string(jBytes) } else { data = valueToString(args[1]) }
	}
	channel := "global"
	if len(args) >= 3 { channel = valueToString(args[2]) }
	_sseHub.broadcast(event, data, channel)
	return null
}

func _sseClientSend(client *sseClient, event string, data Value) Value {
	d := ""
	if data != nil {
		jBytes, err := json.Marshal(valueToGo(data))
		if err == nil { d = string(jBytes) } else { d = valueToString(data) }
	}
	client.ch <- sseEvent{event: event, data: d}
	return null
}

func _sseClientJoin(client *sseClient, channel string) Value {
	client.channels[channel] = true
	return null
}
`)
}

func (c *NativeCompiler) emitUserFunctions() {
	c.ln("// ===== User Functions =====")
	for _, fn := range c.functions {
		c.emitFnDef(fn)
	}
	c.ln("")
}

func (c *NativeCompiler) emitFnDef(fn *FnStatement) {
	tenv := c.fnTypes[fn.Name]
	c.typeEnv = tenv

	// Check if ALL params and return are typed → emit a typed function
	allTyped := tenv != nil && tenv.retType.IsTyped()
	if allTyped {
		for _, p := range fn.Params {
			if !tenv.Get(p).IsTyped() {
				allTyped = false
				break
			}
		}
	}
	if allTyped {
		// Also check that all locals are typed
		vars := c.collectVars(fn.Body)
		for name := range vars {
			t := tenv.Get(name)
			if !t.IsTyped() {
				allTyped = false
				break
			}
		}
	}
	if allTyped {
		// If function references any global vars, force untyped (globals are Value)
		refs := c.collectRefs(fn.Body)
		for name := range refs {
			if c.globalVars[name] {
				allTyped = false
				break
			}
		}
	}

	if allTyped {
		c.emitTypedFn(fn, tenv)
	} else {
		c.emitUntypedFn(fn, tenv)
	}
	c.typeEnv = nil
}

// emitTypedFn generates a function with concrete Go types (no interface{} boxing)
func (c *NativeCompiler) emitTypedFn(fn *FnStatement, tenv *TypeEnv) {
	params := make([]string, len(fn.Params))
	paramSet := make(map[string]bool)
	for i, p := range fn.Params {
		params[i] = fmt.Sprintf("%s %s", safeIdent(p), tenv.Get(p).String())
		paramSet[p] = true
	}
	retType := tenv.retType.String()
	c.lnf("func fn_%s_typed(%s) %s {", safeIdent(fn.Name), strings.Join(params, ", "), retType)
	c.indent++
	vars := c.collectVars(fn.Body)
	for name := range vars {
		if !paramSet[name] && !c.globalVars[name] {
			t := tenv.Get(name)
			switch t {
			case TypeInt:
				c.lnf("var %s int64", safeIdent(name))
			case TypeFloat:
				c.lnf("var %s float64", safeIdent(name))
			case TypeString:
				c.lnf("var %s string", safeIdent(name))
			case TypeBool:
				c.lnf("var %s bool", safeIdent(name))
			}
		}
	}
	c.emitTypedBlock(fn.Body)
	// zero-value return as fallback
	switch tenv.retType {
	case TypeInt:
		c.ln("return 0")
	case TypeFloat:
		c.ln("return 0")
	case TypeString:
		c.ln(`return ""`)
	case TypeBool:
		c.ln("return false")
	}
	c.indent--
	c.ln("}")
	// Also emit a Value wrapper so it can be called from untyped code
	params2 := make([]string, len(fn.Params))
	for i, p := range fn.Params {
		params2[i] = fmt.Sprintf("%s Value", safeIdent(p))
	}
	c.lnf("func fn_%s(%s) Value {", safeIdent(fn.Name), strings.Join(params2, ", "))
	c.indent++
	// Convert args and call typed version
	callArgs := make([]string, len(fn.Params))
	for i, p := range fn.Params {
		t := tenv.Get(p)
		switch t {
		case TypeInt:
			callArgs[i] = fmt.Sprintf("toInt64(%s)", safeIdent(p))
		case TypeFloat:
			callArgs[i] = fmt.Sprintf("toFloat64v(%s)", safeIdent(p))
		case TypeString:
			callArgs[i] = fmt.Sprintf("valueToString(%s)", safeIdent(p))
		case TypeBool:
			callArgs[i] = fmt.Sprintf("isTruthy(%s)", safeIdent(p))
		default:
			callArgs[i] = safeIdent(p)
		}
	}
	c.lnf("return Value(fn_%s_typed(%s))", safeIdent(fn.Name), strings.Join(callArgs, ", "))
	c.indent--
	c.ln("}")
	c.ln("")
}

func (c *NativeCompiler) emitTypedBlock(block *BlockStatement) {
	for _, stmt := range block.Statements {
		c.emitTypedStmt(stmt)
	}
}

func (c *NativeCompiler) emitTypedStmt(stmt Statement) {
	switch s := stmt.(type) {
	case *AssignStatement:
		if len(s.Names) == 1 && len(s.Values) == 1 {
			c.lnf("%s = %s", safeIdent(s.Names[0]), c.typedExpr(s.Values[0]))
		}
	case *CompoundAssignStatement:
		name := safeIdent(s.Name)
		switch s.Operator {
		case "+=":
			c.lnf("%s += %s", name, c.typedExpr(s.Value))
		case "-=":
			c.lnf("%s -= %s", name, c.typedExpr(s.Value))
		}
	case *ReturnStatement:
		if len(s.Values) == 1 {
			c.lnf("return %s", c.typedExpr(s.Values[0]))
		}
	case *IfStatement:
		c.lnf("if %s {", c.typedBoolExpr(s.Condition))
		c.indent++
		c.emitTypedBlock(s.Consequence)
		c.indent--
		if s.Alternative != nil {
			switch alt := s.Alternative.(type) {
			case *BlockStatement:
				c.ln("} else {")
				c.indent++
				c.emitTypedBlock(alt)
				c.indent--
			case *IfStatement:
				c.ln("} else {")
				c.indent++
				c.emitTypedStmt(alt)
				c.indent--
				c.ln("}")
				return
			}
		}
		c.ln("}")
	case *SwitchStatement:
		c.emitSwitchTyped(s)
	case *WhileStatement:
		c.lnf("for %s {", c.typedBoolExpr(s.Condition))
		c.indent++
		c.emitTypedBlock(s.Body)
		c.indent--
		c.ln("}")
	case *BreakStatement:
		c.ln("break")
	case *ContinueStatement:
		c.ln("continue")
	case *ExpressionStatement:
		c.lnf("_ = %s", c.typedExpr(s.Expression))
	}
}

func (c *NativeCompiler) emitSwitchTyped(s *SwitchStatement) {
	c.emitSwitch(s)
}

// typedExpr returns a Go expression with concrete types (no Value)
func (c *NativeCompiler) typedExpr(e Expression) string {
	if e == nil { return "0" }
	switch ex := e.(type) {
	case *IntegerLiteral:
		return fmt.Sprintf("int64(%d)", ex.Value)
	case *FloatLiteral:
		return fmt.Sprintf("float64(%s)", strconv.FormatFloat(ex.Value, 'f', -1, 64))
	case *StringLiteral:
		return fmt.Sprintf("%q", ex.Value)
	case *BooleanLiteral:
		if ex.Value { return "true" }
		return "false"
	case *Identifier:
		// If it's a typed local, use directly
		if c.typeEnv != nil {
			if c.typeEnv.Get(ex.Value).IsTyped() {
				return safeIdent(ex.Value)
			}
		}
		return safeIdent(ex.Value)
	case *InfixExpression:
		l := c.typedExpr(ex.Left)
		r := c.typedExpr(ex.Right)
		switch ex.Operator {
		case "+": return fmt.Sprintf("(%s + %s)", l, r)
		case "-": return fmt.Sprintf("(%s - %s)", l, r)
		case "*": return fmt.Sprintf("(%s * %s)", l, r)
		case "/": return fmt.Sprintf("(%s / %s)", l, r)
		case "%%": return fmt.Sprintf("(%s %% %s)", l, r)
		case "==": return fmt.Sprintf("(%s == %s)", l, r)
		case "!=": return fmt.Sprintf("(%s != %s)", l, r)
		case "<": return fmt.Sprintf("(%s < %s)", l, r)
		case ">": return fmt.Sprintf("(%s > %s)", l, r)
		case "<=": return fmt.Sprintf("(%s <= %s)", l, r)
		case ">=": return fmt.Sprintf("(%s >= %s)", l, r)
		case "&&": return fmt.Sprintf("(%s && %s)", l, r)
		case "||": return fmt.Sprintf("(%s || %s)", l, r)
		}
	case *PrefixExpression:
		switch ex.Operator {
		case "-": return fmt.Sprintf("(-%s)", c.typedExpr(ex.Right))
		case "!": return fmt.Sprintf("(!%s)", c.typedExpr(ex.Right))
		}
	case *CallExpression:
		if ident, ok := ex.Function.(*Identifier); ok {
			// Check if calling a typed function
			if tenv, ok := c.fnTypes[ident.Value]; ok && tenv.retType.IsTyped() {
				allParamsTyped := true
				for _, fn := range c.functions {
					if fn.Name == ident.Value {
						for _, p := range fn.Params {
							if !tenv.Get(p).IsTyped() { allParamsTyped = false }
						}
					}
				}
				if allParamsTyped {
					args := make([]string, len(ex.Arguments))
					for i, a := range ex.Arguments {
						args[i] = c.typedExpr(a)
					}
					return fmt.Sprintf("fn_%s_typed(%s)", safeIdent(ident.Value), strings.Join(args, ", "))
				}
			}
		}
		// Fall back to untyped
		return c.callExpr(ex)
	}
	// Fallback: use untyped expr
	return c.expr(e)
}

// typedBoolExpr returns a Go bool expression
func (c *NativeCompiler) typedBoolExpr(e Expression) string {
	et := inferExprType(e, c.typeEnv)
	if et == TypeBool {
		return c.typedExpr(e)
	}
	// Wrap in isTruthy for non-bool expressions
	return fmt.Sprintf("isTruthy(%s)", c.expr(e))
}

func (c *NativeCompiler) emitUntypedFn(fn *FnStatement, tenv *TypeEnv) {
	params := make([]string, len(fn.Params))
	paramSet := make(map[string]bool)
	for i, p := range fn.Params {
		params[i] = fmt.Sprintf("%s Value", safeIdent(p))
		paramSet[p] = true
	}
	c.lnf("func fn_%s(%s) Value {", safeIdent(fn.Name), strings.Join(params, ", "))
	c.indent++
	vars := c.collectVars(fn.Body)
	for name := range vars {
		if !paramSet[name] && !c.globalVars[name] {
			c.lnf("var %s Value = null", safeIdent(name))
		}
	}
	c.emitBlock(fn.Body, false)
	c.ln("return null")
	c.indent--
	c.ln("}")
	c.ln("")
}

func safeIdent(name string) string {
	// Avoid Go keywords
	switch name {
	case "type", "map", "func", "var", "range", "select", "case", "default", "chan", "go", "defer", "interface", "struct", "package", "import", "return", "break", "continue", "for", "if", "else", "switch":
		return "_" + name
	}
	return strings.ReplaceAll(name, "-", "_")
}

func (c *NativeCompiler) emitBlock(block *BlockStatement, isRoute bool) {
	for _, stmt := range block.Statements {
		c.emitStmt(stmt, isRoute)
	}
}

func (c *NativeCompiler) emitStmt(stmt Statement, isRoute bool) {
	switch s := stmt.(type) {
	case *AssignStatement:
		c.emitAssign(s, isRoute)
	case *CompoundAssignStatement:
		c.emitCompoundAssign(s)
	case *IndexAssignStatement:
		c.lnf("setIndex(%s, %s, %s)", c.expr(s.Left), c.expr(s.Index), c.expr(s.Value))
	case *ExpressionStatement:
		// Fire-and-forget async: bare "async doWork()" → go func()
		if ae, ok := s.Expression.(*AsyncExpression); ok {
			inner := c.expr(ae.Expression)
			c.lnf("go func() { _ = %s }()", inner)
		} else if c.isRenderCall(s.Expression) {
			c.emitRenderStmt(s.Expression)
		} else {
			c.lnf("_ = %s", c.expr(s.Expression))
		}
	case *ReturnStatement:
		if isRoute && len(s.Values) == 0 {
			// In route context, bare return sends the response and exits
			c.ln("writeResponse(_w, response)")
			c.ln("return")
		} else if isRoute && len(s.Values) == 1 {
			// return response (or return controllerFn(request, response))
			c.lnf("response = %s", c.expr(s.Values[0]))
			c.ln("writeResponse(_w, response)")
			c.ln("return")
		} else if len(s.Values) == 0 {
			c.ln("return null")
		} else if len(s.Values) == 1 {
			c.lnf("return %s", c.expr(s.Values[0]))
		} else {
			vals := make([]string, len(s.Values))
			for i, v := range s.Values {
				vals[i] = c.expr(v)
			}
			c.lnf("return &multiReturn{Values: []Value{%s}}", strings.Join(vals, ", "))
		}
	case *TryCatchStatement:
		c.emitTryCatch(s, isRoute)
	case *ThrowStatement:
		c.lnf("throw(%s)", c.expr(s.Value))
	case *IfStatement:
		c.emitIf(s, isRoute)
	case *WhileStatement:
		c.emitWhile(s, isRoute)
	case *EachStatement:
		c.emitEach(s, isRoute)
	case *SwitchStatement:
		c.emitSwitch(s)
	case *BreakStatement:
		c.ln("break")
	case *ContinueStatement:
		c.ln("continue")
	case *BlockStatement:
		c.emitBlock(s, isRoute)
	case *FnStatement:
		// Nested function — emit as closure var
		params := make([]string, len(s.Params))
		for i, p := range s.Params {
			params[i] = fmt.Sprintf("%s Value", safeIdent(p))
		}
		c.lnf("%s := func(%s) Value {", safeIdent(s.Name), strings.Join(params, ", "))
		c.indent++
		c.emitBlock(s.Body, false)
		c.ln("return null")
		c.indent--
		c.ln("}")
	case *ObjectDestructureStatement:
		tmp := c.tmp()
		c.lnf("%s := %s", tmp, c.expr(s.Value))
		for _, key := range s.Keys {
			c.lnf("%s = dotValue(%s, %q)", safeIdent(key), tmp, key)
		}
	case *ArrayDestructureStatement:
		tmp := c.tmp()
		c.lnf("%s := %s", tmp, c.expr(s.Value))
		for i, name := range s.Names {
			c.lnf("%s = indexValue(%s, int64(%d))", safeIdent(name), tmp, i)
		}
	}
}

func (c *NativeCompiler) isRenderCall(e Expression) bool {
	call, ok := e.(*CallExpression)
	if !ok { return false }
	ident, ok := call.Function.(*Identifier)
	return ok && ident.Value == "render"
}

func (c *NativeCompiler) emitRenderStmt(e Expression) {
	call := e.(*CallExpression)
	args := call.Arguments
	if len(args) < 1 {
		c.ln(`throw(Value("render requires a template name"))`)
		return
	}
	nameExpr := c.expr(args[0])
	pageExpr := "Value(map[string]Value{})"
	if len(args) >= 2 {
		pageExpr = c.expr(args[1])
	}
	if c.csrfEnabled && c.sessionEnabled {
		c.lnf(`setIndex(response, Value("body"), _render(valueToString(%s), %s, request, _sessData))`, nameExpr, pageExpr)
	} else {
		c.lnf(`setIndex(response, Value("body"), _render(valueToString(%s), %s, request))`, nameExpr, pageExpr)
	}
	c.ln(`setIndex(response, Value("type"), Value("html"))`)
}

func (c *NativeCompiler) emitTryCatch(s *TryCatchStatement, isRoute bool) {
	errVar := safeIdent(s.CatchVar)
	c.ln("func() {")
	c.indent++
	c.ln("defer func() {")
	c.indent++
	c.ln("if _r := recover(); _r != nil {")
	c.indent++
	c.lnf("var %s Value", errVar)
	c.ln("if _tv, ok := _r.(*throwValue); ok {")
	c.indent++
	c.lnf("%s = _tv.value", errVar)
	c.indent--
	c.ln("} else {")
	c.indent++
	c.lnf("%s = Value(fmt.Sprintf(\"%%v\", _r))", errVar)
	c.indent--
	c.ln("}")
	c.lnf("_ = %s", errVar)
	// Catch block vars are hoisted to enclosing scope, no need to declare here
	c.emitBlock(s.Catch, isRoute)
	c.indent--
	c.ln("}")
	c.indent--
	c.ln("}()")
	c.emitBlock(s.Try, isRoute)
	c.indent--
	c.ln("}()")
}

func (c *NativeCompiler) emitAssign(s *AssignStatement, isRoute bool) {
	if len(s.Names) == 1 && len(s.Values) == 1 {
		name := safeIdent(s.Names[0])
		c.lnf("%s = %s", name, c.expr(s.Values[0]))
	} else if len(s.Names) > 1 && len(s.Values) == 1 {
		// Destructuring: x, y = fn()
		tmp := c.tmp()
		c.lnf("%s := %s", tmp, c.expr(s.Values[0]))
		c.lnf("if _mr, ok := %s.(*multiReturn); ok {", tmp)
		c.indent++
		for i, name := range s.Names {
			c.lnf("if len(_mr.Values) > %d { %s = _mr.Values[%d] }", i, safeIdent(name), i)
		}
		c.indent--
		c.lnf("} else {")
		c.indent++
		c.lnf("%s = %s", safeIdent(s.Names[0]), tmp)
		for i := 1; i < len(s.Names); i++ {
			c.lnf("%s = null", safeIdent(s.Names[i]))
		}
		c.indent--
		c.ln("}")
	} else {
		for i, name := range s.Names {
			if i < len(s.Values) {
				c.lnf("%s = %s", safeIdent(name), c.expr(s.Values[i]))
			}
		}
	}
}

func (c *NativeCompiler) emitCompoundAssign(s *CompoundAssignStatement) {
	name := safeIdent(s.Name)
	switch s.Operator {
	case "+=":
		c.lnf("%s = addValues(%s, %s)", name, name, c.expr(s.Value))
	case "-=":
		c.lnf("%s = subtractValues(%s, %s)", name, name, c.expr(s.Value))
	default:
		c.lnf("%s = addValues(%s, %s)", name, name, c.expr(s.Value))
	}
}

func (c *NativeCompiler) emitSwitch(s *SwitchStatement) {
	subj := c.tmp()
	c.lnf("%s := %s", subj, c.expr(s.Subject))
	for i, cs := range s.Cases {
		kw := "if"
		if i > 0 { kw = "} else if" }
		conds := make([]string, len(cs.Values))
		for j, v := range cs.Values {
			conds[j] = fmt.Sprintf("valuesEqual(%s, %s)", subj, c.expr(v))
		}
		c.lnf("%s %s {", kw, strings.Join(conds, " || "))
		c.indent++
		c.emitBlock(cs.Body, true)
		c.indent--
	}
	if s.Default != nil {
		if len(s.Cases) > 0 {
			c.ln("} else {")
		} else {
			c.ln("{")
		}
		c.indent++
		c.emitBlock(s.Default, true)
		c.indent--
	}
	if len(s.Cases) > 0 || s.Default != nil {
		c.ln("}")
	}
}

func (c *NativeCompiler) emitIf(s *IfStatement, isRoute bool) {
	c.lnf("if isTruthy(%s) {", c.expr(s.Condition))
	c.indent++
	c.emitBlock(s.Consequence, isRoute)
	c.indent--
	if s.Alternative != nil {
		switch alt := s.Alternative.(type) {
		case *IfStatement:
			c.ln("} else {")
			c.indent++
			c.emitIf(alt, isRoute)
			c.indent--
			c.ln("}")
			return
		case *BlockStatement:
			c.ln("} else {")
			c.indent++
			c.emitBlock(alt, isRoute)
			c.indent--
		}
	}
	c.ln("}")
}

func (c *NativeCompiler) emitWhile(s *WhileStatement, isRoute bool) {
	c.lnf("for isTruthy(%s) {", c.expr(s.Condition))
	c.indent++
	c.emitBlock(s.Body, isRoute)
	c.indent--
	c.ln("}")
}

func (c *NativeCompiler) emitEach(s *EachStatement, isRoute bool) {
	iterVar := c.tmp()
	c.lnf("%s := %s", iterVar, c.expr(s.Iterable))
	valName := safeIdent(s.Value)
	idxName := "_"
	if s.Index != "" {
		idxName = safeIdent(s.Index)
	}

	c.lnf("switch _col := %s.(type) {", iterVar)
	c.ln("case []Value:")
	c.indent++
	if s.Index != "" {
		c.lnf("for _i, _v := range _col { %s = _v; %s = int64(_i)", valName, idxName)
	} else {
		c.lnf("for _, _v := range _col { %s = _v", valName)
	}
	c.indent++
	c.emitBlock(s.Body, isRoute)
	c.indent--
	c.ln("}")
	c.indent--
	c.ln("case map[string]Value:")
	c.indent++
	if s.Index != "" {
		c.lnf("for _k, _v := range _col { %s = _k; %s = _v", valName, idxName)
	} else {
		c.lnf("for _k, _ := range _col { %s = _k", valName)
	}
	c.indent++
	c.emitBlock(s.Body, isRoute)
	c.indent--
	c.ln("}")
	c.indent--
	c.ln("}")
	// suppress unused
	c.lnf("_ = %s", iterVar)
	if idxName != "_" {
		c.lnf("_ = %s", idxName)
	}
	c.lnf("_ = %s", valName)
}

// expr compiles an expression to a Go expression string
func (c *NativeCompiler) expr(e Expression) string {
	if e == nil {
		return "null"
	}
	switch ex := e.(type) {
	case *IntegerLiteral:
		return fmt.Sprintf("Value(int64(%d))", ex.Value)
	case *FloatLiteral:
		return fmt.Sprintf("Value(float64(%s))", strconv.FormatFloat(ex.Value, 'f', -1, 64))
	case *StringLiteral:
		return fmt.Sprintf("Value(%q)", ex.Value)
	case *BooleanLiteral:
		if ex.Value {
			return "Value(true)"
		}
		return "Value(false)"
	case *NullLiteral:
		return "null"
	case *Identifier:
		return c.identExpr(ex.Value)
	case *PrefixExpression:
		return c.prefixExpr(ex)
	case *InfixExpression:
		return c.infixExpr(ex)
	case *TernaryExpression:
		return fmt.Sprintf("ternary(%s, %s, %s)", c.expr(ex.Condition), c.expr(ex.Consequence), c.expr(ex.Alternative))
	case *CallExpression:
		return c.callExpr(ex)
	case *DotExpression:
		return c.dotExpr(ex)
	case *IndexExpression:
		return fmt.Sprintf("indexValue(%s, %s)", c.expr(ex.Left), c.expr(ex.Index))
	case *ArrayLiteral:
		if len(ex.Elements) == 0 {
			return "Value([]Value{})"
		}
		elems := make([]string, len(ex.Elements))
		for i, el := range ex.Elements {
			elems[i] = c.expr(el)
		}
		return fmt.Sprintf("Value([]Value{%s})", strings.Join(elems, ", "))
	case *HashLiteral:
		if len(ex.Pairs) == 0 {
			return "Value(map[string]Value{})"
		}
		pairs := make([]string, len(ex.Pairs))
		for i, p := range ex.Pairs {
			key := c.hashKeyStr(p.Key)
			pairs[i] = fmt.Sprintf("%s: %s", key, c.expr(p.Value))
		}
		return fmt.Sprintf("Value(map[string]Value{%s})", strings.Join(pairs, ", "))
	case *FunctionLiteral:
		// Anonymous function — always variadic so callValue can invoke
		var fb strings.Builder
		old := c.b
		c.b = fb
		// Unpack args into named params
		for i, p := range ex.Params {
			c.lnf("var %s Value = null", safeIdent(p))
			c.lnf("if len(_args) > %d { %s = _args[%d] }", i, safeIdent(p), i)
		}
		c.emitBlock(ex.Body, false)
		c.b.WriteString(strings.Repeat("\t", c.indent+1))
		c.b.WriteString("return null\n")
		body := c.b.String()
		c.b = old
		return fmt.Sprintf("Value(func(_args ...Value) Value {\n%s%s})", body, strings.Repeat("\t", c.indent))
	case *AsyncExpression:
		inner := c.expr(ex.Expression)
		tmp := c.tmp()
		return fmt.Sprintf("func() Value { %s := &dslFuture{ch: make(chan Value, 1)}; go func() { %s.ch <- %s }(); return %s }()", tmp, tmp, inner, tmp)
	}
	return "null"
}

func (c *NativeCompiler) hashKeyStr(e Expression) string {
	switch k := e.(type) {
	case *StringLiteral:
		return fmt.Sprintf("%q", k.Value)
	case *Identifier:
		return fmt.Sprintf("%q", k.Value)
	default:
		return fmt.Sprintf("valueToString(%s)", c.expr(e))
	}
}

func (c *NativeCompiler) identExpr(name string) string {
	// Known builtins map to builtin_xxx functions
	builtinNames := map[string]bool{
		"print": true, "len": true, "str": true, "int": true, "float": true,
		"bool": true, "type": true, "append": true, "keys": true, "values": true,
		"contains": true, "has": true, "includes": true, "trim": true, "split": true,
		"join": true, "upper": true, "lower": true, "replace": true,
		"starts_with": true, "ends_with": true, "slice": true, "reverse": true,
		"unique": true, "merge": true, "delete": true, "index_of": true,
		"repeat": true, "flat": true, "sort": true, "sort_by": true,
		"regex_match": true, "regex_replace": true, "rand": true,
		"uuid": true, "cuid2": true, "abs": true, "ceil": true, "floor": true, "round": true,
		"base64_encode": true, "base64_decode": true, "url_encode": true, "url_decode": true,
		"hash": true, "hmac_hash": true, "hash_password": true, "verify_password": true,
		"validate": true, "is_email": true, "is_url": true, "is_uuid": true, "is_numeric": true,
		"log": true, "log_info": true, "log_warn": true, "log_error": true,
		"map": true, "filter": true, "reduce": true,
		"find": true, "some": true, "every": true, "count": true,
		"pluck": true, "group_by": true, "chunk": true, "range": true,
		"sum": true, "min": true, "max": true, "clamp": true,
		"pad_left": true, "pad_right": true, "truncate": true, "capitalize": true,
		"date": true, "date_format": true, "date_parse": true, "strtotime": true,
		"redirect": true,
		"server_stats": true,
		"set_session_store": true,
		"csrf_token": true,
		"csrf_field": true,
		"render": true,
		"broadcast": true,
		"exec": true,
	}
	if builtinNames[name] {
		// Return as a callable value - but since Go can't store these directly,
		// we handle them at call sites instead.
		// For non-call references, wrap in a func
		return fmt.Sprintf("builtin_%s", safeIdent(name))
	}
	// User functions
	for _, fn := range c.functions {
		if fn.Name == name {
			return "fn_" + safeIdent(name)
		}
	}
	return safeIdent(name)
}

func (c *NativeCompiler) prefixExpr(e *PrefixExpression) string {
	switch e.Operator {
	case "-":
		return fmt.Sprintf("subtractValues(int64(0), %s)", c.expr(e.Right))
	case "!":
		return fmt.Sprintf("Value(!isTruthy(%s))", c.expr(e.Right))
	}
	return c.expr(e.Right)
}

func (c *NativeCompiler) infixExpr(e *InfixExpression) string {
	l := c.expr(e.Left)
	r := c.expr(e.Right)
	switch e.Operator {
	case "+":
		return fmt.Sprintf("addValues(%s, %s)", l, r)
	case "-":
		return fmt.Sprintf("subtractValues(%s, %s)", l, r)
	case "*":
		return fmt.Sprintf("multiplyValues(%s, %s)", l, r)
	case "/":
		return fmt.Sprintf("divideValues(%s, %s)", l, r)
	case "%":
		return fmt.Sprintf("modValues(%s, %s)", l, r)
	case "==":
		return fmt.Sprintf("Value(valuesEqual(%s, %s))", l, r)
	case "!=":
		return fmt.Sprintf("Value(!valuesEqual(%s, %s))", l, r)
	case "<":
		return fmt.Sprintf("Value(compareLess(%s, %s))", l, r)
	case ">":
		return fmt.Sprintf("Value(compareLess(%s, %s))", r, l)
	case "<=":
		return fmt.Sprintf("Value(compareLess(%s, %s) || valuesEqual(%s, %s))", l, r, l, r)
	case ">=":
		return fmt.Sprintf("Value(compareLess(%s, %s) || valuesEqual(%s, %s))", r, l, l, r)
	case "&&":
		return fmt.Sprintf("logicalAnd(%s, %s)", l, r)
	case "||":
		return fmt.Sprintf("logicalOr(%s, %s)", l, r)
	case "??":
		return fmt.Sprintf("nullCoalesce(%s, %s)", l, r)
	}
	return "null"
}

func (c *NativeCompiler) callExpr(e *CallExpression) string {
	args := make([]string, len(e.Arguments))
	for i, a := range e.Arguments {
		args[i] = c.expr(a)
	}
	argStr := strings.Join(args, ", ")

	// Handle dot-call patterns: json.parse, json.stringify, store.*
	if dot, ok := e.Function.(*DotExpression); ok {
		// Nested dot: request.session.destroy()
		if c.sessionEnabled {
			if outerDot, ok := dot.Left.(*DotExpression); ok {
				if ident, ok := outerDot.Left.(*Identifier); ok && ident.Value == "request" && outerDot.Field == "session" && dot.Field == "destroy" {
					return "(func() Value { _sessDestroyed = true; return null })()"
				}
			}
		}
		if ident, ok := dot.Left.(*Identifier); ok {
			switch ident.Value {
			case "json":
				switch dot.Field {
				case "parse":
					return fmt.Sprintf("builtin_json_parse(%s)", argStr)
				case "stringify":
					return fmt.Sprintf("builtin_json_stringify(%s)", argStr)
				}
			case "file":
				switch dot.Field {
				case "read":
					return fmt.Sprintf("builtin_file_read(%s)", argStr)
				case "write":
					return fmt.Sprintf("builtin_file_write(%s)", argStr)
				case "append":
					return fmt.Sprintf("builtin_file_append(%s)", argStr)
				case "read_json":
					return fmt.Sprintf("builtin_file_read_json(%s)", argStr)
				case "write_json":
					return fmt.Sprintf("builtin_file_write_json(%s)", argStr)
				case "exists":
					return fmt.Sprintf("builtin_file_exists(%s)", argStr)
				case "delete":
					return fmt.Sprintf("builtin_file_delete(%s)", argStr)
				case "list":
					return fmt.Sprintf("builtin_file_list(%s)", argStr)
				case "mkdir":
					return fmt.Sprintf("builtin_file_mkdir(%s)", argStr)
				case "chmod":
					return fmt.Sprintf("builtin_file_chmod(%s)", argStr)
				}
			case "db":
				switch dot.Field {
				case "open":
					return fmt.Sprintf("dslDBOpen(%s)", argStr)
				}
			case "jwt":
				c.usedBuiltins["jwt"] = true
				switch dot.Field {
				case "sign":
					return fmt.Sprintf("builtin_jwt_sign(%s)", argStr)
				case "verify":
					return fmt.Sprintf("builtin_jwt_verify(%s)", argStr)
				}
			case "store":
				switch dot.Field {
				case "get":
					if len(args) >= 2 {
						return fmt.Sprintf("storeGet(valueToString(%s), %s)", args[0], args[1])
					} else if len(args) == 1 {
						return fmt.Sprintf("storeGet(valueToString(%s), null)", args[0])
					}
					return "null"
				case "set":
					if len(args) >= 3 {
						return fmt.Sprintf("storeSet(valueToString(%s), %s, toInt64(%s))", args[0], args[1], args[2])
					} else if len(args) >= 2 {
						return fmt.Sprintf("storeSet(valueToString(%s), %s, 0)", args[0], args[1])
					}
					return "null"
				case "delete":
					if len(args) >= 1 {
						return fmt.Sprintf("(func() Value { storeDelete(valueToString(%s)); return null })()", args[0])
					}
					return "null"
				case "has":
					if len(args) >= 1 {
						return fmt.Sprintf("Value(storeHas(valueToString(%s)))", args[0])
					}
					return "Value(false)"
				case "all":
					return "storeAll()"
				case "incr":
					if len(args) >= 3 {
						return fmt.Sprintf("Value(storeIncr(valueToString(%s), toInt64(%s), toInt64(%s)))", args[0], args[1], args[2])
					} else if len(args) >= 2 {
						return fmt.Sprintf("Value(storeIncr(valueToString(%s), toInt64(%s), 0))", args[0], args[1])
					} else if len(args) == 1 {
						return fmt.Sprintf("Value(storeIncr(valueToString(%s), 1, 0))", args[0])
					}
					return "Value(int64(0))"
				case "sync":
					return fmt.Sprintf("storeSync(%s)", argStr)
				}
			case "stream":
				switch dot.Field {
				case "send":
					if len(args) >= 2 {
						// stream.send(event, data)
						return fmt.Sprintf("_sseClientSend(_sseClient, valueToString(%s), %s)", args[0], args[1])
					} else if len(args) == 1 {
						// stream.send(data) — default event "message"
						return fmt.Sprintf(`_sseClientSend(_sseClient, "message", %s)`, args[0])
					}
					return "null"
				case "join":
					if len(args) >= 1 {
						return fmt.Sprintf("_sseClientJoin(_sseClient, valueToString(%s))", args[0])
					}
					return "null"
				}
			}
		}
		// Method calls on db handles: mydb.query(...) etc.
		if len(c.dbDrivers) > 0 {
			objExpr := c.expr(dot.Left)
			switch dot.Field {
			case "exec":
				return fmt.Sprintf("dslDBExec(%s, %s)", objExpr, argStr)
			case "query":
				return fmt.Sprintf("dslDBQuery(%s, %s)", objExpr, argStr)
			case "query_one":
				return fmt.Sprintf("dslDBQueryOne(%s, %s)", objExpr, argStr)
			case "query_value":
				return fmt.Sprintf("dslDBQueryValue(%s, %s)", objExpr, argStr)
			case "insert":
				return fmt.Sprintf("dslMongoInsert(%s, %s)", objExpr, argStr)
			case "insert_many":
				return fmt.Sprintf("dslMongoInsertMany(%s, %s)", objExpr, argStr)
			case "find":
				return fmt.Sprintf("dslMongoFind(%s, %s)", objExpr, argStr)
			case "find_one":
				return fmt.Sprintf("dslMongoFindOne(%s, %s)", objExpr, argStr)
			case "update":
				return fmt.Sprintf("dslMongoUpdate(%s, %s)", objExpr, argStr)
			case "delete":
				return fmt.Sprintf("dslMongoDelete(%s, %s)", objExpr, argStr)
			case "count":
				return fmt.Sprintf("dslMongoCount(%s, %s)", objExpr, argStr)
			case "close":
				return fmt.Sprintf("dslDBClose(%s)", objExpr)
			}
		}
	}

	// Handle known builtins by name
	if ident, ok := e.Function.(*Identifier); ok {
		switch ident.Value {
		case "print":
			return fmt.Sprintf("builtin_print(%s)", argStr)
		case "env":
			return fmt.Sprintf("builtin_env(%s)", argStr)
		case "await":
			return fmt.Sprintf("builtin_await(%s)", argStr)
		case "race":
			return fmt.Sprintf("builtin_race(%s)", argStr)
		case "sleep":
			return fmt.Sprintf("builtin_sleep(%s)", argStr)
		case "fetch":
			return fmt.Sprintf("builtin_fetch(%s)", argStr)
		case "now":
			return "Value(time.Now().Unix())"
		case "server_stats":
			return "builtin_server_stats()"
		case "set_session_store":
			return fmt.Sprintf("set_session_store(%s)", argStr)
		case "csrf_token":
			return "csrfToken(_sessData)"
		case "csrf_field":
			return "csrfField(_sessData)"
		case "render":
			if len(args) < 1 { return "null" }
			nameExpr := args[0]
			pageExpr := "Value(map[string]Value{})"
			if len(args) >= 2 { pageExpr = args[1] }
			if c.csrfEnabled && c.sessionEnabled {
				return fmt.Sprintf("_render(valueToString(%s), %s, request, _sessData)", nameExpr, pageExpr)
			}
			return fmt.Sprintf("_render(valueToString(%s), %s, request)", nameExpr, pageExpr)
		case "broadcast":
			return fmt.Sprintf("builtin_broadcast(%s)", argStr)
		case "exec":
			return fmt.Sprintf("builtin_exec(%s)", argStr)
		case "now_ms":
			return "Value(time.Now().UnixMilli())"
		case "len":
			return fmt.Sprintf("builtin_len(%s)", argStr)
		case "str":
			return fmt.Sprintf("builtin_str(%s)", argStr)
		case "int":
			return fmt.Sprintf("builtin_int(%s)", argStr)
		case "float":
			return fmt.Sprintf("builtin_float(%s)", argStr)
		case "bool":
			return fmt.Sprintf("builtin_bool(%s)", argStr)
		case "type":
			return fmt.Sprintf("builtin_type(%s)", argStr)
		case "append":
			return fmt.Sprintf("builtin_append(%s)", argStr)
		case "keys":
			return fmt.Sprintf("builtin_keys(%s)", argStr)
		case "values":
			return fmt.Sprintf("builtin_values(%s)", argStr)
		case "contains":
			return fmt.Sprintf("builtin_contains(%s)", argStr)
		case "has":
			return fmt.Sprintf("builtin_has(%s)", argStr)
		case "includes":
			return fmt.Sprintf("builtin_includes(%s)", argStr)
		case "trim":
			return fmt.Sprintf("builtin_trim(%s)", argStr)
		case "split":
			return fmt.Sprintf("builtin_split(%s)", argStr)
		case "join":
			return fmt.Sprintf("builtin_join(%s)", argStr)
		case "upper":
			return fmt.Sprintf("builtin_upper(%s)", argStr)
		case "lower":
			return fmt.Sprintf("builtin_lower(%s)", argStr)
		case "replace":
			return fmt.Sprintf("builtin_replace(%s)", argStr)
		case "starts_with":
			return fmt.Sprintf("builtin_starts_with(%s)", argStr)
		case "ends_with":
			return fmt.Sprintf("builtin_ends_with(%s)", argStr)
		case "slice":
			return fmt.Sprintf("builtin_slice(%s)", argStr)
		case "reverse":
			return fmt.Sprintf("builtin_reverse(%s)", argStr)
		case "unique":
			return fmt.Sprintf("builtin_unique(%s)", argStr)
		case "merge":
			return fmt.Sprintf("builtin_merge(%s)", argStr)
		case "delete":
			return fmt.Sprintf("builtin_delete(%s)", argStr)
		case "index_of":
			return fmt.Sprintf("builtin_index_of(%s)", argStr)
		case "repeat":
			return fmt.Sprintf("builtin_repeat(%s)", argStr)
		case "flat":
			return fmt.Sprintf("builtin_flat(%s)", argStr)
		case "sort":
			return fmt.Sprintf("builtin_sort(%s)", argStr)
		case "sort_by":
			return fmt.Sprintf("builtin_sort_by(%s)", argStr)
		case "regex_match":
			return fmt.Sprintf("builtin_regex_match(%s)", argStr)
		case "regex_replace":
			return fmt.Sprintf("builtin_regex_replace(%s)", argStr)
		case "rand":
			return fmt.Sprintf("builtin_rand(%s)", argStr)
		case "uuid":
			return fmt.Sprintf("builtin_uuid(%s)", argStr)
		case "cuid2":
			return fmt.Sprintf("builtin_cuid2(%s)", argStr)
		case "abs":
			return fmt.Sprintf("builtin_abs(%s)", argStr)
		case "ceil":
			return fmt.Sprintf("builtin_ceil(%s)", argStr)
		case "floor":
			return fmt.Sprintf("builtin_floor(%s)", argStr)
		case "round":
			return fmt.Sprintf("builtin_round(%s)", argStr)
		case "base64_encode":
			return fmt.Sprintf("builtin_base64_encode(%s)", argStr)
		case "base64_decode":
			return fmt.Sprintf("builtin_base64_decode(%s)", argStr)
		case "url_encode":
			return fmt.Sprintf("builtin_url_encode(%s)", argStr)
		case "url_decode":
			return fmt.Sprintf("builtin_url_decode(%s)", argStr)
		case "hash":
			return fmt.Sprintf("builtin_hash(%s)", argStr)
		case "hmac_hash":
			return fmt.Sprintf("builtin_hmac_hash(%s)", argStr)
		case "hash_password":
			return fmt.Sprintf("builtin_hash_password(%s)", argStr)
		case "verify_password":
			return fmt.Sprintf("builtin_verify_password(%s)", argStr)
		case "validate":
			return fmt.Sprintf("builtin_validate(%s)", argStr)
		case "is_email":
			return fmt.Sprintf("builtin_is_email(%s)", argStr)
		case "is_url":
			return fmt.Sprintf("builtin_is_url(%s)", argStr)
		case "is_uuid":
			return fmt.Sprintf("builtin_is_uuid(%s)", argStr)
		case "is_numeric":
			return fmt.Sprintf("builtin_is_numeric(%s)", argStr)
		case "log":
			if c.inRouteHandler {
				return fmt.Sprintf("dslLog(\"\", _r, %s)", argStr)
			}
			return fmt.Sprintf("dslLog(\"\", nil, %s)", argStr)
		case "log_info":
			if c.inRouteHandler {
				return fmt.Sprintf("dslLog(\"INFO\", _r, %s)", argStr)
			}
			return fmt.Sprintf("dslLog(\"INFO\", nil, %s)", argStr)
		case "log_warn":
			if c.inRouteHandler {
				return fmt.Sprintf("dslLog(\"WARN\", _r, %s)", argStr)
			}
			return fmt.Sprintf("dslLog(\"WARN\", nil, %s)", argStr)
		case "log_error":
			if c.inRouteHandler {
				return fmt.Sprintf("dslLog(\"ERROR\", _r, %s)", argStr)
			}
			return fmt.Sprintf("dslLog(\"ERROR\", nil, %s)", argStr)
		case "map":
			return fmt.Sprintf("builtin_map(%s)", argStr)
		case "filter":
			return fmt.Sprintf("builtin_filter(%s)", argStr)
		case "reduce":
			return fmt.Sprintf("builtin_reduce(%s)", argStr)
		case "find":
			return fmt.Sprintf("builtin_find(%s)", argStr)
		case "some":
			return fmt.Sprintf("builtin_some(%s)", argStr)
		case "every":
			return fmt.Sprintf("builtin_every(%s)", argStr)
		case "count":
			return fmt.Sprintf("builtin_count(%s)", argStr)
		case "pluck":
			return fmt.Sprintf("builtin_pluck(%s)", argStr)
		case "group_by":
			return fmt.Sprintf("builtin_group_by(%s)", argStr)
		case "chunk":
			return fmt.Sprintf("builtin_chunk(%s)", argStr)
		case "range":
			return fmt.Sprintf("builtin_range(%s)", argStr)
		case "sum":
			return fmt.Sprintf("builtin_sum(%s)", argStr)
		case "min":
			return fmt.Sprintf("builtin_min(%s)", argStr)
		case "max":
			return fmt.Sprintf("builtin_max(%s)", argStr)
		case "clamp":
			return fmt.Sprintf("builtin_clamp(%s)", argStr)
		case "pad_left":
			return fmt.Sprintf("builtin_pad_left(%s)", argStr)
		case "pad_right":
			return fmt.Sprintf("builtin_pad_right(%s)", argStr)
		case "truncate":
			return fmt.Sprintf("builtin_truncate(%s)", argStr)
		case "capitalize":
			return fmt.Sprintf("builtin_capitalize(%s)", argStr)
		case "date":
			return fmt.Sprintf("builtin_date(%s)", argStr)
		case "date_format":
			return fmt.Sprintf("builtin_date_format(%s)", argStr)
		case "date_parse":
			return fmt.Sprintf("builtin_date_parse(%s)", argStr)
		case "strtotime":
			return fmt.Sprintf("builtin_strtotime(%s)", argStr)
		case "redirect":
			return fmt.Sprintf("builtin_redirect(%s)", argStr)
		}

		// User-defined function
		for _, fn := range c.functions {
			if fn.Name == ident.Value {
				return fmt.Sprintf("fn_%s(%s)", safeIdent(ident.Value), argStr)
			}
		}
		// Unknown - might be a variable holding a function
		if argStr == "" {
			return fmt.Sprintf("callValue(%s)", safeIdent(ident.Value))
		}
		return fmt.Sprintf("callValue(%s, %s)", safeIdent(ident.Value), argStr)
	}

	// General case: expression call (e.g. fn(x){ ... }(arg))
	fnExpr := c.expr(e.Function)
	if argStr == "" {
		return fmt.Sprintf("callValue(%s)", fnExpr)
	}
	return fmt.Sprintf("callValue(%s, %s)", fnExpr, argStr)
}

func (c *NativeCompiler) dotExpr(e *DotExpression) string {
	// json and store are handled at call sites
	if ident, ok := e.Left.(*Identifier); ok {
		if ident.Value == "json" || ident.Value == "store" || ident.Value == "file" || ident.Value == "db" || ident.Value == "jwt" || ident.Value == "stream" {
			// These are namespace objects; the actual call is handled in callExpr
			return fmt.Sprintf("dotValue(%s, %q)", safeIdent(ident.Value), e.Field)
		}
	}
	return fmt.Sprintf("dotValue(%s, %q)", c.expr(e.Left), e.Field)
}

func (c *NativeCompiler) emitMain() {
	c.ln("// ===== Main =====")
	c.ln("func main() {")
	c.indent++
	// Run init blocks
	if len(c.initBlocks) > 0 {
		c.ln("// init blocks")
		for _, block := range c.initBlocks {
			c.emitBlock(block, false)
		}
		c.ln("")
	}

	if len(c.templateFiles) > 0 {
		c.ln("_initTemplates()")
		c.ln("")
	}
	c.ln("rt := &dslRouter{errorHandlers: make(map[int]http.HandlerFunc)}")
	if c.throttleRPS > 0 {
		c.lnf("rt.limiter = newRateLimiter(%d)", c.throttleRPS)
	}
	for _, sm := range c.staticMounts {
		prefix := sm.Prefix
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		c.lnf("rt.statics = append(rt.statics, staticMountEntry{")
		c.indent++
		c.lnf("prefix: %q,", prefix)
		c.lnf("handler: http.StripPrefix(%q, http.FileServer(noDirFS{http.Dir(filepath.Clean(%q))})),", prefix, sm.Dir)
		c.indent--
		c.ln("})") 
	}
	c.ln("")

	for _, route := range c.routes {
		if route.Method == "SSE" {
			c.emitSSERoute(route)
		} else {
			c.emitRoute(route)
		}
	}

	for _, eh := range c.errorHandlers {
		c.emitErrorHandler(eh)
	}

	// Scheduled tasks
	for i, ev := range c.everyBlocks {
		vars := c.collectVars(ev.Body)
		if ev.CronExpr != "" {
			// Cron expression
			c.lnf("cronRun(%q, func() { // cron task %d", ev.CronExpr, i)
			c.indent++
			c.ln("defer func() { if r := recover(); r != nil { fmt.Fprintf(os.Stderr, \"cron task panic: %v\\n\", r) } }()")
			for name := range vars {
				if !c.globalVars[name] {
					c.lnf("var %s Value = null", safeIdent(name))
				}
			}
			c.emitBlock(ev.Body, false)
			c.indent--
			c.ln("})") 
		} else {
			// Interval
			c.lnf("go func() { // every %ds", ev.Interval)
			c.indent++
			c.lnf("_ticker := time.NewTicker(%d * time.Second)", ev.Interval)
			c.ln("defer _ticker.Stop()")
			c.ln("for range _ticker.C {")
			c.indent++
			c.lnf("func() { // task %d", i)
			c.indent++
			c.ln("defer func() { if r := recover(); r != nil { fmt.Fprintf(os.Stderr, \"scheduled task panic: %v\\n\", r) } }()")
			for name := range vars {
				if !c.globalVars[name] {
					c.lnf("var %s Value = null", safeIdent(name))
				}
			}
			c.emitBlock(ev.Body, false)
			c.indent--
			c.ln("}()")
			c.indent--
			c.ln("}")
			c.indent--
			c.ln("}()")
		}
	}

	c.lnf("addr := \":%d\"", c.port)
	c.ln(`fmt.Printf("httpdsl native server on %s\n", addr)`)
	if c.corsOrigins != "" {
		c.ln("var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {")
		c.indent++
		c.lnf("w.Header().Set(\"Access-Control-Allow-Origin\", %q)", c.corsOrigins)
		methods := c.corsMethods
		if methods == "" { methods = "GET, POST, PUT, PATCH, DELETE, OPTIONS" }
		c.lnf("w.Header().Set(\"Access-Control-Allow-Methods\", %q)", methods)
		headers := c.corsHeaders
		if headers == "" { headers = "Content-Type, Authorization" }
		c.lnf("w.Header().Set(\"Access-Control-Allow-Headers\", %q)", headers)
		c.ln("if r.Method == \"OPTIONS\" { w.WriteHeader(204); return }")
		c.ln("rt.ServeHTTP(w, r)")
		c.indent--
		c.ln("})")
	} else {
		c.ln("var handler http.Handler = rt")
	}
	if c.gzipEnabled {
		c.ln("handler = gzipMiddleware(handler)")
	}

	// Graceful shutdown handler
	if len(c.shutdownBlocks) > 0 || c.usedBuiltins["store"] || c.sessionEnabled {
		c.ln("// Graceful shutdown")
		c.ln("go func() {")
		c.indent++
		c.ln("_sigCh := make(chan os.Signal, 1)")
		c.ln("signal.Notify(_sigCh, os.Interrupt, syscall.SIGTERM)")
		c.ln("<-_sigCh")
		c.ln(`fmt.Println("\nShutting down...")`)
		// Emit user shutdown blocks
		if len(c.shutdownBlocks) > 0 {
			for _, block := range c.shutdownBlocks {
				vars := c.collectVars(block)
				for name := range vars {
					if !c.globalVars[name] {
						c.lnf("var %s Value = null", safeIdent(name))
					}
				}
				c.emitBlock(block, false)
			}
		}
		// Flush store if synced
		if c.sessionEnabled {
			c.ln("sessionFlush()")
		}
		if c.usedBuiltins["store"] {
			c.ln("if _storeFlush != nil { _storeFlush() }")
		}
		c.ln("os.Exit(0)")
		c.indent--
		c.ln("}()")
		c.ln("")
	}

	c.ln("if err := http.ListenAndServe(addr, handler); err != nil {")

	c.indent++
	c.ln(`fmt.Printf("Server error: %s\n", err)`)
	c.indent--
	c.ln("}")

	c.indent--
	c.ln("}")
}


// collectVars finds all variable names assigned in a block
func (c *NativeCompiler) collectVars(block *BlockStatement) map[string]bool {
	vars := make(map[string]bool)
	c.collectVarsFromBlock(block, vars)
	return vars
}

// collectRefs finds all identifier names referenced (read) in a block
func (c *NativeCompiler) collectRefs(block *BlockStatement) map[string]bool {
	refs := make(map[string]bool)
	c.collectRefsFromBlock(block, refs)
	return refs
}

func (c *NativeCompiler) collectRefsFromBlock(block *BlockStatement, refs map[string]bool) {
	if block == nil { return }
	for _, stmt := range block.Statements {
		c.collectRefsFromStmt(stmt, refs)
	}
}

func (c *NativeCompiler) collectRefsFromStmt(stmt Statement, refs map[string]bool) {
	if stmt == nil { return }
	switch s := stmt.(type) {
	case *AssignStatement:
		for _, v := range s.Values { c.collectRefsFromExpr(v, refs) }
	case *CompoundAssignStatement:
		refs[s.Name] = true
		c.collectRefsFromExpr(s.Value, refs)
	case *ExpressionStatement:
		c.collectRefsFromExpr(s.Expression, refs)
	case *ReturnStatement:
		for _, v := range s.Values { c.collectRefsFromExpr(v, refs) }
	case *IfStatement:
		c.collectRefsFromExpr(s.Condition, refs)
		c.collectRefsFromBlock(s.Consequence, refs)
		if alt, ok := s.Alternative.(*BlockStatement); ok { c.collectRefsFromBlock(alt, refs) }
		if alt, ok := s.Alternative.(*IfStatement); ok { c.collectRefsFromStmt(alt, refs) }
	case *WhileStatement:
		c.collectRefsFromExpr(s.Condition, refs)
		c.collectRefsFromBlock(s.Body, refs)
	case *EachStatement:
		c.collectRefsFromExpr(s.Iterable, refs)
		c.collectRefsFromBlock(s.Body, refs)
	case *SwitchStatement:
		c.collectRefsFromExpr(s.Subject, refs)
		for _, cs := range s.Cases { c.collectRefsFromBlock(cs.Body, refs) }
		if s.Default != nil { c.collectRefsFromBlock(s.Default, refs) }
	case *BlockStatement:
		c.collectRefsFromBlock(s, refs)
	case *TryCatchStatement:
		c.collectRefsFromBlock(s.Try, refs)
		c.collectRefsFromBlock(s.Catch, refs)
	case *ThrowStatement:
		c.collectRefsFromExpr(s.Value, refs)
	case *IndexAssignStatement:
		c.collectRefsFromExpr(s.Left, refs)
		c.collectRefsFromExpr(s.Index, refs)
		c.collectRefsFromExpr(s.Value, refs)
	}
}

func (c *NativeCompiler) collectRefsFromExpr(expr Expression, refs map[string]bool) {
	if expr == nil { return }
	switch e := expr.(type) {
	case *Identifier:
		refs[e.Value] = true
	case *InfixExpression:
		c.collectRefsFromExpr(e.Left, refs)
		c.collectRefsFromExpr(e.Right, refs)
	case *PrefixExpression:
		c.collectRefsFromExpr(e.Right, refs)
	case *CallExpression:
		c.collectRefsFromExpr(e.Function, refs)
		for _, a := range e.Arguments { c.collectRefsFromExpr(a, refs) }
	case *DotExpression:
		c.collectRefsFromExpr(e.Left, refs)
	case *IndexExpression:
		c.collectRefsFromExpr(e.Left, refs)
		c.collectRefsFromExpr(e.Index, refs)
	case *ArrayLiteral:
		for _, el := range e.Elements { c.collectRefsFromExpr(el, refs) }
	case *HashLiteral:
		for _, p := range e.Pairs {
			c.collectRefsFromExpr(p.Key, refs)
			c.collectRefsFromExpr(p.Value, refs)
		}
	case *TernaryExpression:
		c.collectRefsFromExpr(e.Condition, refs)
		c.collectRefsFromExpr(e.Consequence, refs)
		c.collectRefsFromExpr(e.Alternative, refs)
	}
}

func (c *NativeCompiler) collectVarsFromBlock(block *BlockStatement, vars map[string]bool) {
	for _, stmt := range block.Statements {
		c.collectVarsFromStmt(stmt, vars)
	}
}

func (c *NativeCompiler) collectVarsFromStmt(stmt Statement, vars map[string]bool) {
	switch s := stmt.(type) {
	case *AssignStatement:
		for _, n := range s.Names {
			vars[n] = true
		}
	case *CompoundAssignStatement:
		vars[s.Name] = true
	case *EachStatement:
		vars[s.Value] = true
		if s.Index != "" {
			vars[s.Index] = true
		}
		c.collectVarsFromBlock(s.Body, vars)
	case *IfStatement:
		c.collectVarsFromBlock(s.Consequence, vars)
		if s.Alternative != nil {
			switch alt := s.Alternative.(type) {
			case *BlockStatement:
				c.collectVarsFromBlock(alt, vars)
			case *IfStatement:
				c.collectVarsFromStmt(alt, vars)
			}
		}
	case *SwitchStatement:
		for _, cs := range s.Cases { c.collectVarsFromBlock(cs.Body, vars) }
		if s.Default != nil { c.collectVarsFromBlock(s.Default, vars) }
	case *WhileStatement:
		c.collectVarsFromBlock(s.Body, vars)
	case *BlockStatement:
		c.collectVarsFromBlock(s, vars)
	case *FnStatement:
		vars[s.Name] = true
		c.collectVarsFromBlock(s.Body, vars)
	case *TryCatchStatement:
		// Hoist both try and catch block vars so Go closures capture them.
		c.collectVarsFromBlock(s.Try, vars)
		c.collectVarsFromBlock(s.Catch, vars)
	case *ObjectDestructureStatement:
		for _, key := range s.Keys {
			vars[key] = true
		}
	case *ArrayDestructureStatement:
		for _, name := range s.Names {
			vars[name] = true
		}
	}
}
func (c *NativeCompiler) emitSSERoute(route *RouteStatement) {
	c.lnf(`rt.add("GET", %q, func(_w http.ResponseWriter, _r *http.Request) {`, route.Path)
	c.indent++

	// SSE headers
	c.ln(`_flusher, _ok := _w.(http.Flusher)`)
	c.ln(`if !_ok { http.Error(_w, "SSE not supported", http.StatusInternalServerError); return }`)
	c.ln(`_w.Header().Set("Content-Type", "text/event-stream")`)
	c.ln(`_w.Header().Set("Cache-Control", "no-cache")`)
	c.ln(`_w.Header().Set("Connection", "keep-alive")`)
	if c.corsOrigins != "" {
		c.lnf(`_w.Header().Set("Access-Control-Allow-Origin", %q)`, c.corsOrigins)
	}

	// Create SSE client
	c.ln(`_sseClient := &sseClient{`)
	c.indent++
	c.ln(`ch:       make(chan sseEvent, 64),`)
	c.ln(`channels: map[string]bool{"global": true},`)
	c.indent--
	c.ln(`}`)
	c.ln(`_sseHub.register(_sseClient)`)
	c.ln(`defer _sseHub.unregister(_sseClient)`)
	c.ln(``)

	// Parse path params, query, headers for request object
	c.ln(`_pathParams := getParams(_r)`)
	c.ln(`_queryMap := make(map[string]Value)`)
	c.ln(`for _k, _v := range _r.URL.Query() {`)
	c.indent++
	c.ln(`if len(_v) == 1 { _queryMap[_k] = Value(_v[0]) } else {`)
	c.indent++
	c.ln(`_arr := make([]Value, len(_v))`)
	c.ln(`for _i, _s := range _v { _arr[_i] = Value(_s) }`)
	c.ln(`_queryMap[_k] = Value(_arr)`)
	c.indent--
	c.ln(`}`)
	c.indent--
	c.ln(`}`)
	c.ln(`_reqHeaders := make(map[string]Value)`)
	c.ln(`for _k, _v := range _r.Header {`)
	c.indent++
	c.ln(`if len(_v) > 0 { _reqHeaders[strings.ToLower(_k)] = Value(_v[0]) }`)
	c.indent--
	c.ln(`}`)
	c.ln(`_reqCookies := make(map[string]Value)`)
	c.ln(`for _, _c := range _r.Cookies() { _reqCookies[_c.Name] = Value(_c.Value) }`)

	// Bearer/basic auth
	c.ln(`var _bearer Value = Value("")`)
	c.ln(`var _basic Value = null`)
	c.ln(`if _authH := _r.Header.Get("Authorization"); _authH != "" {`)
	c.indent++
	c.ln(`if strings.HasPrefix(_authH, "Bearer ") { _bearer = Value(_authH[7:]) }`)
	c.ln(`if strings.HasPrefix(_authH, "Basic ") {`)
	c.indent++
	c.ln(`if _decoded, _err := base64.StdEncoding.DecodeString(_authH[6:]); _err == nil {`)
	c.indent++
	c.ln(`if _idx := strings.IndexByte(string(_decoded), ':'); _idx >= 0 {`)
	c.indent++
	c.ln(`_basic = Value(map[string]Value{"username": Value(string(_decoded[:_idx])), "password": Value(string(_decoded[_idx+1:]))})`)
	c.indent--
	c.ln(`}`)
	c.indent--
	c.ln(`}`)
	c.indent--
	c.ln(`}`)
	c.indent--
	c.ln(`}`)

	// Session loading (for SSE auth)
	if c.sessionEnabled {
		c.ln(`_sessID, _sessData, _ := sessionLoad(_r)`)
		c.ln(`_sessDestroyed := false`)
		c.ln(`_ = _sessID`)
		c.ln(`_ = _sessDestroyed`)
	}

	// Build request object
	c.ln(`request := Value(map[string]Value{`)
	c.indent++
	c.ln(`"method":  Value("SSE"),`)
	c.ln(`"path":    Value(_r.URL.Path),`)
	c.ln(`"params":  Value(_pathParams),`)
	c.ln(`"query":   Value(_queryMap),`)
	c.ln(`"headers": Value(_reqHeaders),`)
	c.ln(`"cookies": Value(_reqCookies),`)
	c.ln(`"bearer":  _bearer,`)
	c.ln(`"basic":   _basic,`)
	if c.sessionEnabled {
		c.ln(`"session": Value(_sessData),`)
	}
	c.indent--
	c.ln(`})`)
	c.ln(`_ = request`)
	c.ln(``)

	// Declare user variables
	vars := c.collectVars(route.Body)
	for name := range vars {
		if name != "request" && !c.globalVars[name] {
			c.lnf("var %s Value = null", safeIdent(name))
		}
	}

	// Run route body (initial events, auth, joins)
	c.emitBlock(route.Body, true)
	c.ln(``)

	// Event loop: block on client channel or disconnect
	c.ln(`for {`)
	c.indent++
	c.ln(`select {`)
	c.ln(`case _evt, _ok := <-_sseClient.ch:`)
	c.indent++
	c.ln(`if !_ok { return }`)
	c.ln(`if _evt.event != "" {`)
	c.indent++
	c.ln(`fmt.Fprintf(_w, "event: %s\n", _evt.event)`)
	c.indent--
	c.ln(`}`)
	c.ln(`fmt.Fprintf(_w, "data: %s\n\n", _evt.data)`)
	c.ln(`_flusher.Flush()`)
	c.indent--
	c.ln(`case <-_r.Context().Done():`)
	c.indent++
	c.ln(`return`)
	c.indent--
	c.ln(`}`)
	c.indent--
	c.ln(`}`)

	c.indent--
	c.ln(`})`)
	c.ln(``)
}

func (c *NativeCompiler) emitRoute(route *RouteStatement) {
	c.lnf("rt.add(%q, %q, func(_w http.ResponseWriter, _r *http.Request) {", route.Method, route.Path)
	c.indent++
	c.inRouteHandler = true
	defer func() { c.inRouteHandler = false }()

	// Request timeout
	timeout := route.Timeout
	if timeout == 0 {
		timeout = c.defaultTimeout
	}
	if timeout > 0 {
		c.lnf("_ctx, _cancel := context.WithTimeout(_r.Context(), %d*time.Second)", timeout)
		c.ln("defer _cancel()")
		c.ln("_r = _r.WithContext(_ctx)")
	}

	// Handle redirects
	c.ln("defer func() {")
	c.indent++
	c.ln("if _rv := recover(); _rv != nil {")
	c.indent++
	c.ln("if _rs, ok := _rv.(*redirectSignal); ok {")
	c.indent++
	c.ln("http.Redirect(_w, _r, _rs.url, _rs.status)")
	c.indent--
	c.ln("} else { panic(_rv) }")
	c.indent--
	c.ln("}")
	c.indent--
	c.ln("}()")

	// Read request basics
	c.ln("_pathParams := getParams(_r)")
	c.ln("_bodyBytes, _ := io.ReadAll(_r.Body)")
	c.ln("defer _r.Body.Close()")

	// Parse query params
	c.ln("_queryMap := make(map[string]Value)")
	c.ln("for _k, _v := range _r.URL.Query() {")
	c.indent++
	c.ln("if len(_v) == 1 { _queryMap[_k] = Value(_v[0]) } else {")
	c.indent++
	c.ln("_arr := make([]Value, len(_v))")
	c.ln("for _i, _s := range _v { _arr[_i] = Value(_s) }")
	c.ln("_queryMap[_k] = Value(_arr)")
	c.indent--
	c.ln("}")
	c.indent--
	c.ln("}")

	// Parse request headers
	c.ln("_reqHeaders := make(map[string]Value)")
	c.ln("for _k, _v := range _r.Header {")
	c.indent++
	c.ln("_hk := strings.ToLower(_k)")
	c.ln("if len(_v) == 1 { _reqHeaders[_hk] = Value(_v[0]) } else {")
	c.indent++
	c.ln("_arr := make([]Value, len(_v))")
	c.ln("for _i, _s := range _v { _arr[_i] = Value(_s) }")
	c.ln("_reqHeaders[_hk] = Value(_arr)")
	c.indent--
	c.ln("}")
	c.indent--
	c.ln("}")

	// Parse request cookies
	c.ln("_reqCookies := make(map[string]Value)")
	c.ln("for _, _c := range _r.Cookies() {")
	c.indent++
	c.ln("_reqCookies[_c.Name] = Value(_c.Value)")
	c.indent--
	c.ln("}")

	// Client IP
	c.ln(`_clientIP := _r.RemoteAddr`)
	c.ln(`if _xff := _r.Header.Get("X-Forwarded-For"); _xff != "" { _clientIP = strings.Split(_xff, ",")[0] }`)

	// Content type and body parsing
	c.ln("_contentType := _r.Header.Get(\"Content-Type\")")
	c.ln("var _reqData Value = null")
	c.ln("_bodyStr := string(_bodyBytes)")
	c.ln("var _reqFiles Value = Value([]Value{})")

	typeCheck := route.TypeCheck
	hasTypeCheck := typeCheck != ""

	if !hasTypeCheck {
		// Auto-detect content type
		c.ln("if strings.Contains(_contentType, \"application/json\") {")
		c.indent++
		c.ln("_reqData = parseJSONBody(_bodyStr)")
		c.indent--
		c.ln("} else if strings.Contains(_contentType, \"application/x-www-form-urlencoded\") {")
		c.indent++
		c.ln("_reqData = parseFormBody(_bodyStr)")
		c.indent--
		c.ln("} else if strings.Contains(_contentType, \"multipart/form-data\") {")
		c.indent++
		c.ln("_r.Body = io.NopCloser(bytes.NewReader(_bodyBytes))")
		c.ln("_reqData, _reqFiles = parseMultipartBody(_r)")
		c.indent--
		c.ln("} else if len(_bodyBytes) > 0 {")
		c.indent++
		c.ln("_reqData = parseJSONBody(_bodyStr)")
		c.ln("if _reqData == null { _reqData = Value(_bodyStr) }")
		c.indent--
		c.ln("}")
	} else {
		c.lnf("// Enforced type: %s", typeCheck)
		c.ln("var _typeError Value = null")
		switch typeCheck {
		case "json":
			c.ln("if strings.Contains(_contentType, \"application/json\") || _contentType == \"\" {")
			c.indent++
			c.ln("if len(_bodyBytes) > 0 {")
			c.indent++
			c.ln("_reqData = parseJSONBody(_bodyStr)")
			c.ln("if _reqData == null { _typeError = Value(\"invalid JSON body\") }")
			c.indent--
			c.ln("}")
			c.indent--
			c.ln("} else {")
			c.indent++
			c.ln("_typeError = Value(\"expected Content-Type application/json, got \" + _contentType)")
			c.indent--
			c.ln("}")
		case "text":
			c.ln("_reqData = Value(_bodyStr)")
		case "form":
			c.ln("if strings.Contains(_contentType, \"application/x-www-form-urlencoded\") {")
			c.indent++
			c.ln("_reqData = parseFormBody(_bodyStr)")
			c.indent--
			c.ln("} else if strings.Contains(_contentType, \"multipart/form-data\") {")
			c.indent++
			c.ln("_r.Body = io.NopCloser(bytes.NewReader(_bodyBytes))")
			c.ln("_reqData, _reqFiles = parseMultipartBody(_r)")
			c.indent--
			c.ln("} else {")
			c.indent++
			c.ln("_typeError = Value(\"expected form data, got \" + _contentType)")
			c.indent--
			c.ln("}")
		}
	}

	// Session loading
	if c.sessionEnabled {
		c.ln("_sessID, _sessData, _ := sessionLoad(_r)")
		c.ln("_sessDestroyed := false")
	}

	// CSRF validation
	if c.csrfEnabled && !route.CSRFDisabled {
		c.ln(`// CSRF protection`)
		c.ln(`if !csrfValidate(_r, _sessData, _bodyStr) {`)
		c.indent++
		c.ln(`_w.Header().Set("Content-Type", "application/json")`)
		c.ln(`_w.WriteHeader(403)`)
		c.ln(`_w.Write([]byte("{\"error\":\"CSRF token mismatch\"}"))`)
		c.ln(`return`)
		c.indent--
		c.ln(`}`)
	}

	// Parse Authorization header
	c.ln(`var _bearer Value = Value("")`)
	c.ln(`var _basic Value = null`)
	c.ln(`if _authH := _r.Header.Get("Authorization"); _authH != "" {`)
	c.indent++
	c.ln(`if strings.HasPrefix(_authH, "Bearer ") { _bearer = Value(_authH[7:]) }`)
	c.ln(`if strings.HasPrefix(_authH, "Basic ") {`)
	c.indent++
	c.ln(`if _decoded, _err := base64.StdEncoding.DecodeString(_authH[6:]); _err == nil {`)
	c.indent++
	c.ln(`if _idx := strings.IndexByte(string(_decoded), ':'); _idx >= 0 {`)
	c.indent++
	c.ln(`_basic = Value(map[string]Value{"username": Value(string(_decoded[:_idx])), "password": Value(string(_decoded[_idx+1:]))})`)
	c.indent--
	c.ln(`}`)
	c.indent--
	c.ln(`}`)
	c.indent--
	c.ln(`}`)
	c.indent--
	c.ln(`}`)

	// Build request object
	c.ln("request := Value(map[string]Value{")
	c.indent++
	c.ln("\"method\":  Value(_r.Method),")
	c.ln("\"path\":    Value(_r.URL.Path),")
	c.ln("\"body\":    Value(_bodyStr),")
	c.ln("\"data\":    _reqData,")
	c.ln("\"params\":  Value(_pathParams),")
	c.ln("\"query\":   Value(_queryMap),")
	c.ln("\"headers\": Value(_reqHeaders),")
	c.ln("\"cookies\": Value(_reqCookies),")
	c.ln("\"ip\":      Value(_clientIP),")
	c.ln("\"files\":   _reqFiles,")
	c.ln("\"bearer\":  _bearer,")
	c.ln("\"basic\":   _basic,")
	if c.sessionEnabled {
		c.ln("\"session\": Value(_sessData),")
	}
	c.indent--
	c.ln("})")


	// Build response object
	c.ln("response := Value(map[string]Value{")
	c.indent++
	c.ln("\"status\":  Value(int64(200)),")
	c.ln("\"type\":    Value(\"json\"),")
	c.ln("\"body\":    Value(map[string]Value{}),")
	c.ln("\"headers\": Value(map[string]Value{}),")
	c.ln("\"cookies\": Value(map[string]Value{}),")
	c.indent--
	c.ln("})")

	// Handle type check failure
	if hasTypeCheck {
		if route.ElseBlock != nil {
			c.ln("if _typeError != null {")
			c.indent++
			c.ln("error := _typeError")
			c.ln("_ = error")
			c.ln("_ = request")
			c.ln("_ = response")

			elseVars := c.collectVars(route.ElseBlock)
			for name := range elseVars {
				if name != "request" && name != "response" && name != "error" && !c.globalVars[name] {
					c.lnf("var %s Value = null", safeIdent(name))
				}
			}
			c.emitBlock(route.ElseBlock, true)
			c.ln("writeResponse(_w, response)")
			c.ln("return")
			c.indent--
			c.ln("}")
		} else {
			c.ln("if _typeError != null {")
			c.indent++
			c.ln(`_w.Header().Set("Content-Type", "application/json")`)
			c.ln("_w.WriteHeader(400)")
			c.ln(`json.NewEncoder(_w).Encode(map[string]interface{}{"error": valueToGo(_typeError)})`)
			c.ln("return")
			c.indent--
			c.ln("}")
		}
	}

	// Collect all before/after blocks for this route
	var beforeBlocks []*BlockStatement
	beforeBlocks = append(beforeBlocks, c.globalBefore...)
	beforeBlocks = append(beforeBlocks, c.routeBeforeMap[route]...)
	var afterBlocks []*BlockStatement
	afterBlocks = append(afterBlocks, c.globalAfter...)
	afterBlocks = append(afterBlocks, c.routeAfterMap[route]...)

	// Declare all variables (before + body + after)
	vars := c.collectVars(route.Body)
	for _, bb := range beforeBlocks {
		for k, v := range c.collectVars(bb) { vars[k] = v }
	}
	for _, ab := range afterBlocks {
		for k, v := range c.collectVars(ab) { vars[k] = v }
	}
	for name := range vars {
		if name != "request" && name != "response" && !c.globalVars[name] {
			c.lnf("var %s Value = null", safeIdent(name))
		}
	}
	c.ln("_ = request")
	c.ln("_ = response")
	c.ln("")

	// Wrap before + body in func so return in before skips body
	hasBefore := len(beforeBlocks) > 0
	if hasBefore {
		c.ln("func() {")
		c.indent++
		for _, bb := range beforeBlocks {
			c.emitBlock(bb, true)
		}
	}

	// Wrap route body in recover for throw — catch goes to else block
	if route.ElseBlock != nil {
		c.ln("func() {")
		c.indent++
		c.ln("defer func() {")
		c.indent++
		c.ln("if _r := recover(); _r != nil {")
		c.indent++
		c.ln("if _tv, ok := _r.(*throwValue); ok {")
		c.indent++
		c.ln("error := _tv.value")
		c.ln("_ = error")
		elseVars := c.collectVars(route.ElseBlock)
		for name := range elseVars {
			if name != "request" && name != "response" && name != "error" && !c.globalVars[name] {
				c.lnf("var %s Value = null", safeIdent(name))
			}
		}
		c.emitBlock(route.ElseBlock, true)
		c.indent--
		c.ln("} else { panic(_r) }")
		c.indent--
		c.ln("}")
		c.indent--
		c.ln("}()")
	}

	// Emit route body
	c.emitBlock(route.Body, true)

	if route.ElseBlock != nil {
		c.indent--
		c.ln("}()")
	}

	if hasBefore {
		c.indent--
		c.ln("}()")
	}

	// Session save
	if c.sessionEnabled {
		c.ln("// Save session")
		c.ln("if _rm, ok := request.(map[string]Value); ok {")
		c.indent++
		c.ln("if _sd, ok := _rm[\"session\"].(map[string]Value); ok {")
		c.indent++
		c.ln("sessionSave(_w, _sessID, _sd, _sessDestroyed)")
		c.indent--
		c.ln("} else if _rm[\"session\"] == null || _rm[\"session\"] == nil {")
		c.indent++
		c.ln("sessionSave(_w, _sessID, nil, true)")
		c.indent--
		c.ln("}")
		c.indent--
		c.ln("}")
	}

	// Auto-return response
	timeoutActive := route.Timeout > 0 || c.defaultTimeout > 0
	if timeoutActive {
		c.ln("if _r.Context().Err() == context.DeadlineExceeded {")
		c.indent++
		c.ln(`_w.Header().Set("Content-Type", "application/json")`)
		c.ln("_w.WriteHeader(504)")
		c.ln(`_w.Write([]byte("{\"error\":\"request timeout\"}"))`)
		c.indent--
		c.ln("} else {")
		c.indent++
		c.ln("writeResponse(_w, response)")
		c.indent--
		c.ln("}")
	} else {
		c.ln("writeResponse(_w, response)")
	}

	// After blocks — fire and forget goroutine
	if len(afterBlocks) > 0 {
		c.ln("go func(_afterReq, _afterResp Value) {")
		c.indent++
		c.ln("defer func() { recover() }() // never crash from after block")
		// Shadow request/response with copies
		c.ln("request := _afterReq")
		c.ln("response := _afterResp")
		c.ln("_ = request")
		c.ln("_ = response")
		for _, ab := range afterBlocks {
			c.emitBlock(ab, true)
		}
		c.indent--
		c.ln("}(request, response)")
	}

	c.indent--
	c.ln("})")
	c.ln("")
}

func (c *NativeCompiler) emitErrorHandler(eh *ErrorStatement) {
	c.lnf("rt.errorHandlers[%d] = func(_w http.ResponseWriter, _r *http.Request) {", eh.StatusCode)
	c.indent++
	c.inRouteHandler = true
	defer func() { c.inRouteHandler = false }()

	// Build minimal request object (no body parsing needed for error pages)
	c.ln("_pathParams := make(map[string]Value)")
	c.ln("_queryMap := make(map[string]Value)")
	c.ln("for _k, _v := range _r.URL.Query() {")
	c.indent++
	c.ln("if len(_v) == 1 { _queryMap[_k] = Value(_v[0]) } else {")
	c.indent++
	c.ln("_arr := make([]Value, len(_v))")
	c.ln("for _i, _s := range _v { _arr[_i] = Value(_s) }")
	c.ln("_queryMap[_k] = Value(_arr)")
	c.indent--
	c.ln("}")
	c.indent--
	c.ln("}")

	c.ln("_reqHeaders := make(map[string]Value)")
	c.ln("for _k, _v := range _r.Header { _reqHeaders[strings.ToLower(_k)] = Value(_v[0]) }")

	c.ln("request := Value(map[string]Value{")
	c.indent++
	c.ln(`"method":  Value(_r.Method),`)
	c.ln(`"path":    Value(_r.URL.Path),`)
	c.ln(`"params":  Value(_pathParams),`)
	c.ln(`"query":   Value(_queryMap),`)
	c.ln(`"headers": Value(_reqHeaders),`)
	c.indent--
	c.ln("})")

	// Build response object with the error status code as default
	c.ln("response := Value(map[string]Value{")
	c.indent++
	c.lnf(`"status":  Value(int64(%d)),`, eh.StatusCode)
	c.ln(`"type":    Value("json"),`)
	c.ln(`"body":    Value(map[string]Value{}),`)
	c.ln(`"headers": Value(map[string]Value{}),`)
	c.ln(`"cookies": Value(map[string]Value{}),`)
	c.indent--
	c.ln("})")

	// Declare variables
	vars := c.collectVars(eh.Body)
	for name := range vars {
		if name != "request" && name != "response" && !c.globalVars[name] {
			c.lnf("var %s Value = null", safeIdent(name))
		}
	}

	c.ln("_ = request")

	// Emit body
	c.emitBlock(eh.Body, true)

	// Write response
	c.ln("writeResponse(_w, response)")

	c.indent--
	c.ln("}")
	c.ln("")
}
