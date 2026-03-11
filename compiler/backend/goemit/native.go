package goemit

import (
	"fmt"
	runtimeassets "httpdsl/compiler/runtime"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type NativeCompiler struct {
	b                  strings.Builder
	indent             int
	port               int
	routes             []*RouteStatement
	groups             []*GroupStatement
	functions          []*FnStatement
	usedBuiltins       map[string]bool
	usedImports        map[string]bool
	needsBcrypt        bool
	needsArgon2        bool
	dbDrivers          map[string]bool // "sqlite", "postgres", "mysql", "mongo"
	tmpCounter         int
	typeEnv            *TypeEnv            // current function's type info
	fnTypes            map[string]*TypeEnv // per-function type info
	corsOrigins        string              // CORS: allowed origins ("*" or comma-separated)
	corsMethods        string              // CORS: allowed methods
	corsHeaders        string              // CORS: allowed headers
	errorHandlers      []*ErrorStatement
	inRouteHandler     bool // true when emitting code inside a route/error handler
	inSSERoute         bool // true when emitting code inside an SSE route handler
	throttleRPS        int  // per-IP requests/second limit; 0 = disabled
	defaultTimeout     int  // server-level timeout in seconds; 0 = no timeout
	gzipEnabled        bool // gzip compression
	globalBefore       []*BlockStatement
	globalAfter        []*BlockStatement
	routeBeforeMap     map[*RouteStatement][]*BlockStatement // group before blocks per route
	routeAfterMap      map[*RouteStatement][]*BlockStatement // group after blocks per route
	staticMounts       []staticMount                         // static file serving
	everyBlocks        []*EveryStatement
	initBlocks         []*BlockStatement
	shutdownBlocks     []*BlockStatement
	globalVars         map[string]bool // variables declared in init blocks
	sessionEnabled     bool
	sessionCookie      string     // cookie name, default "sid"
	sessionExpires     int        // seconds, default 86400 (24h)
	sessionSecret      string     // HMAC signing secret (literal)
	sessionSecretExpr  Expression // secret expression (e.g., env("..."))
	csrfEnabled        bool
	csrfSafeOrigins    []string
	templatesDir       string            // path to templates directory
	templateFiles      map[string]string // name -> content (embedded at compile time)
	hasSSE             bool              // whether any SSE routes exist
	hasCron            bool              // whether any cron expressions are used
	hasExec            bool              // whether exec() builtin is used
	helpText           string            // help text from help block
	sslCert            string            // path to SSL certificate file (literal)
	sslKey             string            // path to SSL private key file (literal)
	sslCertExpr        Expression        // SSL cert expression (e.g., env("SSL_CERT"))
	sslKeyExpr         Expression        // SSL key expression (e.g., env("SSL_KEY"))
	autocertDomain     string            // autocert domain (literal)
	autocertDomainExpr Expression        // autocert domain expression
	autocertDir        string            // autocert cache directory (literal)
	autocertDirExpr    Expression        // autocert cache dir expression
	httpsRedirect      string            // true, false, or empty (auto: true when TLS active)
	wwwRedirect        string            // true or false (default: false)
	emitErr            error
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

func newNativeCompiler() *NativeCompiler {
	return &NativeCompiler{
		port:           8080,
		usedBuiltins:   make(map[string]bool),
		usedImports:    make(map[string]bool),
		dbDrivers:      make(map[string]bool),
		routeBeforeMap: make(map[*RouteStatement][]*BlockStatement),
		routeAfterMap:  make(map[*RouteStatement][]*BlockStatement),
		globalVars:     make(map[string]bool),
	}
}

func GenerateNativeCode(program *Program) (string, error) {
	c := newNativeCompiler()
	if err := c.prepareProgram(program); err != nil {
		return "", err
	}
	return c.emitProgram()
}

func GenerateGoFromIR(program *Program, dbDrivers map[string]bool) (string, error) {
	if program == nil {
		return "", fmt.Errorf("nil program")
	}
	c := newNativeCompiler()
	if len(dbDrivers) > 0 {
		c.dbDrivers = make(map[string]bool, len(dbDrivers))
		for k, v := range dbDrivers {
			c.dbDrivers[k] = v
		}
	}
	if err := c.prepareProgram(program); err != nil {
		return "", err
	}
	return c.emitProgram()
}

func (c *NativeCompiler) prepareProgram(program *Program) error {
	if program == nil {
		return fmt.Errorf("nil program")
	}
	if err := c.loadFromStatements(program.Statements); err != nil {
		return err
	}
	return c.finalizeProgram(program)
}

func (c *NativeCompiler) loadFromStatements(statements []Statement) error {
	for _, stmt := range statements {
		switch s := stmt.(type) {
		case *RouteStatement:
			c.routes = append(c.routes, s)
			c.scanBlock(s.Body)
			if s.Method == "SSE" {
				c.hasSSE = true
			}
		case *GroupStatement:
			for _, b := range s.Before {
				c.scanBlock(b)
			}
			for _, a := range s.After {
				c.scanBlock(a)
			}
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
			if s.CronExpr != "" {
				c.hasCron = true
			}
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
		case *HelpStatement:
			c.helpText = s.Text
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
						if sl, ok := p.Key.(*StringLiteral); ok {
							key = sl.Value
						}
						val := ""
						if sv, ok := p.Value.(*StringLiteral); ok {
							val = sv.Value
						}
						switch key {
						case "origins":
							c.corsOrigins = val
						case "methods":
							c.corsMethods = val
						case "headers":
							c.corsHeaders = val
						}
					}
				}
			}
			// SSL/TLS config
			if cert, ok := s.Settings["ssl_cert"]; ok {
				if sv, ok := cert.(*StringLiteral); ok {
					c.sslCert = sv.Value
				} else {
					c.sslCertExpr = cert
				}
			}
			if key, ok := s.Settings["ssl_key"]; ok {
				if sv, ok := key.(*StringLiteral); ok {
					c.sslKey = sv.Value
				} else {
					c.sslKeyExpr = key
				}
			}
			// Autocert config
			if domain, ok := s.Settings["autocert"]; ok {
				if sv, ok := domain.(*StringLiteral); ok {
					c.autocertDomain = sv.Value
				} else {
					c.autocertDomainExpr = domain
				}
			}
			if dir, ok := s.Settings["autocert_dir"]; ok {
				if sv, ok := dir.(*StringLiteral); ok {
					c.autocertDir = sv.Value
				} else {
					c.autocertDirExpr = dir
				}
			}
			// Redirect settings
			if v, ok := s.Settings["https_redirect"]; ok {
				if bl, ok := v.(*BooleanLiteral); ok {
					if bl.Value {
						c.httpsRedirect = "true"
					} else {
						c.httpsRedirect = "false"
					}
				}
			}
			if v, ok := s.Settings["www_redirect"]; ok {
				if bl, ok := v.(*BooleanLiteral); ok {
					if bl.Value {
						c.wwwRedirect = "true"
					} else {
						c.wwwRedirect = "false"
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
						if sl, ok := p.Key.(*StringLiteral); ok {
							key = sl.Value
						}
						switch key {
						case "cookie":
							if sv, ok := p.Value.(*StringLiteral); ok {
								c.sessionCookie = sv.Value
							}
						case "expires":
							if iv, ok := p.Value.(*IntegerLiteral); ok {
								c.sessionExpires = int(iv.Value)
							}
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
		default:
			return fmt.Errorf("unexpected top-level statement — use init {} for startup code")
		}
	}
	return nil
}

func (c *NativeCompiler) finalizeProgram(program *Program) error {
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
	// Always available via -a flag or AUTOCERT_DOMAIN env
	c.usedImports["crypto/tls"] = true
	// Import tracking for builtins
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
	if len(c.dbDrivers) == 0 && program != nil {
		c.detectDBDrivers(program)
	}
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
			return fmt.Errorf("templates: %w", err)
		}
		if len(c.templateFiles) > 0 {
			c.usedImports["html/template"] = true
		}
	}
	return nil
}

func (c *NativeCompiler) emitProgram() (string, error) {
	c.emitHeader()
	c.emitGlobalVars()
	c.emitEnvRuntime()
	c.emitRuntime()
	c.emitBuiltinFuncs()
	c.emitCronRuntime()
	c.emitDBRuntime()
	c.emitSessionRuntime()
	c.emitTemplateRuntime()
	c.emitSSERuntime()
	c.emitUserFunctions()
	c.emitMain()
	if c.emitErr != nil {
		return "", c.emitErr
	}
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
		for _, v := range s.Values {
			c.scanExpr(v)
		}
	case *CompoundAssignStatement:
		c.scanExpr(s.Value)
	case *IndexAssignStatement:
		c.scanExpr(s.Left)
		c.scanExpr(s.Index)
		c.scanExpr(s.Value)
	case *ReturnStatement:
		for _, v := range s.Values {
			c.scanExpr(v)
		}
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
			case *BlockStatement:
				c.scanBlock(alt)
			case *IfStatement:
				c.scanStmt(alt)
			}
		}
	case *SwitchStatement:
		c.scanExpr(s.Subject)
		for _, cs := range s.Cases {
			for _, v := range cs.Values {
				c.scanExpr(v)
			}
			c.scanBlock(cs.Body)
		}
		if s.Default != nil {
			c.scanBlock(s.Default)
		}
	case *WhileStatement:
		c.scanExpr(s.Condition)
		c.scanBlock(s.Body)
	case *EachStatement:
		c.scanExpr(s.Iterable)
		c.scanBlock(s.Body)
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
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *Identifier:
		c.usedBuiltins[e.Value] = true
	case *CallExpression:
		c.scanExpr(e.Function)
		for _, a := range e.Arguments {
			c.scanExpr(a)
		}
		// Track exec() as top-level call (not db.exec())
		if id, ok := e.Function.(*Identifier); ok && id.Value == "exec" {
			c.hasExec = true
		}
	case *InfixExpression:
		c.scanExpr(e.Left)
		c.scanExpr(e.Right)
	case *TernaryExpression:
		c.scanExpr(e.Condition)
		c.scanExpr(e.Consequence)
		c.scanExpr(e.Alternative)
	case *PrefixExpression:
		c.scanExpr(e.Right)
	case *IndexExpression:
		c.scanExpr(e.Left)
		c.scanExpr(e.Index)
	case *DotExpression:
		c.scanExpr(e.Left)
		if id, ok := e.Left.(*Identifier); ok {
			c.usedBuiltins[id.Value] = true
		}
	case *ArrayLiteral:
		for _, el := range e.Elements {
			c.scanExpr(el)
		}
	case *HashLiteral:
		for _, p := range e.Pairs {
			c.scanExpr(p.Key)
			c.scanExpr(p.Value)
		}
	case *FunctionLiteral:
		c.scanBlock(e.Body)
	case *AsyncExpression:
		c.scanExpr(e.Expression)
	}
}

