package compiler

// DetectDBDrivers returns which database drivers are used in the program.
func DetectDBDrivers(program *Program) map[string]bool {
	d := &dbDetector{drivers: make(map[string]bool)}
	d.detectProgram(program)
	return d.drivers
}

type dbDetector struct {
	drivers map[string]bool
}

func (d *dbDetector) detectProgram(program *Program) {
	if program == nil {
		return
	}
	for _, stmt := range program.Statements {
		d.detectStmt(stmt)
	}
}

func (d *dbDetector) detectStmt(stmt Statement) {
	if stmt == nil {
		return
	}
	switch s := stmt.(type) {
	case *AssignStatement:
		for _, v := range s.Values {
			d.detectExpr(v)
		}
	case *ExpressionStatement:
		d.detectExpr(s.Expression)
	case *IfStatement:
		d.detectBlock(s.Consequence)
		if alt, ok := s.Alternative.(*BlockStatement); ok {
			d.detectBlock(alt)
		}
		if alt, ok := s.Alternative.(*IfStatement); ok {
			d.detectStmt(alt)
		}
	case *SwitchStatement:
		for _, cs := range s.Cases {
			d.detectBlock(cs.Body)
		}
		if s.Default != nil {
			d.detectBlock(s.Default)
		}
	case *WhileStatement:
		d.detectBlock(s.Body)
	case *EachStatement:
		d.detectBlock(s.Body)
	case *BlockStatement:
		d.detectBlock(s)
	case *RouteStatement:
		d.detectBlock(s.Body)
		if s.ElseBlock != nil {
			d.detectBlock(s.ElseBlock)
		}
	case *GroupStatement:
		for _, r := range s.Routes {
			d.detectStmt(r)
		}
		for _, b := range s.Before {
			d.detectBlock(b)
		}
		for _, a := range s.After {
			d.detectBlock(a)
		}
	case *BeforeStatement:
		d.detectBlock(s.Body)
	case *AfterStatement:
		d.detectBlock(s.Body)
	case *EveryStatement:
		d.detectBlock(s.Body)
	case *InitStatement:
		d.detectBlock(s.Body)
	case *ShutdownStatement:
		d.detectBlock(s.Body)
	case *FnStatement:
		d.detectBlock(s.Body)
	case *TryCatchStatement:
		d.detectBlock(s.Try)
		d.detectBlock(s.Catch)
	case *ReturnStatement:
		for _, v := range s.Values {
			d.detectExpr(v)
		}
	case *ObjectDestructureStatement:
		d.detectExpr(s.Value)
	case *ArrayDestructureStatement:
		d.detectExpr(s.Value)
	}
}

func (d *dbDetector) detectBlock(block *BlockStatement) {
	if block == nil {
		return
	}
	for _, stmt := range block.Statements {
		d.detectStmt(stmt)
	}
}

func (d *dbDetector) detectExpr(expr Expression) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *CallExpression:
		if dot, ok := e.Function.(*DotExpression); ok {
			if ident, ok := dot.Left.(*Identifier); ok && ident.Value == "db" && dot.Field == "open" {
				if len(e.Arguments) >= 1 {
					if lit, ok := e.Arguments[0].(*StringLiteral); ok {
						d.drivers[lit.Value] = true
					}
				}
			}
		}
		for _, a := range e.Arguments {
			d.detectExpr(a)
		}
	case *InfixExpression:
		d.detectExpr(e.Left)
		d.detectExpr(e.Right)
	case *TernaryExpression:
		d.detectExpr(e.Condition)
		d.detectExpr(e.Consequence)
		d.detectExpr(e.Alternative)
	case *PrefixExpression:
		d.detectExpr(e.Right)
	case *DotExpression:
		d.detectExpr(e.Left)
	case *IndexExpression:
		d.detectExpr(e.Left)
		d.detectExpr(e.Index)
	case *ArrayLiteral:
		for _, el := range e.Elements {
			d.detectExpr(el)
		}
	case *HashLiteral:
		for _, p := range e.Pairs {
			d.detectExpr(p.Key)
			d.detectExpr(p.Value)
		}
	case *AsyncExpression:
		d.detectExpr(e.Expression)
	}
}
