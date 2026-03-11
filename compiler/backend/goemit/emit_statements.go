package goemit

import (
	"fmt"
	"strings"
)

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
		} else if c.isPushCall(s.Expression) {
			c.emitPushStmt(s.Expression)
		} else {
			c.lnf("_ = %s", c.expr(s.Expression))
		}
	case *ReturnStatement:
		if c.inSSERoute && isRoute {
			// SSE routes have no response object — bare return exits the handler
			c.ln("return")
		} else if isRoute && len(s.Values) == 0 {
			// In route context, return exits the route closure.
			// Response is written once by the outer handler.
			c.ln("return")
		} else if isRoute && len(s.Values) == 1 {
			// return response (or return controllerFn(request, response))
			c.lnf("response = %s", c.expr(s.Values[0]))
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

func (c *NativeCompiler) isPushCall(e Expression) bool {
	call, ok := e.(*CallExpression)
	if !ok {
		return false
	}
	ident, ok := call.Function.(*Identifier)
	return ok && ident.Value == "push" && len(call.Arguments) >= 2
}

func (c *NativeCompiler) emitPushStmt(e Expression) {
	call := e.(*CallExpression)
	// push(arr, item) → arr = append(arr, item)
	arrExpr := c.expr(call.Arguments[0])
	itemExprs := make([]string, len(call.Arguments)-1)
	for i := 1; i < len(call.Arguments); i++ {
		itemExprs[i-1] = c.expr(call.Arguments[i])
	}
	c.lnf("%s = builtin_append(%s, %s)", arrExpr, arrExpr, strings.Join(itemExprs, ", "))
}

func (c *NativeCompiler) isRenderCall(e Expression) bool {
	call, ok := e.(*CallExpression)
	if !ok {
		return false
	}
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
		if i > 0 {
			kw = "} else if"
		}
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