// --- emit helpers ---
func (c *NativeCompiler) ln(s string) {
	for i := 0; i < c.indent; i++ {
		c.b.WriteByte('\t')
	}
	c.b.WriteString(s)
	c.b.WriteByte('\n')
}

func (c *NativeCompiler) lnf(f string, a ...interface{}) {
	for i := 0; i < c.indent; i++ {
		c.b.WriteByte('\t')
	}
	fmt.Fprintf(&c.b, f, a...)
	c.b.WriteByte('\n')
}

func (c *NativeCompiler) raw(s string) { c.b.WriteString(s) }

func (c *NativeCompiler) emitRuntimeTemplate(name string, data any) {
	if c.emitErr != nil {
		return
	}
	out, err := runtimeassets.Render(name, data)
	if err != nil {
		c.emitErr = err
		return
	}
	c.raw(out)
}

type sessionRuntimeTemplateData struct {
	SessionSecretExpr string
	SessionExpires    int
	SessionCookie     string
}

type csrfRuntimeTemplateData struct {
	SafeOrigins []string
}

type storeSyncRuntimeTemplateData struct {
	EnableDB bool
}

// detectDBDrivers scans all db.open() calls for the driver string literal
func (c *NativeCompiler) detectDBDrivers(program *Program) {
	for _, stmt := range program.Statements {
		c.detectDBInStmt(stmt)
	}
}

