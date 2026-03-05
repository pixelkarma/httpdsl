package compiler

import (
	"fmt"
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
	needsBcrypt  bool
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
	globalBefore   []*BlockStatement
	globalAfter    []*BlockStatement
	routeBeforeMap map[*RouteStatement][]*BlockStatement // group before blocks per route
	routeAfterMap  map[*RouteStatement][]*BlockStatement // group after blocks per route
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
	}
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *RouteStatement:
			c.routes = append(c.routes, s)
			c.scanBlock(s.Body)
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
	if c.usedBuiltins["bcrypt_hash"] || c.usedBuiltins["bcrypt_verify"] {
		c.needsBcrypt = true
	}

	if c.usedBuiltins["file"] {
		c.usedImports["os"] = true
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

	c.emitHeader()
	c.emitRuntime()
	c.emitBuiltinFuncs()
	c.emitDBRuntime()
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
	case *InfixExpression:
		c.scanExpr(e.Left); c.scanExpr(e.Right)
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
	stdlib := []string{"bytes", "context", "crypto/hmac", "crypto/md5", "crypto/rand",
		"crypto/sha256", "crypto/sha512", "database/sql", "encoding/base64", "encoding/hex", "encoding/json",
		"fmt", "hash", "io", "math", "math/rand", "net/http", "net/url",
		"os", "regexp", "sort", "strconv", "strings", "sync", "time"}
	for _, imp := range stdlib {
		if c.usedImports[imp] {
			switch imp {
			case "math/rand":
				c.lnf("mrand %q", imp)
			case "crypto/rand":
				c.lnf("crand %q", imp)
			default:
				c.lnf("%q", imp)
			}
		}
	}
	if c.needsBcrypt {
		c.ln("")
		c.lnf("%q", "golang.org/x/crypto/bcrypt")
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
type dslRouter struct {
	routes        []routeEntry
	errorHandlers map[int]http.HandlerFunc
	limiter       *rateLimiter
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
// Concurrent store
type concurrentStore struct {
	mu   sync.RWMutex
	data map[string]Value
}
var globalStore = &concurrentStore{data: make(map[string]Value)}

func storeGet(key string, def Value) Value {
	globalStore.mu.RLock()
	v, ok := globalStore.data[key]
	globalStore.mu.RUnlock()
	if !ok { return def }
	return v
}
func storeSet(key string, val Value) Value {
	globalStore.mu.Lock()
	globalStore.data[key] = val
	globalStore.mu.Unlock()
	return val
}
func storeDelete(key string) {
	globalStore.mu.Lock()
	delete(globalStore.data, key)
	globalStore.mu.Unlock()
}
func storeHas(key string) bool {
	globalStore.mu.RLock()
	_, ok := globalStore.data[key]
	globalStore.mu.RUnlock()
	return ok
}
func storeAll() map[string]Value {
	globalStore.mu.RLock()
	r := make(map[string]Value, len(globalStore.data))
	for k, v := range globalStore.data { r[k] = v }
	globalStore.mu.RUnlock()
	return r
}
func storeIncr(key string, amount int64) int64 {
	globalStore.mu.Lock()
	defer globalStore.mu.Unlock()
	cur, _ := globalStore.data[key]
	var n int64
	if ci, ok := cur.(int64); ok { n = ci + amount } else { n = amount }
	globalStore.data[key] = n
	return n
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
	c.ln("")
}

func (c *NativeCompiler) emitDBRuntime() {
	if len(c.dbDrivers) == 0 {
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
		if !paramSet[name] {
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
		if !paramSet[name] {
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
		"hash": true, "hmac_hash": true, "log": true, "log_info": true, "log_warn": true, "log_error": true,
		"map": true, "filter": true, "reduce": true,
		"date": true, "date_format": true, "date_parse": true, "strtotime": true,
		"redirect": true,
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
		return fmt.Sprintf("Value(isTruthy(%s) && isTruthy(%s))", l, r)
	case "||":
		return fmt.Sprintf("Value(isTruthy(%s) || isTruthy(%s))", l, r)
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
					if len(args) >= 2 {
						return fmt.Sprintf("storeSet(valueToString(%s), %s)", args[0], args[1])
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
					if len(args) >= 2 {
						return fmt.Sprintf("Value(storeIncr(valueToString(%s), toInt64(%s)))", args[0], args[1])
					} else if len(args) == 1 {
						return fmt.Sprintf("Value(storeIncr(valueToString(%s), 1))", args[0])
					}
					return "Value(int64(0))"
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
		if ident.Value == "json" || ident.Value == "store" || ident.Value == "file" || ident.Value == "db" || ident.Value == "jwt" {
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
	c.ln("rt := &dslRouter{errorHandlers: make(map[int]http.HandlerFunc)}")
	if c.throttleRPS > 0 {
		c.lnf("rt.limiter = newRateLimiter(%d)", c.throttleRPS)
	}
	c.ln("")

	for _, route := range c.routes {
		c.emitRoute(route)
	}

	for _, eh := range c.errorHandlers {
		c.emitErrorHandler(eh)
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
		c.ln("if err := http.ListenAndServe(addr, handler); err != nil {")
	} else {
		c.ln("if err := http.ListenAndServe(addr, rt); err != nil {")
	}
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
func (c *NativeCompiler) emitRoute(route *RouteStatement) {
	c.lnf("rt.add(%q, %q, func(_w http.ResponseWriter, _r *http.Request) {", route.Method, route.Path)
	c.indent++
	c.inRouteHandler = true
	defer func() { c.inRouteHandler = false }()

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
				if name != "request" && name != "response" && name != "error" {
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
		if name != "request" && name != "response" {
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
			if name != "request" && name != "response" && name != "error" {
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

	// Auto-return response
	c.ln("writeResponse(_w, response)")

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
		if name != "request" && name != "response" {
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
