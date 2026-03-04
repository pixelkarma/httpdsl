package compiler

// SymbolScope represents where a variable lives
type SymbolScope int

const (
	ScopeLocal SymbolScope = iota
	ScopeGlobal
)

type Symbol struct {
	Name  string
	Scope SymbolScope
	Index int
}

type SymbolTable struct {
	store map[string]*Symbol
	count int
}

func NewSymbolTable() *SymbolTable {
	return &SymbolTable{store: make(map[string]*Symbol)}
}

func (s *SymbolTable) Define(name string) *Symbol {
	if sym, ok := s.store[name]; ok {
		return sym
	}
	sym := &Symbol{Name: name, Scope: ScopeLocal, Index: s.count}
	s.store[name] = sym
	s.count++
	return sym
}

func (s *SymbolTable) Resolve(name string) (*Symbol, bool) {
	sym, ok := s.store[name]
	return sym, ok
}

// BytecodeCompiler compiles AST to bytecode
type BytecodeCompiler struct {
	builder    *CodeBuilder
	symbols    *SymbolTable
	loopStarts []int   // stack of loop start positions
	loopBreaks [][]int // stack of break positions to patch
}

func NewBytecodeCompiler() *BytecodeCompiler {
	return &BytecodeCompiler{
		builder: NewCodeBuilder(),
		symbols: NewSymbolTable(),
	}
}

// CompileFunction compiles a function body to bytecode
func CompileFunction(params []string, body *BlockStatement) *CompiledFunction {
	c := NewBytecodeCompiler()
	// Define parameters as local slots
	for _, p := range params {
		c.symbols.Define(p)
	}
	c.compileBlock(body)
	// Ensure we always return
	c.builder.Emit(OP_NULL)
	c.builder.EmitU8(OP_RETURN, 1)

	return &CompiledFunction{
		Code:      c.builder.code,
		Constants: c.builder.constants,
		Names:     c.builder.names,
		NumLocals: c.symbols.count,
		NumParams: len(params),
	}
}

// CompileRoute compiles a route body (params and request are pre-defined locals)
func CompileRoute(pathParams []string, body *BlockStatement) *CompiledFunction {
	c := NewBytecodeCompiler()
	// slot 0 = params, slot 1 = request
	c.symbols.Define("params")
	c.symbols.Define("request")
	c.compileBlock(body)
	// Default: return null
	c.builder.Emit(OP_NULL)
	c.builder.EmitU8(OP_RETURN, 1)

	return &CompiledFunction{
		Code:      c.builder.code,
		Constants: c.builder.constants,
		Names:     c.builder.names,
		NumLocals: c.symbols.count,
		NumParams: 2, // params, request
	}
}

func (c *BytecodeCompiler) compileBlock(block *BlockStatement) {
	for _, stmt := range block.Statements {
		c.compileStatement(stmt)
	}
}

func (c *BytecodeCompiler) compileStatement(stmt Statement) {
	switch s := stmt.(type) {
	case *AssignStatement:
		c.compileAssign(s)
	case *CompoundAssignStatement:
		c.compileCompoundAssign(s)
	case *IndexAssignStatement:
		c.compileIndexAssign(s)
	case *ExpressionStatement:
		c.compileExpr(s.Expression)
		c.builder.Emit(OP_POP)
	case *ReturnStatement:
		for _, v := range s.Values {
			c.compileExpr(v)
		}
		c.builder.EmitU8(OP_RETURN, byte(len(s.Values)))
	case *HTTPReturnStatement:
		c.compileHTTPReturn(s)
	case *IfStatement:
		c.compileIf(s)
	case *WhileStatement:
		c.compileWhile(s)
	case *EachStatement:
		c.compileEach(s)
	case *BreakStatement:
		pos := c.builder.EmitJump(OP_JMP)
		if len(c.loopBreaks) > 0 {
			c.loopBreaks[len(c.loopBreaks)-1] = append(c.loopBreaks[len(c.loopBreaks)-1], pos)
		}
	case *ContinueStatement:
		if len(c.loopStarts) > 0 {
			c.builder.EmitU16(OP_JMP, c.loopStarts[len(c.loopStarts)-1])
		}
	case *BlockStatement:
		c.compileBlock(s)
	case *FnStatement:
		// Compile nested function
		fn := CompileFunction(s.Params, s.Body)
		idx := c.builder.AddConstant(fn)
		c.builder.EmitU16(OP_CONST, idx)
		sym := c.symbols.Define(s.Name)
		c.builder.EmitU8(OP_SET_LOCAL, byte(sym.Index))
	}
}