func (c *NativeCompiler) detectDBInStmt(stmt Statement) {
	if stmt == nil {
		return
	}
	switch s := stmt.(type) {
	case *AssignStatement:
		for _, v := range s.Values {
			c.detectDBInExpr(v)
		}
	case *ExpressionStatement:
		c.detectDBInExpr(s.Expression)
	case *IfStatement:
		c.detectDBInBlock(s.Consequence)
		if alt, ok := s.Alternative.(*BlockStatement); ok {
			c.detectDBInBlock(alt)
		}
		if alt, ok := s.Alternative.(*IfStatement); ok {
			c.detectDBInStmt(alt)
		}
	case *SwitchStatement:
		for _, cs := range s.Cases {
			c.detectDBInBlock(cs.Body)
		}
		if s.Default != nil {
			c.detectDBInBlock(s.Default)
		}
	case *WhileStatement:
		c.detectDBInBlock(s.Body)
	case *EachStatement:
		c.detectDBInBlock(s.Body)
	case *BlockStatement:
		c.detectDBInBlock(s)
	case *RouteStatement:
		c.detectDBInBlock(s.Body)
		if s.ElseBlock != nil {
			c.detectDBInBlock(s.ElseBlock)
		}
	case *GroupStatement:
		for _, r := range s.Routes {
			c.detectDBInStmt(r)
		}
		for _, b := range s.Before {
			c.detectDBInBlock(b)
		}
		for _, a := range s.After {
			c.detectDBInBlock(a)
		}
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
		for _, v := range s.Values {
			c.detectDBInExpr(v)
		}
	case *ObjectDestructureStatement:
		c.detectDBInExpr(s.Value)
	case *ArrayDestructureStatement:
		c.detectDBInExpr(s.Value)
	}
}

