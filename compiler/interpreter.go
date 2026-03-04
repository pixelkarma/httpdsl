package compiler

import (
	"encoding/json"
	"fmt"
	"httpdsl/runtime"
	"io"
	"net/http"
	"strconv"
	"sync"
)

// Value types
type Value interface{}
type NullValue struct{}
type BreakSignal struct{}
type ContinueSignal struct{}

type ReturnValue struct {
	Values []Value
}

type HTTPReturnValue struct {
	ResponseType string
	StatusCode   int
	Body         Value
}

type FunctionValue struct {
	Params []string
	Body   *BlockStatement
	Env    *Environment
}

type BuiltinFunction struct {
	Fn func(args ...Value) Value
}

// EnvBuiltinFunction has access to the calling environment (for header, cookie, etc.)
type EnvBuiltinFunction struct {
	Fn func(env *Environment, args ...Value) Value
}

// RedirectValue signals an HTTP redirect from a route handler
type RedirectValue struct {
	URL        string
	StatusCode int
}

// Environment (scope)
type Environment struct {
	mu       sync.RWMutex
	store    map[string]Value
	outer    *Environment
	boundary bool // true = function boundary, stops SetExisting from walking up
}

func NewEnvironment() *Environment {
	return &Environment{store: make(map[string]Value)}
}

func NewEnclosedEnvironment(outer *Environment) *Environment {
	env := NewEnvironment()
	env.outer = outer
	return env
}

func NewFunctionEnvironment(outer *Environment) *Environment {
	env := NewEnvironment()
	env.outer = outer
	env.boundary = true
	return env
}

func (e *Environment) Get(name string) (Value, bool) {
	e.mu.RLock()
	val, ok := e.store[name]
	e.mu.RUnlock()
	if !ok && e.outer != nil {
		return e.outer.Get(name)
	}
	return val, ok
}

func (e *Environment) Set(name string, val Value) {
	e.mu.Lock()
	e.store[name] = val
	e.mu.Unlock()
}

// SetExisting sets in the scope where the variable already exists,
// but won't cross function boundaries (boundary == true)
func (e *Environment) SetExisting(name string, val Value) {
	e.mu.RLock()
	_, ok := e.store[name]
	e.mu.RUnlock()
	if ok {
		e.mu.Lock()
		e.store[name] = val
		e.mu.Unlock()
		return
	}
	if e.outer != nil && !e.boundary {
		e.outer.SetExisting(name, val)
		return
	}
	// Create in current scope
	e.mu.Lock()
	e.store[name] = val
	e.mu.Unlock()
}

// ConcurrentStore is a thread-safe key-value store for cross-request state
type ConcurrentStore struct {
	mu   sync.RWMutex
	data map[string]Value
}

func NewConcurrentStore() *ConcurrentStore {
	return &ConcurrentStore{data: make(map[string]Value)}
}

func (s *ConcurrentStore) Get(key string) (Value, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok
}

func (s *ConcurrentStore) SetVal(key string, val Value) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = val
}

func (s *ConcurrentStore) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
}

func (s *ConcurrentStore) All() map[string]Value {
	s.mu.RLock()
	defer s.mu.RUnlock()
	copy_ := make(map[string]Value, len(s.data))
	for k, v := range s.data {
		copy_[k] = v
	}
	return copy_
}

// Interpreter
type Interpreter struct {
	global *Environment
	Server *runtime.Server
	Store  *ConcurrentStore
}

func NewInterpreter() *Interpreter {
	i := &Interpreter{
		global: NewEnvironment(),
		Server: runtime.NewServer(),
		Store:  NewConcurrentStore(),
	}
	i.registerBuiltins()
	i.registerStoreBuiltins()
	return i
}