func (c *BytecodeCompiler) compileAssign(s *AssignStatement) {
	// Check for optimized append pattern: x = append(x, val)
	if len(s.Names) == 1 && len(s.Values) == 1 {
		if call, ok := s.Values[0].(*CallExpression); ok {
			if fnIdent, ok := call.Function.(*Identifier); ok && fnIdent.Value == "append" && len(call.Arguments) >= 2 {
				if argIdent, ok := call.Arguments[0].(*Identifier); ok && argIdent.Value == s.Names[0] {
					// Emit: GET_LOCAL arr, compile val, APPEND, SET_LOCAL arr
					c.compileIdent(argIdent.Value)
					for _, arg := range call.Arguments[1:] {
						c.compileExpr(arg)
						c.builder.Emit(OP_APPEND)
					}
					sym := c.symbols.Define(s.Names[0])
					c.builder.EmitU8(OP_SET_LOCAL, byte(sym.Index))
					return
				}
			}
		}
	}

	if len(s.Names) == 1 {
		c.compileExpr(s.Values[0])
		sym := c.symbols.Define(s.Names[0])
		c.builder.EmitU8(OP_SET_LOCAL, byte(sym.Index))
	} else {
		// Multi-assign: compile all values, then store in reverse
		for _, v := range s.Values {
			c.compileExpr(v)
		}
		for i := len(s.Names) - 1; i >= 0; i-- {
			sym := c.symbols.Define(s.Names[i])
			c.builder.EmitU8(OP_SET_LOCAL, byte(sym.Index))
		}
	}
}

func (c *BytecodeCompiler) compileCompoundAssign(s *CompoundAssignStatement) {
	sym, ok := c.symbols.Resolve(s.Name)
	if !ok {
		sym = c.symbols.Define(s.Name)
	}
	// Optimized path for local int increment
	if s.Operator == "+=" {
		if sym.Scope == ScopeLocal {
			c.compileExpr(s.Value)
			c.builder.EmitU8(OP_INC_LOCAL, byte(sym.Index))
			return
		}
	}
	if s.Operator == "-=" {
		if sym.Scope == ScopeLocal {
			c.compileExpr(s.Value)
			c.builder.EmitU8(OP_DEC_LOCAL, byte(sym.Index))
			return
		}
	}
	// General path
	c.compileIdent(s.Name)
	c.compileExpr(s.Value)
	if s.Operator == "+=" {
		c.builder.Emit(OP_ADD)
	} else {
		c.builder.Emit(OP_SUB)
	}
	c.builder.EmitU8(OP_SET_LOCAL, byte(sym.Index))
}

func (c *BytecodeCompiler) compileIndexAssign(s *IndexAssignStatement) {
	c.compileExpr(s.Left)
	c.compileExpr(s.Index)
	c.compileExpr(s.Value)
	c.builder.Emit(OP_SET_INDEX)
}

func (c *BytecodeCompiler) compileHTTPReturn(s *HTTPReturnStatement) {
	// Push: body, status code, response type string
	c.compileExpr(s.Body)
	idx := c.builder.AddConstant(int64(s.StatusCode))
	c.builder.EmitU16(OP_CONST, idx)
	typeIdx := c.builder.AddConstant(s.ResponseType)
	c.builder.EmitU16(OP_CONST, typeIdx)
	c.builder.Emit(OP_RETURN_HTTP)
}

func (c *BytecodeCompiler) compileIf(s *IfStatement) {
	c.compileExpr(s.Condition)
	falseJump := c.builder.EmitJump(OP_JMP_FALSE)
	c.compileBlock(s.Consequence)

	if s.Alternative != nil {
		endJump := c.builder.EmitJump(OP_JMP)
		c.builder.PatchJump(falseJump)
		switch alt := s.Alternative.(type) {
		case *BlockStatement:
			c.compileBlock(alt)
		case *IfStatement:
			c.compileIf(alt)
		}
		c.builder.PatchJump(endJump)
	} else {
		c.builder.PatchJump(falseJump)
	}
}

func (c *BytecodeCompiler) compileWhile(s *WhileStatement) {
	loopStart := c.builder.CurrentPos()
	c.loopStarts = append(c.loopStarts, loopStart)
	c.loopBreaks = append(c.loopBreaks, nil)

	c.compileExpr(s.Condition)
	exitJump := c.builder.EmitJump(OP_JMP_FALSE)
	c.compileBlock(s.Body)
	c.builder.EmitU16(OP_JMP, loopStart)
	c.builder.PatchJump(exitJump)

	// Patch break statements
	breaks := c.loopBreaks[len(c.loopBreaks)-1]
	for _, pos := range breaks {
		c.builder.PatchJump(pos)
	}
	c.loopStarts = c.loopStarts[:len(c.loopStarts)-1]
	c.loopBreaks = c.loopBreaks[:len(c.loopBreaks)-1]
}