func (c *NativeCompiler) detectDBInBlock(block *BlockStatement) {
	if block == nil {
		return
	}
	for _, stmt := range block.Statements {
		c.detectDBInStmt(stmt)
	}
}

func (c *NativeCompiler) detectDBInExpr(expr Expression) {
	if expr == nil {
		return
	}
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
		for _, a := range e.Arguments {
			c.detectDBInExpr(a)
		}
	case *InfixExpression:
		c.detectDBInExpr(e.Left)
		c.detectDBInExpr(e.Right)
	case *TernaryExpression:
		c.detectDBInExpr(e.Condition)
		c.detectDBInExpr(e.Consequence)
		c.detectDBInExpr(e.Alternative)
	case *PrefixExpression:
		c.detectDBInExpr(e.Right)
	case *DotExpression:
		c.detectDBInExpr(e.Left)
	case *IndexExpression:
		c.detectDBInExpr(e.Left)
		c.detectDBInExpr(e.Index)
	case *ArrayLiteral:
		for _, el := range e.Elements {
			c.detectDBInExpr(el)
		}
	case *HashLiteral:
		for _, p := range e.Pairs {
			c.detectDBInExpr(p.Key)
			c.detectDBInExpr(p.Value)
		}
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
		"crypto/sha256", "crypto/sha512", "crypto/subtle", "crypto/tls", "database/sql", "encoding/base64", "encoding/hex", "encoding/json",
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
	c.lnf("%q", "golang.org/x/crypto/acme/autocert")
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
	c.emitRuntimeTemplate("store_sync_runtime.gotmpl", storeSyncRuntimeTemplateData{
		EnableDB: len(c.dbDrivers) > 0,
	})
}

func (c *NativeCompiler) emitGlobalVars() {
	c.ln("// ===== Global Variables =====")
	c.ln("var _envMap = map[string]string{}")
	c.ln("var _argsMap = map[string]Value{}")
	// Sort for deterministic output
	names := make([]string, 0, len(c.globalVars))
	for name := range c.globalVars {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if name == "args" {
			continue
		} // handled by _argsMap
		c.lnf("var %s Value = null", safeIdent(name))
	}
	if c.usedBuiltins["server_stats"] {
		c.ln("var _startTime = time.Now()")
	}
	c.ln("")
}