func (interp *Interpreter) registerStoreBuiltins() {
	// Thread-safe store namespace for cross-request state
	storeObj := map[string]Value{
		"get": &BuiltinFunction{Fn: func(args ...Value) Value {
			if len(args) < 1 {
				return &NullValue{}
			}
			key := valueToString(args[0])
			val, ok := interp.Store.Get(key)
			if !ok {
				if len(args) >= 2 {
					return args[1] // default value
				}
				return &NullValue{}
			}
			return val
		}},
		"set": &BuiltinFunction{Fn: func(args ...Value) Value {
			if len(args) < 2 {
				return &NullValue{}
			}
			interp.Store.SetVal(valueToString(args[0]), args[1])
			return args[1]
		}},
		"delete": &BuiltinFunction{Fn: func(args ...Value) Value {
			if len(args) < 1 {
				return &NullValue{}
			}
			interp.Store.Delete(valueToString(args[0]))
			return &NullValue{}
		}},
		"has": &BuiltinFunction{Fn: func(args ...Value) Value {
			if len(args) < 1 {
				return false
			}
			_, ok := interp.Store.Get(valueToString(args[0]))
			return ok
		}},
		"all": &BuiltinFunction{Fn: func(args ...Value) Value {
			return interp.Store.All()
		}},
		"incr": &BuiltinFunction{Fn: func(args ...Value) Value {
			if len(args) < 1 {
				return int64(0)
			}
			key := valueToString(args[0])
			amount := int64(1)
			if len(args) >= 2 {
				if n, ok := args[1].(int64); ok {
					amount = n
				}
			}
			interp.Store.mu.Lock()
			defer interp.Store.mu.Unlock()
			cur, _ := interp.Store.data[key]
			var newVal int64
			if n, ok := cur.(int64); ok {
				newVal = n + amount
			} else {
				newVal = amount
			}
			interp.Store.data[key] = newVal
			return newVal
		}},
	}
	interp.global.Set("store", storeObj)
}

// callFunction invokes a function value with the given arguments
func (interp *Interpreter) callFunction(fn Value, args []Value) Value {
	switch f := fn.(type) {
	case *BuiltinFunction:
		return f.Fn(args...)
	case *FunctionValue:
		fnEnv := NewFunctionEnvironment(f.Env)
		for i, param := range f.Params {
			if i < len(args) {
				fnEnv.Set(param, args[i])
			} else {
				fnEnv.Set(param, &NullValue{})
			}
		}
		result := interp.execStatements(f.Body.Statements, fnEnv)
		if rv, ok := result.(*ReturnValue); ok {
			if len(rv.Values) == 1 {
				return rv.Values[0]
			}
			return rv
		}
		if hrv, ok := result.(*HTTPReturnValue); ok {
			return hrv
		}
		return &NullValue{}
	default:
		return &NullValue{}
	}
}

// Execute a program
func (interp *Interpreter) Execute(program *Program) {
	interp.execStatements(program.Statements, interp.global)
}

func (interp *Interpreter) execStatements(stmts []Statement, env *Environment) Value {
	for _, stmt := range stmts {
		result := interp.execStatement(stmt, env)
		switch result.(type) {
		case *ReturnValue, *HTTPReturnValue, *BreakSignal, *ContinueSignal:
			return result
		}
	}
	return &NullValue{}
}

func (interp *Interpreter) execStatement(stmt Statement, env *Environment) Value {
	switch s := stmt.(type) {
	case *RouteStatement:
		return interp.execRouteStatement(s, env)
	case *FnStatement:
		return interp.execFnStatement(s, env)
	case *ReturnStatement:
		vals := make([]Value, len(s.Values))
		for i, v := range s.Values {
			vals[i] = interp.evalExpr(v, env)
		}
		return &ReturnValue{Values: vals}
	case *HTTPReturnStatement:
		body := interp.evalExpr(s.Body, env)
		return &HTTPReturnValue{
			ResponseType: s.ResponseType,
			StatusCode:   s.StatusCode,
			Body:         body,
		}
	case *AssignStatement:
		return interp.execAssign(s, env)
	case *IndexAssignStatement:
		return interp.execIndexAssign(s, env)
	case *CompoundAssignStatement:
		return interp.execCompoundAssign(s, env)
	case *IfStatement:
		return interp.execIf(s, env)
	case *WhileStatement:
		return interp.execWhile(s, env)
	case *EachStatement:
		return interp.execEach(s, env)
	case *ServerStatement:
		return interp.execServer(s, env)
	case *ExpressionStatement:
		interp.evalExpr(s.Expression, env)
		return &NullValue{}
	case *BlockStatement:
		return interp.execStatements(s.Statements, env)
	case *BreakStatement:
		return &BreakSignal{}
	case *ContinueStatement:
		return &ContinueSignal{}
	default:
		return &NullValue{}
	}
}

