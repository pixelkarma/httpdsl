package compiler

// TypeInfo represents the inferred type of a variable
type TypeInfo int

const (
	TypeUnknown TypeInfo = iota // could be anything → use Value
	TypeInt                     // always int64
	TypeFloat                   // always float64
	TypeString                  // always string
	TypeBool                    // always bool
	TypeArray                   // always []Value
	TypeMap                     // always map[string]Value
	TypeDynamic                 // mixed types seen → must use Value
)

func (t TypeInfo) String() string {
	switch t {
	case TypeInt:     return "int64"
	case TypeFloat:   return "float64"
	case TypeString:  return "string"
	case TypeBool:    return "bool"
	default:          return "Value"
	}
}

// IsTyped returns true if the type is a concrete known type
func (t TypeInfo) IsTyped() bool {
	return t == TypeInt || t == TypeFloat || t == TypeString || t == TypeBool
}

// TypeEnv holds inferred types for variables in a scope
type TypeEnv struct {
	vars   map[string]TypeInfo
	params map[string]TypeInfo // function parameters
	retType TypeInfo           // return type
}

func NewTypeEnv() *TypeEnv {
	return &TypeEnv{
		vars:   make(map[string]TypeInfo),
		params: make(map[string]TypeInfo),
	}
}

// Set updates a variable's type. If it was previously a different concrete type, widen to Dynamic.
func (e *TypeEnv) Set(name string, t TypeInfo) {
	if t == TypeUnknown || t == TypeDynamic {
		e.vars[name] = TypeDynamic
		return
	}
	prev, exists := e.vars[name]
	if !exists || prev == TypeUnknown {
		e.vars[name] = t
	} else if prev != t {
		e.vars[name] = TypeDynamic // conflicting types
	}
}

func (e *TypeEnv) Get(name string) TypeInfo {
	if t, ok := e.vars[name]; ok {
		return t
	}
	if t, ok := e.params[name]; ok {
		return t
	}
	return TypeUnknown
}

func (e *TypeEnv) RetType() TypeInfo { return e.retType }
func (e *TypeEnv) Vars() map[string]TypeInfo { return e.vars }

// inferParamType determines a parameter's type from how it's used in the body.
// If it's only compared with ints and used in int arithmetic, it's int64.
func inferParamType(name string, block *BlockStatement, env *TypeEnv) TypeInfo {
	uses := collectParamUses(name, block)
	if len(uses) == 0 {
		return TypeUnknown
	}
	result := TypeUnknown
	for _, u := range uses {
		if result == TypeUnknown {
			result = u
		} else if result != u {
			return TypeDynamic
		}
	}
	return result
}

// collectParamUses finds all type contexts where a parameter is used
func collectParamUses(name string, block *BlockStatement) []TypeInfo {
	var uses []TypeInfo
	for _, stmt := range block.Statements {
		uses = append(uses, collectParamUsesStmt(name, stmt)...)
	}
	return uses
}

func collectParamUsesStmt(name string, stmt Statement) []TypeInfo {
	var uses []TypeInfo
	switch s := stmt.(type) {
	case *AssignStatement:
		for _, v := range s.Values {
			uses = append(uses, collectParamUsesExpr(name, v)...)
		}
	case *CompoundAssignStatement:
		uses = append(uses, collectParamUsesExpr(name, s.Value)...)
	case *ReturnStatement:
		for _, v := range s.Values {
			uses = append(uses, collectParamUsesExpr(name, v)...)
		}
	case *IfStatement:
		uses = append(uses, collectParamUsesExpr(name, s.Condition)...)
		uses = append(uses, collectParamUses(name, s.Consequence)...)
		if s.Alternative != nil {
			switch alt := s.Alternative.(type) {
			case *BlockStatement:
				uses = append(uses, collectParamUses(name, alt)...)
			case *IfStatement:
				uses = append(uses, collectParamUsesStmt(name, alt)...)
			}
		}
	case *WhileStatement:
		uses = append(uses, collectParamUsesExpr(name, s.Condition)...)
		uses = append(uses, collectParamUses(name, s.Body)...)
	case *ExpressionStatement:
		uses = append(uses, collectParamUsesExpr(name, s.Expression)...)
	}
	return uses
}

