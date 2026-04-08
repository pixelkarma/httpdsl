package goemit

import (
	"fmt"
	"strconv"
	"strings"
)

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
	allTyped := tenv != nil && tenv.RetType().IsTyped()
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
	retType := tenv.RetType().String()
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
	switch tenv.RetType() {
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
	c.emitSwitch(s, false)
}

// typedExpr returns a Go expression with concrete types (no Value)
func (c *NativeCompiler) typedExpr(e Expression) string {
	if e == nil {
		return "0"
	}
	switch ex := e.(type) {
	case *IntegerLiteral:
		return fmt.Sprintf("int64(%d)", ex.Value)
	case *FloatLiteral:
		return fmt.Sprintf("float64(%s)", strconv.FormatFloat(ex.Value, 'f', -1, 64))
	case *StringLiteral:
		return fmt.Sprintf("%q", ex.Value)
	case *BooleanLiteral:
		if ex.Value {
			return "true"
		}
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
		case "+":
			return fmt.Sprintf("(%s + %s)", l, r)
		case "-":
			return fmt.Sprintf("(%s - %s)", l, r)
		case "*":
			return fmt.Sprintf("(%s * %s)", l, r)
		case "/":
			return fmt.Sprintf("(%s / %s)", l, r)
		case "%%":
			return fmt.Sprintf("(%s %% %s)", l, r)
		case "==":
			return fmt.Sprintf("(%s == %s)", l, r)
		case "!=":
			return fmt.Sprintf("(%s != %s)", l, r)
		case "<":
			return fmt.Sprintf("(%s < %s)", l, r)
		case ">":
			return fmt.Sprintf("(%s > %s)", l, r)
		case "<=":
			return fmt.Sprintf("(%s <= %s)", l, r)
		case ">=":
			return fmt.Sprintf("(%s >= %s)", l, r)
		case "&&":
			return fmt.Sprintf("(%s && %s)", l, r)
		case "||":
			return fmt.Sprintf("(%s || %s)", l, r)
		}
	case *PrefixExpression:
		switch ex.Operator {
		case "-":
			return fmt.Sprintf("(-%s)", c.typedExpr(ex.Right))
		case "!":
			return fmt.Sprintf("(!%s)", c.typedExpr(ex.Right))
		}
	case *CallExpression:
		if ident, ok := ex.Function.(*Identifier); ok {
			// Check if calling a typed function
			if tenv, ok := c.fnTypes[ident.Value]; ok && tenv.RetType().IsTyped() {
				allParamsTyped := true
				for _, fn := range c.functions {
					if fn.Name == ident.Value {
						for _, p := range fn.Params {
							if !tenv.Get(p).IsTyped() {
								allParamsTyped = false
							}
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
	// Map DSL 'args' to the built-in _argsMap.
	if name == "args" {
		return "_argsMap"
	}
	// Avoid Go keywords.
	switch name {
	case "type", "map", "func", "var", "range", "select", "case", "default", "chan", "go", "defer", "interface", "struct", "package", "import", "return", "break", "continue", "for", "if", "else", "switch":
		return "_" + name
	}
	return strings.ReplaceAll(name, "-", "_")
}