func (interp *Interpreter) execRouteStatement(s *RouteStatement, env *Environment) Value {
	// Capture the route body and register it as an HTTP handler
	body := s.Body
	method := s.Method
	path := s.Path

	interp.Server.Router.Add(method, path, func(w http.ResponseWriter, r *http.Request, pathParams map[string]string) {
		// Create a new env for this request (function boundary)
		routeEnv := NewFunctionEnvironment(env)

		// Response context for header()/cookie() builtins
		routeEnv.Set("_response_headers", make(map[string]Value))
		routeEnv.Set("_response_cookies", []Value{})

		// Set up params
		paramsMap := make(map[string]Value)
		for k, v := range pathParams {
			paramsMap[k] = v
		}
		routeEnv.Set("params", paramsMap)

		// Set up request object
		bodyBytes, _ := io.ReadAll(r.Body)
		defer r.Body.Close()

		queryMap := make(map[string]Value)
		for k, v := range r.URL.Query() {
			if len(v) == 1 {
				queryMap[k] = v[0]
			} else {
				arr := make([]Value, len(v))
				for i, s := range v {
					arr[i] = s
				}
				queryMap[k] = arr
			}
		}

		headerMap := make(map[string]Value)
		for k, v := range r.Header {
			if len(v) == 1 {
				headerMap[k] = v[0]
			} else {
				arr := make([]Value, len(v))
				for i, s := range v {
					arr[i] = s
				}
				headerMap[k] = arr
			}
		}

		reqObj := map[string]Value{
			"body":    string(bodyBytes),
			"method":  r.Method,
			"path":    r.URL.Path,
			"query":   queryMap,
			"headers": headerMap,
		}
		routeEnv.Set("request", reqObj)

		result := interp.execStatements(body.Statements, routeEnv)

		// Apply accumulated response headers
		if hdrs, ok := routeEnv.Get("_response_headers"); ok {
			if hmap, ok := hdrs.(map[string]Value); ok {
				for k, v := range hmap {
					w.Header().Set(k, valueToString(v))
				}
			}
		}

		// Apply accumulated cookies
		if cookies, ok := routeEnv.Get("_response_cookies"); ok {
			if carr, ok := cookies.([]Value); ok {
				for _, c := range carr {
					if cs, ok := c.(string); ok {
						w.Header().Add("Set-Cookie", cs)
					}
				}
			}
		}

		switch rv := result.(type) {
		case *HTTPReturnValue:
			switch rv.ResponseType {
			case "json":
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(rv.StatusCode)
				data := valueToGo(rv.Body)
				json.NewEncoder(w).Encode(data)
			case "text":
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(rv.StatusCode)
				fmt.Fprint(w, valueToString(rv.Body))
			}
		case *ReturnValue:
			// Check if any return value is a redirect
			if len(rv.Values) > 0 {
				if redir, ok := rv.Values[0].(*RedirectValue); ok {
					http.Redirect(w, r, redir.URL, redir.StatusCode)
					return
				}
			}
			w.Header().Set("Content-Type", "application/json")
			if len(rv.Values) > 0 {
				data := valueToGo(rv.Values[0])
				json.NewEncoder(w).Encode(data)
			}
		default:
			w.WriteHeader(200)
		}
	})

	return &NullValue{}
}

func (interp *Interpreter) execFnStatement(s *FnStatement, env *Environment) Value {
	fn := &FunctionValue{
		Params: s.Params,
		Body:   s.Body,
		Env:    env,
	}
	env.Set(s.Name, fn)
	return &NullValue{}
}

func (interp *Interpreter) execAssign(s *AssignStatement, env *Environment) Value {
	if len(s.Values) == 1 && len(s.Names) > 1 {
		// Multiple names, one value (must be a function call returning multiple values)
		val := interp.evalExpr(s.Values[0], env)
		if rv, ok := val.(*ReturnValue); ok {
			for i, name := range s.Names {
				if i < len(rv.Values) {
					env.SetExisting(name, rv.Values[i])
				} else {
					env.SetExisting(name, &NullValue{})
				}
			}
		} else {
			env.SetExisting(s.Names[0], val)
			for i := 1; i < len(s.Names); i++ {
				env.SetExisting(s.Names[i], &NullValue{})
			}
		}
	} else {
		for i, name := range s.Names {
			if i < len(s.Values) {
				env.SetExisting(name, interp.evalExpr(s.Values[i], env))
			}
		}
	}
	return &NullValue{}
}

func (interp *Interpreter) execIndexAssign(s *IndexAssignStatement, env *Environment) Value {
	left := interp.evalExpr(s.Left, env)
	idx := interp.evalExpr(s.Index, env)
	val := interp.evalExpr(s.Value, env)

	switch obj := left.(type) {
	case map[string]Value:
		key := valueToString(idx)
		obj[key] = val
	case []Value:
		if i, ok := idx.(int64); ok && i >= 0 && int(i) < len(obj) {
			obj[i] = val
		}
	}
	return &NullValue{}
}

