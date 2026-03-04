package compiler

import (
	"encoding/json"
	"fmt"
	"httpdsl/runtime"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
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

// Environment (scope)
type Environment struct {
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
	val, ok := e.store[name]
	if !ok && e.outer != nil {
		return e.outer.Get(name)
	}
	return val, ok
}

func (e *Environment) Set(name string, val Value) {
	e.store[name] = val
}

// SetExisting sets in the scope where the variable already exists,
// but won't cross function boundaries (boundary == true)
func (e *Environment) SetExisting(name string, val Value) {
	if _, ok := e.store[name]; ok {
		e.store[name] = val
		return
	}
	if e.outer != nil && !e.boundary {
		e.outer.SetExisting(name, val)
		return
	}
	// Create in current scope
	e.store[name] = val
}

// Interpreter
type Interpreter struct {
	global *Environment
	Server *runtime.Server
}

func NewInterpreter() *Interpreter {
	i := &Interpreter{
		global: NewEnvironment(),
		Server: runtime.NewServer(),
	}
	i.registerBuiltins()
	return i
}

func (interp *Interpreter) registerBuiltins() {
	interp.global.Set("print", &BuiltinFunction{Fn: func(args ...Value) Value {
		parts := make([]string, len(args))
		for i, a := range args {
			parts[i] = valueToString(a)
		}
		fmt.Println(strings.Join(parts, " "))
		return &NullValue{}
	}})

	interp.global.Set("len", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) != 1 {
			return int64(0)
		}
		switch v := args[0].(type) {
		case string:
			return int64(len(v))
		case []Value:
			return int64(len(v))
		case map[string]Value:
			return int64(len(v))
		default:
			return int64(0)
		}
	}})

	interp.global.Set("str", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) != 1 {
			return ""
		}
		return valueToString(args[0])
	}})

	interp.global.Set("int", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) != 1 {
			return int64(0)
		}
		switch v := args[0].(type) {
		case int64:
			return v
		case float64:
			return int64(v)
		case string:
			n, _ := strconv.ParseInt(v, 10, 64)
			return n
		case bool:
			if v {
				return int64(1)
			}
			return int64(0)
		default:
			return int64(0)
		}
	}})

	interp.global.Set("float", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) != 1 {
			return float64(0)
		}
		switch v := args[0].(type) {
		case int64:
			return float64(v)
		case float64:
			return v
		case string:
			n, _ := strconv.ParseFloat(v, 64)
			return n
		default:
			return float64(0)
		}
	}})

	interp.global.Set("env", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) != 1 {
			return ""
		}
		if name, ok := args[0].(string); ok {
			return os.Getenv(name)
		}
		return ""
	}})

	interp.global.Set("keys", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) != 1 {
			return []Value{}
		}
		if m, ok := args[0].(map[string]Value); ok {
			result := make([]Value, 0, len(m))
			for k := range m {
				result = append(result, k)
			}
			return result
		}
		return []Value{}
	}})

	interp.global.Set("values", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) != 1 {
			return []Value{}
		}
		if m, ok := args[0].(map[string]Value); ok {
			result := make([]Value, 0, len(m))
			for _, v := range m {
				result = append(result, v)
			}
			return result
		}
		return []Value{}
	}})

	interp.global.Set("append", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) < 2 {
			return args[0]
		}
		if arr, ok := args[0].([]Value); ok {
			return append(arr, args[1:]...)
		}
		return args[0]
	}})

	interp.global.Set("type", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) != 1 {
			return "null"
		}
		switch args[0].(type) {
		case int64:
			return "int"
		case float64:
			return "float"
		case string:
			return "string"
		case bool:
			return "bool"
		case []Value:
			return "array"
		case map[string]Value:
			return "object"
		case *FunctionValue:
			return "function"
		case *NullValue:
			return "null"
		default:
			return "unknown"
		}
	}})

	interp.global.Set("contains", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) != 2 {
			return false
		}
		s, ok1 := args[0].(string)
		sub, ok2 := args[1].(string)
		if !ok1 || !ok2 {
			return false
		}
		return strings.Contains(s, sub)
	}})

	interp.global.Set("trim", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) != 1 {
			return ""
		}
		if s, ok := args[0].(string); ok {
			return strings.TrimSpace(s)
		}
		return ""
	}})

	interp.global.Set("split", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) != 2 {
			return []Value{}
		}
		s, ok1 := args[0].(string)
		sep, ok2 := args[1].(string)
		if !ok1 || !ok2 {
			return []Value{}
		}
		parts := strings.Split(s, sep)
		result := make([]Value, len(parts))
		for i, p := range parts {
			result[i] = p
		}
		return result
	}})

	interp.global.Set("upper", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) != 1 {
			return ""
		}
		if s, ok := args[0].(string); ok {
			return strings.ToUpper(s)
		}
		return ""
	}})

	interp.global.Set("lower", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) != 1 {
			return ""
		}
		if s, ok := args[0].(string); ok {
			return strings.ToLower(s)
		}
		return ""
	}})

	interp.global.Set("replace", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) != 3 {
			return ""
		}
		s, ok1 := args[0].(string)
		old, ok2 := args[1].(string)
		new_, ok3 := args[2].(string)
		if !ok1 || !ok2 || !ok3 {
			return ""
		}
		return strings.ReplaceAll(s, old, new_)
	}})

	interp.global.Set("starts_with", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) != 2 {
			return false
		}
		s, ok1 := args[0].(string)
		prefix, ok2 := args[1].(string)
		if !ok1 || !ok2 {
			return false
		}
		return strings.HasPrefix(s, prefix)
	}})

	interp.global.Set("ends_with", &BuiltinFunction{Fn: func(args ...Value) Value {
		if len(args) != 2 {
			return false
		}
		s, ok1 := args[0].(string)
		suffix, ok2 := args[1].(string)
		if !ok1 || !ok2 {
			return false
		}
		return strings.HasSuffix(s, suffix)
	}})

	// json namespace as an object with parse and stringify
	jsonObj := map[string]Value{
		"parse": &BuiltinFunction{Fn: func(args ...Value) Value {
			if len(args) != 1 {
				return &NullValue{}
			}
			s, ok := args[0].(string)
			if !ok {
				return &NullValue{}
			}
			var raw interface{}
			if err := json.Unmarshal([]byte(s), &raw); err != nil {
				return &NullValue{}
			}
			return goToValue(raw)
		}},
		"stringify": &BuiltinFunction{Fn: func(args ...Value) Value {
			if len(args) != 1 {
				return ""
			}
			data := valueToGo(args[0])
			b, _ := json.Marshal(data)
			return string(b)
		}},
	}
	interp.global.Set("json", jsonObj)
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
			// Regular return in route context — return first value as JSON
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