func (c *NativeCompiler) emitEnvRuntime() {
	c.emitRuntimeTemplate("env_runtime.gotmpl", nil)
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
			reqWithParams := r.WithContext(ctx)
			func() {
				defer func() {
					if rec := recover(); rec != nil {
						if h, ok := rt.errorHandlers[500]; ok {
							h(w, reqWithParams)
							return
						}
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(500)
						w.Write([]byte("{\"error\":\"internal server error\"}"))
					}
				}()
				route.handler(w, reqWithParams)
			}()
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
			_w.Header().Set("Content-Type", "application/json; charset=utf-8")
		}
		_w.WriteHeader(status)
		if body != nil { json.NewEncoder(_w).Encode(valueToGo(body)) }
	case "text":
		if _w.Header().Get("Content-Type") == "" {
			_w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		}
		_w.WriteHeader(status)
		if body != nil { fmt.Fprint(_w, valueToString(body)) }
	case "html":
		if _w.Header().Get("Content-Type") == "" {
			_w.Header().Set("Content-Type", "text/html; charset=utf-8")
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
		c.emitRuntimeTemplate("store_core_runtime.gotmpl", nil)
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

func (w *gzipResponseWriter) Flush() {
	w.gz.Flush()
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		// Skip gzip for SSE — event streams must not be compressed
		if r.Header.Get("Accept") == "text/event-stream" {
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
	key := valueToString(args[0])
	// Check .env file map first, then OS environment
	v, ok := _envMap[key]
	if !ok || v == "" {
		v = os.Getenv(key)
	}
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


func builtin_merge(args ...Value) Value {
	r := make(map[string]Value)
	for _, a := range args {
		if m, ok := a.(map[string]Value); ok {
			for k, v := range m { r[k] = v }
		}
	}
	return r
}

func deepPatch(base, patch map[string]Value, allowNew bool) map[string]Value {
	r := make(map[string]Value, len(base))
	for k, v := range base {
		r[k] = v
	}
	for k, pv := range patch {
		bv, exists := r[k]
		if !exists {
			if allowNew {
				r[k] = deepCopyValue(pv)
			}
			continue
		}
		// Both exist and both are objects — recurse
		bm, bIsMap := bv.(map[string]Value)
		pm, pIsMap := pv.(map[string]Value)
		if bIsMap && pIsMap {
			r[k] = Value(deepPatch(bm, pm, allowNew))
		} else {
			// Overwrite with patch value
			r[k] = deepCopyValue(pv)
		}
	}
	return r
}

func deepCopyValue(v Value) Value {
	switch val := v.(type) {
	case map[string]Value:
		c := make(map[string]Value, len(val))
		for k, v2 := range val { c[k] = deepCopyValue(v2) }
		return Value(c)
	case []Value:
		c := make([]Value, len(val))
		for i, v2 := range val { c[i] = deepCopyValue(v2) }
		return Value(c)
	default:
		return v
	}
}

func builtin_patch(args ...Value) Value {
	if len(args) < 2 { return null }
	base, ok1 := args[0].(map[string]Value)
	patchObj, ok2 := args[1].(map[string]Value)
	if !ok1 || !ok2 { return args[0] }
	allowNew := false
	if len(args) >= 3 { allowNew = isTruthy(args[2]) }
	return Value(deepPatch(base, patchObj, allowNew))
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

func builtin_file_open(args ...Value) Value {
	if len(args) == 0 { throw(Value("file.open requires a path")); return null }
	return Value(map[string]Value{"__type": Value("file_handle"), "path": Value(valueToString(args[0]))})
}

func fileHandlePath(obj Value) string {
	if m, ok := obj.(map[string]Value); ok {
		if p, ok := m["path"]; ok { return valueToString(p) }
	}
	throw(Value("not a file handle")); return ""
}

func fileHandleRead(obj Value) Value {
	return builtin_file_read(Value(fileHandlePath(obj)))
}

func fileHandleWrite(obj Value, args ...Value) Value {
	a := []Value{Value(fileHandlePath(obj))}
	a = append(a, args...)
	return builtin_file_write(a...)
}

func fileHandleAppend(obj Value, args ...Value) Value {
	a := []Value{Value(fileHandlePath(obj))}
	a = append(a, args...)
	return builtin_file_append(a...)
}

func fileHandleJSON(obj Value) Value {
	return builtin_file_read_json(Value(fileHandlePath(obj)))
}

func fileHandleWriteJSON(obj Value, args ...Value) Value {
	a := []Value{Value(fileHandlePath(obj))}
	a = append(a, args...)
	return builtin_file_write_json(a...)
}

func fileHandleExists(obj Value) Value {
	return builtin_file_exists(Value(fileHandlePath(obj)))
}

func fileHandleSize(obj Value) Value {
	info, err := os.Stat(fileHandlePath(obj))
	if err != nil { return Value(int64(-1)) }
	return Value(info.Size())
}

func fileHandleDelete(obj Value) Value {
	return builtin_file_delete(Value(fileHandlePath(obj)))
}

func fileHandleGetPath(obj Value) Value {
	return Value(fileHandlePath(obj))
}

func fileHandleLines(obj Value, args ...Value) Value {
	p := fileHandlePath(obj)
	data, err := os.ReadFile(p)
	if err != nil { return Value([]Value{}) }
	raw := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	total := len(raw)
	if total == 0 { return Value([]Value{}) }

	start := int64(0)
	end := int64(total)
	if len(args) == 1 {
		n := toInt64(args[0])
		if n < 0 {
			// last N lines
			start = int64(total) + n
			if start < 0 { start = 0 }
		} else {
			// first N lines
			end = n
			if end > int64(total) { end = int64(total) }
		}
	} else if len(args) >= 2 {
		start = toInt64(args[0])
		end = toInt64(args[1])
		if start < 0 { start = 0 }
		if end > int64(total) { end = int64(total) }
	}

	result := make([]Value, 0, end-start)
	for i := start; i < end; i++ {
		result = append(result, Value(raw[i]))
	}
	return Value(result)
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
	c.emitRuntimeTemplate("cron_runtime.gotmpl", nil)
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
	secretExpr := fmt.Sprintf("%q", c.sessionSecret)
	if c.sessionSecretExpr != nil {
		secretExpr = fmt.Sprintf("valueToString(%s)", c.expr(c.sessionSecretExpr))
	}
	c.emitRuntimeTemplate("session_runtime.gotmpl", sessionRuntimeTemplateData{
		SessionSecretExpr: secretExpr,
		SessionExpires:    c.sessionExpires,
		SessionCookie:     c.sessionCookie,
	})

	// CSRF runtime
	if c.csrfEnabled {
		c.emitCSRFRuntime()
	}
}

func (c *NativeCompiler) emitCSRFRuntime() {
	c.emitRuntimeTemplate("csrf_runtime.gotmpl", csrfRuntimeTemplateData{
		SafeOrigins: c.csrfSafeOrigins,
	})
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
		},
		"safeJS": func(s interface{}) template.JS {
			switch v := s.(type) {
			case string: return template.JS(v)
			default: return template.JS(fmt.Sprint(v))
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
type sseStream struct {
	id       string
	ch       chan sseEvent
	meta     map[string]Value
	channels map[string]bool
	mu       sync.RWMutex  // protects meta and channels
	done     chan struct{}  // closed on disconnect to signal close()
}

type sseEvent struct {
	event string
	data  string
}

type sseStreamHandle struct {
	stream *sseStream
}

type sseChannelHandle struct {
	name string
}

type sseHub struct {
	mu       sync.RWMutex
	streams  map[string]*sseStream               // id -> stream
	channels map[string]map[string]*sseStream     // channel -> id -> stream
}

var _sseHub = &sseHub{
	streams:  make(map[string]*sseStream),
	channels: make(map[string]map[string]*sseStream),
}

func _sseGenerateID() string {
	b := make([]byte, 16)
	crand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func (h *sseHub) register() *sseStreamHandle {
	s := &sseStream{
		id:       _sseGenerateID(),
		ch:       make(chan sseEvent, 64),
		meta:     make(map[string]Value),
		channels: make(map[string]bool),
		done:     make(chan struct{}),
	}
	h.mu.Lock()
	h.streams[s.id] = s
	h.mu.Unlock()
	return &sseStreamHandle{stream: s}
}

func (h *sseHub) unregister(s *sseStream) {
	h.mu.Lock()
	delete(h.streams, s.id)
	// Remove from all channels
	for ch := range s.channels {
		if members, ok := h.channels[ch]; ok {
			delete(members, s.id)
			if len(members) == 0 {
				delete(h.channels, ch)
			}
		}
	}
	h.mu.Unlock()
	close(s.ch)
}

// --- Stream handle methods ---

func _sseStreamSend(handle Value, event string, data Value) Value {
	d := ""
	if data != nil {
		jBytes, err := json.Marshal(valueToGo(data))
		if err == nil { d = string(jBytes) } else { d = valueToString(data) }
	}
	if h, ok := handle.(*sseStreamHandle); ok && h.stream != nil {
		select {
		case h.stream.ch <- sseEvent{event: event, data: d}:
			return Value(true)
		default:
			return Value(false)
		}
	}
	if ch, ok := handle.(*sseChannelHandle); ok {
		_sseHub.mu.RLock()
		members := _sseHub.channels[ch.name]
		allOk := true
		for _, s := range members {
			select {
			case s.ch <- sseEvent{event: event, data: d}:
			default:
				allOk = false
			}
		}
		_sseHub.mu.RUnlock()
		return Value(allOk)
	}
	return Value(false)
}

func _sseStreamSet(handle Value, key string, val Value) Value {
	h, ok := handle.(*sseStreamHandle)
	if !ok || h.stream == nil { return null }
	h.stream.mu.Lock()
	h.stream.meta[key] = val
	h.stream.mu.Unlock()
	return null
}

func _sseStreamGet(handle Value, key string) Value {
	h, ok := handle.(*sseStreamHandle)
	if !ok || h.stream == nil { return null }
	h.stream.mu.RLock()
	v := h.stream.meta[key]
	h.stream.mu.RUnlock()
	if v == nil { return null }
	return v
}

func _sseStreamJoin(handle Value, channel string) Value {
	h, ok := handle.(*sseStreamHandle)
	if !ok || h.stream == nil { return null }
	s := h.stream
	s.mu.Lock()
	s.channels[channel] = true
	s.mu.Unlock()
	_sseHub.mu.Lock()
	if _sseHub.channels[channel] == nil {
		_sseHub.channels[channel] = make(map[string]*sseStream)
	}
	_sseHub.channels[channel][s.id] = s
	_sseHub.mu.Unlock()
	return null
}

func _sseStreamLeave(handle Value, channel string) Value {
	h, ok := handle.(*sseStreamHandle)
	if !ok || h.stream == nil { return null }
	s := h.stream
	s.mu.Lock()
	delete(s.channels, channel)
	s.mu.Unlock()
	_sseHub.mu.Lock()
	if members, ok := _sseHub.channels[channel]; ok {
		delete(members, s.id)
		if len(members) == 0 {
			delete(_sseHub.channels, channel)
		}
	}
	_sseHub.mu.Unlock()
	return null
}

func _sseStreamClose(handle Value) Value {
	h, ok := handle.(*sseStreamHandle)
	if !ok || h.stream == nil { return null }
	select {
	case <-h.stream.done:
		// already closed
	default:
		close(h.stream.done)
	}
	return null
}

func _sseStreamID(handle Value) Value {
	h, ok := handle.(*sseStreamHandle)
	if !ok || h.stream == nil { return null }
	return Value(h.stream.id)
}

func _sseStreamChannels(handle Value) Value {
	h, ok := handle.(*sseStreamHandle)
	if !ok || h.stream == nil { return Value([]Value{}) }
	h.stream.mu.RLock()
	result := make([]Value, 0, len(h.stream.channels))
	for ch := range h.stream.channels {
		result = append(result, Value(&sseChannelHandle{name: ch}))
	}
	h.stream.mu.RUnlock()
	return Value(result)
}

// --- Global SSE namespace functions ---

func _sseFind(id Value) Value {
	idStr := valueToString(id)
	_sseHub.mu.RLock()
	s, ok := _sseHub.streams[idStr]
	_sseHub.mu.RUnlock()
	if !ok || s == nil { return null }
	return Value(&sseStreamHandle{stream: s})
}

func _sseFindBy(key Value, val Value) Value {
	keyStr := valueToString(key)
	_sseHub.mu.RLock()
	var result []Value
	for _, s := range _sseHub.streams {
		s.mu.RLock()
		if mv, ok := s.meta[keyStr]; ok && valuesEqual(mv, val) {
			result = append(result, Value(&sseStreamHandle{stream: s}))
		}
		s.mu.RUnlock()
	}
	_sseHub.mu.RUnlock()
	if result == nil { result = []Value{} }
	return Value(result)
}

func _sseChannel(name Value) Value {
	return Value(&sseChannelHandle{name: valueToString(name)})
}

func _sseBroadcast(args ...Value) Value {
	if len(args) < 2 { return Value(false) }
	event := valueToString(args[0])
	data := ""
	if args[1] != nil {
		jBytes, err := json.Marshal(valueToGo(args[1]))
		if err == nil { data = string(jBytes) } else { data = valueToString(args[1]) }
	}
	_sseHub.mu.RLock()
	allOk := true
	for _, s := range _sseHub.streams {
		select {
		case s.ch <- sseEvent{event: event, data: data}:
		default:
			allOk = false
		}
	}
	_sseHub.mu.RUnlock()
	return Value(allOk)
}

func _sseCount() Value {
	_sseHub.mu.RLock()
	n := len(_sseHub.streams)
	_sseHub.mu.RUnlock()
	return Value(int64(n))
}

func _sseChannels() Value {
	_sseHub.mu.RLock()
	result := make([]Value, 0, len(_sseHub.channels))
	for name := range _sseHub.channels {
		result = append(result, Value(&sseChannelHandle{name: name}))
	}
	_sseHub.mu.RUnlock()
	return Value(result)
}

// --- Channel handle methods ---

func _sseChannelSend(handle Value, event string, data Value) Value {
	ch, ok := handle.(*sseChannelHandle)
	if !ok { return Value(false) }
	d := ""
	if data != nil {
		jBytes, err := json.Marshal(valueToGo(data))
		if err == nil { d = string(jBytes) } else { d = valueToString(data) }
	}
	_sseHub.mu.RLock()
	members := _sseHub.channels[ch.name]
	allOk := true
	for _, s := range members {
		select {
		case s.ch <- sseEvent{event: event, data: d}:
		default:
			allOk = false
		}
	}
	_sseHub.mu.RUnlock()
	return Value(allOk)
}

func _sseChannelStreams(handle Value) Value {
	ch, ok := handle.(*sseChannelHandle)
	if !ok { return Value([]Value{}) }
	_sseHub.mu.RLock()
	members := _sseHub.channels[ch.name]
	result := make([]Value, 0, len(members))
	for _, s := range members {
		result = append(result, Value(&sseStreamHandle{stream: s}))
	}
	_sseHub.mu.RUnlock()
	return Value(result)
}

func _sseChannelCount(handle Value) Value {
	ch, ok := handle.(*sseChannelHandle)
	if !ok { return Value(int64(0)) }
	_sseHub.mu.RLock()
	n := len(_sseHub.channels[ch.name])
	_sseHub.mu.RUnlock()
	return Value(int64(n))
}

// _sseDotValue handles property access on SSE handles, falling through to dotValue for maps
func _sseDotValue(obj Value, field string) Value {
	obj = resolveValue(obj)
	if h, ok := obj.(*sseStreamHandle); ok && h.stream != nil {
		switch field {
		case "id":
			return Value(h.stream.id)
		}
		return null
	}
	if ch, ok := obj.(*sseChannelHandle); ok {
		switch field {
		case "name":
			return Value(ch.name)
		}
		return null
	}
	if m, ok := obj.(map[string]Value); ok {
		if v, ok := m[field]; ok { return v }
	}
	return null
}
`)
}