func (interp *Interpreter) execCompoundAssign(s *CompoundAssignStatement, env *Environment) Value {
	current, _ := env.Get(s.Name)
	right := interp.evalExpr(s.Value, env)
	var result Value
	switch s.Operator {
	case "+=":
		result = addValues(current, right)
	case "-=":
		result = subtractValues(current, right)
	default:
		result = current
	}
	env.SetExisting(s.Name, result)
	return &NullValue{}
}

func (interp *Interpreter) execIf(s *IfStatement, env *Environment) Value {
	cond := interp.evalExpr(s.Condition, env)
	if isTruthy(cond) {
		return interp.execStatements(s.Consequence.Statements, env)
	}
	if s.Alternative != nil {
		return interp.execStatement(s.Alternative, env)
	}
	return &NullValue{}
}

func (interp *Interpreter) execWhile(s *WhileStatement, env *Environment) Value {
	for {
		cond := interp.evalExpr(s.Condition, env)
		if !isTruthy(cond) {
			break
		}
		result := interp.execStatements(s.Body.Statements, env)
		if _, ok := result.(*BreakSignal); ok {
			break
		}
		if _, ok := result.(*ContinueSignal); ok {
			continue
		}
		if _, ok := result.(*ReturnValue); ok {
			return result
		}
		if _, ok := result.(*HTTPReturnValue); ok {
			return result
		}
	}
	return &NullValue{}
}

func (interp *Interpreter) execEach(s *EachStatement, env *Environment) Value {
	iterable := interp.evalExpr(s.Iterable, env)

	switch v := iterable.(type) {
	case []Value:
		for i, item := range v {
			loopEnv := NewEnclosedEnvironment(env)
			loopEnv.Set(s.Value, item)
			if s.Index != "" {
				loopEnv.Set(s.Index, int64(i))
			}
			result := interp.execStatements(s.Body.Statements, loopEnv)
			if _, ok := result.(*BreakSignal); ok {
				break
			}
			if _, ok := result.(*ContinueSignal); ok {
				continue
			}
			if _, ok := result.(*ReturnValue); ok {
				return result
			}
			if _, ok := result.(*HTTPReturnValue); ok {
				return result
			}
		}
	case map[string]Value:
		i := int64(0)
		for key, val := range v {
			loopEnv := NewEnclosedEnvironment(env)
			loopEnv.Set(s.Value, key)
			if s.Index != "" {
				loopEnv.Set(s.Index, val)
			}
			result := interp.execStatements(s.Body.Statements, loopEnv)
			if _, ok := result.(*BreakSignal); ok {
				break
			}
			if _, ok := result.(*ContinueSignal); ok {
				continue
			}
			if _, ok := result.(*ReturnValue); ok {
				return result
			}
			if _, ok := result.(*HTTPReturnValue); ok {
				return result
			}
			i++
		}
	}
	return &NullValue{}
}

func (interp *Interpreter) execServer(s *ServerStatement, env *Environment) Value {
	for key, expr := range s.Settings {
		val := interp.evalExpr(expr, env)
		switch key {
		case "port":
			if p, ok := val.(int64); ok {
				interp.Server.Port = int(p)
			}
		}
	}
	return &NullValue{}
}

// --- Expression evaluation ---

func (interp *Interpreter) evalExpr(expr Expression, env *Environment) Value {
	if expr == nil {
		return &NullValue{}
	}
	switch e := expr.(type) {
	case *IntegerLiteral:
		return e.Value
	case *FloatLiteral:
		return e.Value
	case *StringLiteral:
		return e.Value
	case *BooleanLiteral:
		return e.Value
	case *NullLiteral:
		return &NullValue{}
	case *Identifier:
		val, ok := env.Get(e.Value)
		if !ok {
			return &NullValue{}
		}
		return val
	case *ArrayLiteral:
		elements := make([]Value, len(e.Elements))
		for i, el := range e.Elements {
			elements[i] = interp.evalExpr(el, env)
		}
		return elements
	case *HashLiteral:
		result := make(map[string]Value)
		for _, pair := range e.Pairs {
			key := interp.evalExpr(pair.Key, env)
			val := interp.evalExpr(pair.Value, env)
			result[valueToString(key)] = val
		}
		return result
	case *PrefixExpression:
		right := interp.evalExpr(e.Right, env)
		return interp.evalPrefix(e.Operator, right)
	case *InfixExpression:
		left := interp.evalExpr(e.Left, env)
		right := interp.evalExpr(e.Right, env)
		return interp.evalInfix(e.Operator, left, right)
	case *CallExpression:
		return interp.evalCall(e, env)
	case *DotExpression:
		return interp.evalDot(e, env)
	case *IndexExpression:
		left := interp.evalExpr(e.Left, env)
		idx := interp.evalExpr(e.Index, env)
		return interp.evalIndex(left, idx)
	default:
		return &NullValue{}
	}
}

