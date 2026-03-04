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
	functions    []*FnStatement
	usedBuiltins map[string]bool
	usedImports  map[string]bool
	needsBcrypt  bool
	tmpCounter   int
}

func GenerateNativeCode(program *Program) (string, error) {
	c := &NativeCompiler{
		port:         8080,
		usedBuiltins: make(map[string]bool),
		usedImports:  make(map[string]bool),
	}
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *RouteStatement:
			c.routes = append(c.routes, s)
			c.scanBlock(s.Body)
		case *FnStatement:
			c.functions = append(c.functions, s)
			c.scanBlock(s.Body)
		case *ServerStatement:
			if pe, ok := s.Settings["port"]; ok {
				if lit, ok := pe.(*IntegerLiteral); ok {
					c.port = int(lit.Value)
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
	if c.usedBuiltins["store"] {
		c.usedImports["sync"] = true
	}
	if c.usedBuiltins["sort"] {
		c.usedImports["sort"] = true
	}
	if c.usedBuiltins["match"] {
		c.usedImports["regexp"] = true
	}
	if c.usedBuiltins["env"] {
		c.usedImports["os"] = true
	}
	if c.usedBuiltins["sleep"] || c.usedBuiltins["now"] || c.usedBuiltins["now_ms"] {
		c.usedImports["time"] = true
	}
	if c.usedBuiltins["abs"] || c.usedBuiltins["ceil"] || c.usedBuiltins["floor"] || c.usedBuiltins["round"] {
		c.usedImports["math"] = true
	}
	if c.usedBuiltins["random"] || c.usedBuiltins["random_int"] {
		c.usedImports["math/rand"] = true
	}
	if c.usedBuiltins["uuid"] {
		c.usedImports["crypto/rand"] = true
	}
	if c.usedBuiltins["sha256"] || c.usedBuiltins["hmac"] {
		c.usedImports["crypto/sha256"] = true
	}
	if c.usedBuiltins["md5"] {
		c.usedImports["crypto/md5"] = true
	}
	if c.usedBuiltins["hmac"] {
		c.usedImports["crypto/hmac"] = true
	}
	if c.usedBuiltins["sha256"] || c.usedBuiltins["md5"] || c.usedBuiltins["hmac"] {
		c.usedImports["encoding/hex"] = true
	}
	if c.usedBuiltins["base64_encode"] || c.usedBuiltins["base64_decode"] {
		c.usedImports["encoding/base64"] = true
	}
	if c.usedBuiltins["url_encode"] || c.usedBuiltins["url_decode"] {
		c.usedImports["net/url"] = true
	}
	if c.usedBuiltins["http_get"] || c.usedBuiltins["http_post"] {
		c.usedImports["bytes"] = true
	}
	if c.usedBuiltins["bcrypt_hash"] || c.usedBuiltins["bcrypt_verify"] {
		c.needsBcrypt = true
	}
	if c.usedBuiltins["log"] || c.usedBuiltins["log_info"] || c.usedBuiltins["log_warn"] || c.usedBuiltins["log_error"] {
		c.usedImports["log"] = true
	}

	c.emitHeader()
	c.emitRuntime()
	c.emitBuiltinFuncs()
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
	case *HTTPReturnStatement:
		c.scanExpr(s.Body)
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

func (c *NativeCompiler) emitHeader() {
	c.ln("// Code generated by httpdsl native compiler. DO NOT EDIT.")
	c.ln("package main")
	c.ln("")
	c.ln("import (")
	c.indent++
	stdlib := []string{"bytes", "context", "crypto/hmac", "crypto/md5", "crypto/rand",
		"crypto/sha256", "encoding/base64", "encoding/hex", "encoding/json",
		"fmt", "io", "log", "math", "math/rand", "net/http", "net/url",
		"os", "regexp", "sort", "strconv", "strings", "sync", "time"}
	for _, imp := range stdlib {
		if c.usedImports[imp] {
			if imp == "math/rand" {
				c.lnf("mrand %q", imp)
			} else {
				c.lnf("%q", imp)
			}
		}
	}
	if c.needsBcrypt {
		c.ln("")
		c.lnf("%q", "golang.org/x/crypto/bcrypt")
	}
	c.indent--
	c.ln(")")
	c.ln("")
}

func (c *NativeCompiler) emitRuntime() {
	c.raw(`// ===== Runtime =====
type Value = interface{}
type nullType struct{}
var null Value = &nullType{}
type multiReturn struct{ Values []Value }

type ctxKey int
const paramsKey ctxKey = 0

func getParams(r *http.Request) map[string]Value {
	if v := r.Context().Value(paramsKey); v != nil {
		return v.(map[string]Value)
	}
	return map[string]Value{}
}

func isTruthy(v Value) bool {
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
	switch av := a.(type) {
	case int64:
		if bv, ok := b.(int64); ok { return av == bv }
		if bv, ok := b.(float64); ok { return float64(av) == bv }
	case float64:
		if bv, ok := b.(float64); ok { return av == bv }
		if bv, ok := b.(int64); ok { return av == float64(bv) }
	case string:
		if bv, ok := b.(string); ok { return av == bv }
	case bool:
		if bv, ok := b.(bool); ok { return av == bv }
	case *nullType:
		_, ok := b.(*nullType); return ok || b == nil
	case nil:
		return b == nil
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
	if m, ok := obj.(map[string]Value); ok {
		if v, ok := m[field]; ok { return v }
	}
	return null
}

func setIndex(obj, idx, val Value) {
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
	literal string
	param   string
}
type dslRouter struct {
	routes []routeEntry
}

func (rt *dslRouter) add(method, pattern string, h http.HandlerFunc) {
	parts := strings.Split(strings.Trim(pattern, "/"), "/")
	segs := make([]routeSeg, len(parts))
	for i, p := range parts {
		if strings.HasPrefix(p, ":") {
			segs[i] = routeSeg{param: p[1:]}
		} else {
			segs[i] = routeSeg{literal: p}
		}
	}
	rt.routes = append(rt.routes, routeEntry{method: method, segments: segs, handler: h})
}

func (rt *dslRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	for _, route := range rt.routes {
		if route.method != r.Method || len(parts) != len(route.segments) { continue }
		ok := true
		for i, seg := range route.segments {
			if seg.param == "" && seg.literal != parts[i] { ok = false; break }
		}
		if !ok { continue }
		params := make(map[string]Value)
		for i, seg := range route.segments {
			if seg.param != "" { params[seg.param] = Value(parts[i]) }
		}
		ctx := context.WithValue(r.Context(), paramsKey, params)
		route.handler(w, r.WithContext(ctx))
		return
	}
	w.WriteHeader(404)
	w.Write([]byte("{\"error\":\"not found\"}"))
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
	c.ln("")
}

func (c *NativeCompiler) emitUserFunctions() {
	c.ln("// ===== User Functions =====")
	for _, fn := range c.functions {
		c.emitFnDef(fn)
	}
	c.ln("")
}

func (c *NativeCompiler) emitFnDef(fn *FnStatement) {
	params := make([]string, len(fn.Params))
	paramSet := make(map[string]bool)
	for i, p := range fn.Params {
		params[i] = fmt.Sprintf("%s Value", safeIdent(p))
		paramSet[p] = true
	}
	c.lnf("func fn_%s(%s) Value {", safeIdent(fn.Name), strings.Join(params, ", "))
	c.indent++
	// Declare local variables (excluding params)
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
		// check if it's a call we need to keep (print, etc)
		c.lnf("_ = %s", c.expr(s.Expression))
	case *ReturnStatement:
		if len(s.Values) == 0 {
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
	case *HTTPReturnStatement:
		c.emitHTTPReturn(s)
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
	}
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

func (c *NativeCompiler) emitHTTPReturn(s *HTTPReturnStatement) {
	bodyExpr := c.expr(s.Body)
	switch s.ResponseType {
	case "json":
		c.ln("_w.Header().Set(\"Content-Type\", \"application/json\")")
		c.lnf("_w.WriteHeader(%d)", s.StatusCode)
		c.lnf("json.NewEncoder(_w).Encode(valueToGo(%s))", bodyExpr)
	case "text":
		c.ln("_w.Header().Set(\"Content-Type\", \"text/plain\")")
		c.lnf("_w.WriteHeader(%d)", s.StatusCode)
		c.lnf("fmt.Fprint(_w, valueToString(%s))", bodyExpr)
	}
	c.ln("return")
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
		// Anonymous function
		params := make([]string, len(ex.Params))
		for i, p := range ex.Params {
			params[i] = fmt.Sprintf("%s Value", safeIdent(p))
		}
		var fb strings.Builder
		old := c.b
		c.b = fb
		c.emitBlock(ex.Body, false)
		c.b.WriteString(strings.Repeat("\t", c.indent+1))
		c.b.WriteString("return null\n")
		body := c.b.String()
		c.b = old
		return fmt.Sprintf("Value(func(%s) Value {\n%s%s})", strings.Join(params, ", "), body, strings.Repeat("\t", c.indent))
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
		"repeat": true, "flat": true,
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
	}

	// Handle known builtins by name
	if ident, ok := e.Function.(*Identifier); ok {
		switch ident.Value {
		case "print":
			return fmt.Sprintf("builtin_print(%s)", argStr)
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
		}

		// User-defined function
		for _, fn := range c.functions {
			if fn.Name == ident.Value {
				return fmt.Sprintf("fn_%s(%s)", safeIdent(ident.Value), argStr)
			}
		}
		// Unknown - might be a variable holding a function
		return fmt.Sprintf("%s(%s)", safeIdent(ident.Value), argStr)
	}

	// General case: expression call
	fnExpr := c.expr(e.Function)
	return fmt.Sprintf("%s(%s)", fnExpr, argStr)
}

func (c *NativeCompiler) dotExpr(e *DotExpression) string {
	// json and store are handled at call sites
	if ident, ok := e.Left.(*Identifier); ok {
		if ident.Value == "json" || ident.Value == "store" {
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
	c.ln("rt := &dslRouter{}")
	c.ln("")

	for _, route := range c.routes {
		c.emitRoute(route)
	}

	c.lnf("addr := \":%d\"", c.port)
	c.ln(`fmt.Printf("httpdsl native server on %s\n", addr)`)
	c.ln("if err := http.ListenAndServe(addr, rt); err != nil {")
	c.indent++
	c.ln(`fmt.Printf("Server error: %s\n", err)`)
	c.indent--
	c.ln("}")
	c.indent--
	c.ln("}")
}

func (c *NativeCompiler) emitRoute(route *RouteStatement) {
	c.lnf("rt.add(%q, %q, func(_w http.ResponseWriter, _r *http.Request) {", route.Method, route.Path)
	c.indent++

	// Declare all variables used in the route body
	vars := c.collectVars(route.Body)
	// Always have params and request
	c.ln("_params := getParams(_r)")

	// Build request object
	c.ln("_bodyBytes, _ := io.ReadAll(_r.Body)")
	c.ln("defer _r.Body.Close()")
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
	c.ln("_headerMap := make(map[string]Value)")
	c.ln("for _k, _v := range _r.Header {")
	c.indent++
	c.ln("if len(_v) == 1 { _headerMap[_k] = Value(_v[0]) } else {")
	c.indent++
	c.ln("_arr := make([]Value, len(_v))")
	c.ln("for _i, _s := range _v { _arr[_i] = Value(_s) }")
	c.ln("_headerMap[_k] = Value(_arr)")
	c.indent--
	c.ln("}")
	c.indent--
	c.ln("}")

	// Set up params and request as variables
	c.ln("params := _params")
	c.ln("request := Value(map[string]Value{")
	c.indent++
	c.ln("\"body\": Value(string(_bodyBytes)),")
	c.ln("\"method\": Value(_r.Method),")
	c.ln("\"path\": Value(_r.URL.Path),")
	c.ln("\"query\": Value(_queryMap),")
	c.ln("\"headers\": Value(_headerMap),")
	c.indent--
	c.ln("})")
	c.ln("_ = params")
	c.ln("_ = request")

	// Declare variables
	for name := range vars {
		if name != "params" && name != "request" {
			c.lnf("var %s Value = null", safeIdent(name))
		}
	}
	c.ln("")

	// Emit route body
	c.emitBlock(route.Body, true)

	c.indent--
	c.ln("})")
	c.ln("")
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
	}
}