func collectParamUsesExpr(name string, expr Expression) []TypeInfo {
	if expr == nil { return nil }
	switch e := expr.(type) {
	case *InfixExpression:
		// If param is on one side of a comparison/arithmetic with a known type
		lHasParam := exprReferences(name, e.Left)
		rHasParam := exprReferences(name, e.Right)
		var uses []TypeInfo
		switch e.Operator {
		case "+", "-", "*", "/", "%", "<", ">", "<=", ">=", "==", "!=":
			if lHasParam {
				// The other side tells us the type
				if _, ok := e.Right.(*IntegerLiteral); ok {
					uses = append(uses, TypeInt)
				} else if _, ok := e.Right.(*FloatLiteral); ok {
					uses = append(uses, TypeFloat)
				} else if _, ok := e.Right.(*StringLiteral); ok {
					uses = append(uses, TypeString)
				}
			}
			if rHasParam {
				if _, ok := e.Left.(*IntegerLiteral); ok {
					uses = append(uses, TypeInt)
				} else if _, ok := e.Left.(*FloatLiteral); ok {
					uses = append(uses, TypeFloat)
				} else if _, ok := e.Left.(*StringLiteral); ok {
					uses = append(uses, TypeString)
				}
			}
		}
		uses = append(uses, collectParamUsesExpr(name, e.Left)...)
		uses = append(uses, collectParamUsesExpr(name, e.Right)...)
		return uses
	case *CallExpression:
		var uses []TypeInfo
		for _, a := range e.Arguments {
			uses = append(uses, collectParamUsesExpr(name, a)...)
		}
		return uses
	case *PrefixExpression:
		return collectParamUsesExpr(name, e.Right)
	}
	return nil
}

func exprReferences(name string, expr Expression) bool {
	switch e := expr.(type) {
	case *Identifier:
		return e.Value == name
	case *InfixExpression:
		return exprReferences(name, e.Left) || exprReferences(name, e.Right)
	case *PrefixExpression:
		return exprReferences(name, e.Right)
	case *CallExpression:
		for _, a := range e.Arguments {
			if exprReferences(name, a) { return true }
		}
	}
	return false
}

// InferFunctionTypes analyzes a function body and infers types for all variables.
// It uses a simple forward-pass analysis: walk the AST and propagate types.
// Run multiple passes until stable.
func InferFunctionTypes(params []string, body *BlockStatement, callerEnv *TypeEnv) *TypeEnv {
	env := NewTypeEnv()
	// Parameters start as Unknown unless caller provides info
	for _, p := range params {
		if callerEnv != nil {
			if t, ok := callerEnv.params[p]; ok {
				env.params[p] = t
				continue
			}
		}
		env.params[p] = TypeUnknown
	}

	// Run inference passes until stable (max 5)
	for pass := 0; pass < 5; pass++ {
		prev := copyTypes(env.vars)
		prevParams := copyTypes(env.params)
		inferBlock(body, env)
		// Infer parameter types from usage context
		for _, p := range params {
			if env.params[p] == TypeUnknown {
				env.params[p] = inferParamType(p, body, env)
			}
		}
		if typesEqual(prev, env.vars) && typesEqual(prevParams, env.params) {
			break
		}
	}

	// Infer return type
	env.retType = inferReturnType(body, env)

	return env
}

func copyTypes(m map[string]TypeInfo) map[string]TypeInfo {
	c := make(map[string]TypeInfo, len(m))
	for k, v := range m { c[k] = v }
	return c
}

func typesEqual(a, b map[string]TypeInfo) bool {
	if len(a) != len(b) { return false }
	for k, v := range a {
		if b[k] != v { return false }
	}
	return true
}

func inferBlock(block *BlockStatement, env *TypeEnv) {
	for _, stmt := range block.Statements {
		inferStmt(stmt, env)
	}
}

func inferStmt(stmt Statement, env *TypeEnv) {
	switch s := stmt.(type) {
	case *AssignStatement:
		for i, name := range s.Names {
			if i < len(s.Values) {
				t := inferExprType(s.Values[i], env)
				env.Set(name, t)
			}
		}
	case *CompoundAssignStatement:
		// x += val: result type depends on both sides
		cur := env.Get(s.Name)
		val := inferExprType(s.Value, env)
		if cur == TypeInt && val == TypeInt {
			env.Set(s.Name, TypeInt)
		} else if (cur == TypeFloat || cur == TypeInt) && (val == TypeFloat || val == TypeInt) {
			env.Set(s.Name, TypeFloat)
		} else {
			env.Set(s.Name, TypeDynamic)
		}
	case *IfStatement:
		inferBlock(s.Consequence, env)
		if s.Alternative != nil {
			switch alt := s.Alternative.(type) {
			case *BlockStatement: inferBlock(alt, env)
			case *IfStatement: inferStmt(alt, env)
			}
		}
	case *WhileStatement:
		inferBlock(s.Body, env)
	case *EachStatement:
		// Iterator variable: if iterating over array, element type unknown
		// Index is always int64
		env.Set(s.Value, TypeDynamic)
		if s.Index != "" {
			env.Set(s.Index, TypeInt)
		}
		inferBlock(s.Body, env)
	case *BlockStatement:
		inferBlock(s, env)
	case *ExpressionStatement:
		inferExprType(s.Expression, env)
	}
}