func (interp *Interpreter) evalPrefix(op string, right Value) Value {
	switch op {
	case "!":
		return !isTruthy(right)
	case "-":
		switch v := right.(type) {
		case int64:
			return -v
		case float64:
			return -v
		}
	}
	return &NullValue{}
}

func (interp *Interpreter) evalInfix(op string, left, right Value) Value {
	switch op {
	case "+":
		return addValues(left, right)
	case "-":
		return subtractValues(left, right)
	case "*":
		return multiplyValues(left, right)
	case "/":
		return divideValues(left, right)
	case "%":
		return modValues(left, right)
	case "==":
		return valuesEqual(left, right)
	case "!=":
		return !valuesEqual(left, right).(bool)
	case "<":
		return compareLess(left, right)
	case ">":
		return compareLess(right, left)
	case "<=":
		l := compareLess(left, right)
		eq := valuesEqual(left, right)
		return l.(bool) || eq.(bool)
	case ">=":
		l := compareLess(right, left)
		eq := valuesEqual(left, right)
		return l.(bool) || eq.(bool)
	case "&&":
		return isTruthy(left) && isTruthy(right)
	case "||":
		return isTruthy(left) || isTruthy(right)
	}
	return &NullValue{}
}

func (interp *Interpreter) evalCall(e *CallExpression, env *Environment) Value {
	fn := interp.evalExpr(e.Function, env)
	args := make([]Value, len(e.Arguments))
	for i, a := range e.Arguments {
		args[i] = interp.evalExpr(a, env)
	}

	switch f := fn.(type) {
	case *BuiltinFunction:
		return f.Fn(args...)
	case *EnvBuiltinFunction:
		return f.Fn(env, args...)
	case *FunctionValue:
		fnEnv := NewFunctionEnvironment(f.Env)
		for i, param := range f.Params {
			if i < len(args) {
				fnEnv.Set(param, args[i])
			} else {
				fnEnv.Set(param, &NullValue{})
			}
		}
		result := interp.execStatements(f.Body.Statements, fnEnv)
		if rv, ok := result.(*ReturnValue); ok {
			if len(rv.Values) == 1 {
				return rv.Values[0]
			}
			return rv // multi-return stays as ReturnValue for destructuring
		}
		if hrv, ok := result.(*HTTPReturnValue); ok {
			return hrv
		}
		return &NullValue{}
	default:
		return &NullValue{}
	}
}

func (interp *Interpreter) evalDot(e *DotExpression, env *Environment) Value {
	left := interp.evalExpr(e.Left, env)

	switch v := left.(type) {
	case map[string]Value:
		if val, ok := v[e.Field]; ok {
			return val
		}
		return &NullValue{}
	default:
		return &NullValue{}
	}
}

func (interp *Interpreter) evalIndex(left, idx Value) Value {
	switch v := left.(type) {
	case []Value:
		if i, ok := idx.(int64); ok {
			if i >= 0 && int(i) < len(v) {
				return v[i]
			}
		}
		return &NullValue{}
	case map[string]Value:
		key := valueToString(idx)
		if val, ok := v[key]; ok {
			return val
		}
		return &NullValue{}
	case string:
		if i, ok := idx.(int64); ok {
			if i >= 0 && int(i) < len(v) {
				return string(v[i])
			}
		}
		return &NullValue{}
	default:
		return &NullValue{}
	}
}

// --- Helpers ---

func isTruthy(v Value) bool {
	switch val := v.(type) {
	case bool:
		return val
	case int64:
		return val != 0
	case float64:
		return val != 0
	case string:
		return val != ""
	case *NullValue:
		return false
	case []Value:
		return len(val) > 0
	case map[string]Value:
		return len(val) > 0
	default:
		return true
	}
}