func (c *BytecodeCompiler) compileEach(s *EachStatement) {
	c.compileExpr(s.Iterable)

	// Define loop variables
	valSym := c.symbols.Define(s.Value)
	var idxSym *Symbol
	if s.Index != "" {
		idxSym = c.symbols.Define(s.Index)
	}

	// Determine if we're iterating array or map based on context
	// We'll use OP_ITER_ARRAY/OP_ITER_MAP decided at runtime
	// For simplicity, emit array iteration (most common) with runtime type check in VM
	c.builder.Emit(OP_ITER_ARRAY)
	loopStart := c.builder.CurrentPos()
	c.loopStarts = append(c.loopStarts, loopStart)
	c.loopBreaks = append(c.loopBreaks, nil)

	// ITER_NEXT pushes: hasMore, index, value
	c.builder.Emit(OP_ITER_NEXT)
	exitJump := c.builder.EmitJump(OP_JMP_FALSE)

	// Store value and index
	c.builder.EmitU8(OP_SET_LOCAL, byte(valSym.Index))
	if idxSym != nil {
		c.builder.EmitU8(OP_SET_LOCAL, byte(idxSym.Index))
	} else {
		c.builder.Emit(OP_POP)
	}

	c.compileBlock(s.Body)
	c.builder.EmitU16(OP_JMP, loopStart)
	c.builder.PatchJump(exitJump)
	// Pop the iterator
	c.builder.Emit(OP_POP)

	breaks := c.loopBreaks[len(c.loopBreaks)-1]
	for _, pos := range breaks {
		c.builder.PatchJump(pos)
	}
	c.loopStarts = c.loopStarts[:len(c.loopStarts)-1]
	c.loopBreaks = c.loopBreaks[:len(c.loopBreaks)-1]
}

func (c *BytecodeCompiler) compileExpr(expr Expression) {
	switch e := expr.(type) {
	case *IntegerLiteral:
		idx := c.builder.AddConstant(e.Value)
		c.builder.EmitU16(OP_CONST, idx)
	case *FloatLiteral:
		idx := c.builder.AddConstant(e.Value)
		c.builder.EmitU16(OP_CONST, idx)
	case *StringLiteral:
		idx := c.builder.AddConstant(e.Value)
		c.builder.EmitU16(OP_CONST, idx)
	case *BooleanLiteral:
		if e.Value {
			c.builder.Emit(OP_TRUE)
		} else {
			c.builder.Emit(OP_FALSE)
		}
	case *NullLiteral:
		c.builder.Emit(OP_NULL)
	case *Identifier:
		c.compileIdent(e.Value)
	case *PrefixExpression:
		c.compileExpr(e.Right)
		switch e.Operator {
		case "-":
			c.builder.Emit(OP_NEG)
		case "!":
			c.builder.Emit(OP_NOT)
		}
	case *InfixExpression:
		c.compileExpr(e.Left)
		c.compileExpr(e.Right)
		switch e.Operator {
		case "+":
			c.builder.Emit(OP_ADD)
		case "-":
			c.builder.Emit(OP_SUB)
		case "*":
			c.builder.Emit(OP_MUL)
		case "/":
			c.builder.Emit(OP_DIV)
		case "%":
			c.builder.Emit(OP_MOD)
		case "==":
			c.builder.Emit(OP_EQ)
		case "!=":
			c.builder.Emit(OP_NEQ)
		case "<":
			c.builder.Emit(OP_LT)
		case ">":
			c.builder.Emit(OP_GT)
		case "<=":
			c.builder.Emit(OP_LTE)
		case ">=":
			c.builder.Emit(OP_GTE)
		case "&&":
			c.builder.Emit(OP_AND)
		case "||":
			c.builder.Emit(OP_OR)
		}
	case *CallExpression:
		c.compileExpr(e.Function)
		for _, arg := range e.Arguments {
			c.compileExpr(arg)
		}
		c.builder.EmitU8(OP_CALL, byte(len(e.Arguments)))
	case *DotExpression:
		c.compileExpr(e.Left)
		nameIdx := c.builder.AddName(e.Field)
		c.builder.EmitU16(OP_DOT, nameIdx)
	case *IndexExpression:
		c.compileExpr(e.Left)
		c.compileExpr(e.Index)
		c.builder.Emit(OP_INDEX)
	case *ArrayLiteral:
		for _, el := range e.Elements {
			c.compileExpr(el)
		}
		c.builder.EmitU16(OP_ARRAY, len(e.Elements))
	case *HashLiteral:
		for _, pair := range e.Pairs {
			c.compileExpr(pair.Key)
			c.compileExpr(pair.Value)
		}
		c.builder.EmitU16(OP_HASH, len(e.Pairs))
	}
}

func (c *BytecodeCompiler) compileIdent(name string) {
	if sym, ok := c.symbols.Resolve(name); ok {
		c.builder.EmitU8(OP_GET_LOCAL, byte(sym.Index))
	} else {
		// Global (builtins, outer functions)
		nameIdx := c.builder.AddName(name)
		c.builder.EmitU16(OP_GET_GLOBAL, nameIdx)
	}
}