func inferExprType(expr Expression, env *TypeEnv) TypeInfo {
	if expr == nil { return TypeUnknown }
	switch e := expr.(type) {
	case *IntegerLiteral:
		return TypeInt
	case *FloatLiteral:
		return TypeFloat
	case *StringLiteral:
		return TypeString
	case *BooleanLiteral:
		return TypeBool
	case *NullLiteral:
		return TypeDynamic
	case *Identifier:
		return env.Get(e.Value)
	case *ArrayLiteral:
		return TypeArray
	case *HashLiteral:
		return TypeMap
	case *PrefixExpression:
		switch e.Operator {
		case "-":
			t := inferExprType(e.Right, env)
			if t == TypeInt { return TypeInt }
			if t == TypeFloat { return TypeFloat }
			return TypeDynamic
		case "!":
			return TypeBool
		}
	case *InfixExpression:
		lt := inferExprType(e.Left, env)
		rt := inferExprType(e.Right, env)
		switch e.Operator {
		case "+":
			if lt == TypeInt && rt == TypeInt { return TypeInt }
			if (lt == TypeFloat || lt == TypeInt) && (rt == TypeFloat || rt == TypeInt) { return TypeFloat }
			if lt == TypeString || rt == TypeString { return TypeString }
			return TypeDynamic
		case "-", "*":
			if lt == TypeInt && rt == TypeInt { return TypeInt }
			if (lt == TypeFloat || lt == TypeInt) && (rt == TypeFloat || rt == TypeInt) { return TypeFloat }
			return TypeDynamic
		case "/":
			if lt == TypeInt && rt == TypeInt { return TypeInt }
			return TypeFloat
		case "%":
			return TypeInt
		case "==", "!=", "<", ">", "<=", ">=":
			return TypeBool
		case "&&", "||":
			return TypeBool
		}
	case *CallExpression:
		if ident, ok := e.Function.(*Identifier); ok {
			switch ident.Value {
			case "int": return TypeInt
			case "float": return TypeFloat
			case "str": return TypeString
			case "bool": return TypeBool
			case "len": return TypeInt
			case "type": return TypeString
			case "append": return TypeArray
			case "keys", "values", "split", "filter", "map", "sort", "reverse", "unique", "flat", "slice":
				// slice could return string or array, but usually array
				if ident.Value == "slice" {
					if len(e.Arguments) > 0 {
						at := inferExprType(e.Arguments[0], env)
						if at == TypeString { return TypeString }
					}
					return TypeArray
				}
				return TypeArray
			case "join", "upper", "lower", "replace", "trim", "repeat":
				return TypeString
			case "contains", "has", "includes", "starts_with", "ends_with":
				return TypeBool
			case "index_of":
				return TypeInt
			case "abs", "ceil", "floor", "round":
				return TypeFloat
			case "min", "max", "clamp":
				// Could be int or float
				if len(e.Arguments) >= 2 {
					at := inferExprType(e.Arguments[0], env)
					bt := inferExprType(e.Arguments[1], env)
					if at == TypeInt && bt == TypeInt { return TypeInt }
				}
				return TypeDynamic
			case "random":
				return TypeFloat
			case "random_int":
				return TypeInt
			case "now", "now_ms":
				return TypeInt
			}
		}
		return TypeDynamic
	case *DotExpression:
		// params.x is always string, request.method is string, etc.
		if ident, ok := e.Left.(*Identifier); ok {
			if ident.Value == "params" { return TypeString }
			if ident.Value == "request" {
				switch e.Field {
				case "body", "method", "path": return TypeString
				case "query", "headers": return TypeMap
				}
			}
		}
		return TypeDynamic
	case *IndexExpression:
		return TypeDynamic
	}
	return TypeDynamic
}

func inferReturnType(block *BlockStatement, env *TypeEnv) TypeInfo {
	retType := TypeUnknown
	for _, stmt := range block.Statements {
		t := stmtReturnType(stmt, env)
		if t != TypeUnknown {
			if retType == TypeUnknown {
				retType = t
			} else if retType != t {
				return TypeDynamic
			}
		}
	}
	return retType
}

func stmtReturnType(stmt Statement, env *TypeEnv) TypeInfo {
	switch s := stmt.(type) {
	case *ReturnStatement:
		if len(s.Values) == 1 {
			return inferExprType(s.Values[0], env)
		}
		return TypeDynamic // multiple return values
	case *IfStatement:
		ct := inferReturnType(s.Consequence, env)
		if s.Alternative != nil {
			var at TypeInfo
			switch alt := s.Alternative.(type) {
			case *BlockStatement: at = inferReturnType(alt, env)
			case *IfStatement: at = stmtReturnType(alt, env)
			}
			if ct != TypeUnknown && at != TypeUnknown && ct == at {
				return ct
			}
			if ct != TypeUnknown && at != TypeUnknown {
				return TypeDynamic
			}
		}
		return ct
	case *WhileStatement:
		return inferReturnType(s.Body, env)
	case *BlockStatement:
		return inferReturnType(s, env)
	}
	return TypeUnknown
}