func valueToString(v Value) string {
	switch val := v.(type) {
	case string:
		return val
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		if val {
			return "true"
		}
		return "false"
	case *NullValue:
		return "null"
	case []Value:
		data := valueToGo(v)
		b, _ := json.Marshal(data)
		return string(b)
	case map[string]Value:
		data := valueToGo(v)
		b, _ := json.Marshal(data)
		return string(b)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func valueToGo(v Value) interface{} {
	switch val := v.(type) {
	case string:
		return val
	case int64:
		return val
	case float64:
		return val
	case bool:
		return val
	case *NullValue:
		return nil
	case []Value:
		result := make([]interface{}, len(val))
		for i, item := range val {
			result[i] = valueToGo(item)
		}
		return result
	case map[string]Value:
		result := make(map[string]interface{})
		for k, item := range val {
			result[k] = valueToGo(item)
		}
		return result
	default:
		return nil
	}
}

func goToValue(v interface{}) Value {
	switch val := v.(type) {
	case float64:
		// JSON numbers come as float64
		if val == float64(int64(val)) {
			return int64(val)
		}
		return val
	case string:
		return val
	case bool:
		return val
	case nil:
		return &NullValue{}
	case []interface{}:
		result := make([]Value, len(val))
		for i, item := range val {
			result[i] = goToValue(item)
		}
		return result
	case map[string]interface{}:
		result := make(map[string]Value)
		for k, item := range val {
			result[k] = goToValue(item)
		}
		return result
	default:
		return &NullValue{}
	}
}

func valuesEqual(a, b Value) Value {
	switch av := a.(type) {
	case int64:
		switch bv := b.(type) {
		case int64:
			return av == bv
		case float64:
			return float64(av) == bv
		}
	case float64:
		switch bv := b.(type) {
		case float64:
			return av == bv
		case int64:
			return av == float64(bv)
		}
	case string:
		if bv, ok := b.(string); ok {
			return av == bv
		}
	case bool:
		if bv, ok := b.(bool); ok {
			return av == bv
		}
	case *NullValue:
		if _, ok := b.(*NullValue); ok {
			return true
		}
	}
	return false
}

func compareLess(a, b Value) Value {
	switch av := a.(type) {
	case int64:
		switch bv := b.(type) {
		case int64:
			return av < bv
		case float64:
			return float64(av) < bv
		}
	case float64:
		switch bv := b.(type) {
		case float64:
			return av < bv
		case int64:
			return av < float64(bv)
		}
	case string:
		if bv, ok := b.(string); ok {
			return av < bv
		}
	}
	return false
}

func addValues(a, b Value) Value {
	switch av := a.(type) {
	case int64:
		switch bv := b.(type) {
		case int64:
			return av + bv
		case float64:
			return float64(av) + bv
		}
	case float64:
		switch bv := b.(type) {
		case float64:
			return av + bv
		case int64:
			return av + float64(bv)
		}
	case string:
		return av + valueToString(b)
	}
	return valueToString(a) + valueToString(b)
}

func subtractValues(a, b Value) Value {
	switch av := a.(type) {
	case int64:
		switch bv := b.(type) {
		case int64:
			return av - bv
		case float64:
			return float64(av) - bv
		}
	case float64:
		switch bv := b.(type) {
		case float64:
			return av - bv
		case int64:
			return av - float64(bv)
		}
	}
	return int64(0)
}

func multiplyValues(a, b Value) Value {
	switch av := a.(type) {
	case int64:
		switch bv := b.(type) {
		case int64:
			return av * bv
		case float64:
			return float64(av) * bv
		}
	case float64:
		switch bv := b.(type) {
		case float64:
			return av * bv
		case int64:
			return av * float64(bv)
		}
	}
	return int64(0)
}

func divideValues(a, b Value) Value {
	switch av := a.(type) {
	case int64:
		switch bv := b.(type) {
		case int64:
			if bv == 0 {
				return int64(0)
			}
			return av / bv
		case float64:
			if bv == 0 {
				return float64(0)
			}
			return float64(av) / bv
		}
	case float64:
		switch bv := b.(type) {
		case float64:
			if bv == 0 {
				return float64(0)
			}
			return av / bv
		case int64:
			if bv == 0 {
				return float64(0)
			}
			return av / float64(bv)
		}
	}
	return int64(0)
}

func modValues(a, b Value) Value {
	switch av := a.(type) {
	case int64:
		if bv, ok := b.(int64); ok && bv != 0 {
			return av % bv
		}
	}
	return int64(0)
}
