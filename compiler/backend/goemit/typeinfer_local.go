package goemit

func inferExprType(expr Expression, env *TypeEnv) TypeInfo {
	if expr == nil {
		return TypeUnknown
	}
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
			if t == TypeInt {
				return TypeInt
			}
			if t == TypeFloat {
				return TypeFloat
			}
			return TypeDynamic
		case "!":
			return TypeBool
		}
	case *InfixExpression:
		lt := inferExprType(e.Left, env)
		rt := inferExprType(e.Right, env)
		switch e.Operator {
		case "+":
			if lt == TypeInt && rt == TypeInt {
				return TypeInt
			}
			if (lt == TypeFloat || lt == TypeInt) && (rt == TypeFloat || rt == TypeInt) {
				return TypeFloat
			}
			if lt == TypeString || rt == TypeString {
				return TypeString
			}
			return TypeDynamic
		case "-", "*":
			if lt == TypeInt && rt == TypeInt {
				return TypeInt
			}
			if (lt == TypeFloat || lt == TypeInt) && (rt == TypeFloat || rt == TypeInt) {
				return TypeFloat
			}
			return TypeDynamic
		case "/":
			if lt == TypeInt && rt == TypeInt {
				return TypeInt
			}
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
			case "int":
				return TypeInt
			case "float":
				return TypeFloat
			case "str":
				return TypeString
			case "bool":
				return TypeBool
			case "len":
				return TypeInt
			case "type":
				return TypeString
			case "append":
				return TypeArray
			case "keys", "values", "split", "filter", "map", "sort", "reverse", "unique", "flat", "slice":
				if ident.Value == "slice" {
					if len(e.Arguments) > 0 {
						at := inferExprType(e.Arguments[0], env)
						if at == TypeString {
							return TypeString
						}
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
				if len(e.Arguments) >= 2 {
					at := inferExprType(e.Arguments[0], env)
					bt := inferExprType(e.Arguments[1], env)
					if at == TypeInt && bt == TypeInt {
						return TypeInt
					}
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
		if ident, ok := e.Left.(*Identifier); ok {
			if ident.Value == "params" {
				return TypeString
			}
			if ident.Value == "request" {
				switch e.Field {
				case "body", "method", "path":
					return TypeString
				case "query", "headers":
					return TypeMap
				}
			}
		}
		return TypeDynamic
	case *IndexExpression:
		return TypeDynamic
	}
	return TypeDynamic
}
