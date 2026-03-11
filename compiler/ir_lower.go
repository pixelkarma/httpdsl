package compiler

import "fmt"

// LowerToIR converts AST to a normalized IR snapshot.
func LowerToIR(program *Program) *IRProgram {
	ir := &IRProgram{}
	if program == nil {
		return ir
	}
	for _, stmt := range program.Statements {
		ir.TopLevel = append(ir.TopLevel, IRTopLevelNode{
			Kind:      classifyTopLevel(stmt),
			Statement: stmt,
		})
		switch s := stmt.(type) {
		case *ServerStatement:
			if ir.Server == nil {
				ir.Server = &IRServer{}
			}
			ir.Server.HasServerBlock = true
			if pe, ok := s.Settings["port"]; ok {
				if lit, ok := pe.(*IntegerLiteral); ok {
					ir.Server.Port = int(lit.Value)
				}
			}
			_, hasCert := s.Settings["ssl_cert"]
			_, hasKey := s.Settings["ssl_key"]
			if hasCert && hasKey {
				ir.Server.HasTLS = true
				ir.Features.HasTLS = true
			}
			if _, ok := s.Settings["autocert"]; ok {
				ir.Server.HasAutocert = true
				ir.Features.HasTLS = true
			}
			if _, ok := s.Settings["templates"]; ok {
				ir.Server.HasTemplates = true
			}
			if _, ok := s.Settings["session"]; ok {
				ir.Server.HasSession = true
				ir.Features.HasSession = true
			}
		case *RouteStatement:
			r := IRRoute{
				Method:        s.Method,
				Path:          s.Path,
				TypeCheck:     s.TypeCheck,
				Timeout:       s.Timeout,
				CSRFDisabled:  s.CSRFDisabled,
				HasElse:       s.ElseBlock != nil,
				HasDisconnect: s.DisconnectBlock != nil,
				BodyPreview:   previewBlockLines(s.Body),
				ElsePreview:   previewBlockLines(s.ElseBlock),
				DiscPreview:   previewBlockLines(s.DisconnectBlock),
			}
			ir.Routes = append(ir.Routes, r)
			if s.Method == "SSE" {
				ir.Features.HasSSE = true
			}
			if routeHasBuiltin(s.Body, "exec") {
				ir.Features.HasExec = true
			}
		case *GroupStatement:
			g := IRGroup{
				Prefix:      s.Prefix,
				RouteCount:  len(s.Routes),
				BeforeCount: len(s.Before),
				AfterCount:  len(s.After),
			}
			ir.Groups = append(ir.Groups, g)
			for _, route := range s.Routes {
				r := IRRoute{
					Method:        route.Method,
					Path:          route.Path,
					TypeCheck:     route.TypeCheck,
					Timeout:       route.Timeout,
					CSRFDisabled:  route.CSRFDisabled,
					HasElse:       route.ElseBlock != nil,
					HasDisconnect: route.DisconnectBlock != nil,
					BodyPreview:   previewBlockLines(route.Body),
					ElsePreview:   previewBlockLines(route.ElseBlock),
					DiscPreview:   previewBlockLines(route.DisconnectBlock),
				}
				ir.Routes = append(ir.Routes, r)
				if route.Method == "SSE" {
					ir.Features.HasSSE = true
				}
			}
		case *FnStatement:
			ir.Functions = append(ir.Functions, IRFunction{
				Name:        s.Name,
				Params:      append([]string(nil), s.Params...),
				BodyPreview: previewBlockLines(s.Body),
			})
		case *BeforeStatement:
			ir.Hooks.BeforeCount++
		case *AfterStatement:
			ir.Hooks.AfterCount++
		case *InitStatement:
			ir.Hooks.InitCount++
		case *ShutdownStatement:
			ir.Hooks.ShutdownCount++
		case *ErrorStatement:
			ir.Errors = append(ir.Errors, IRErrorHandler{StatusCode: s.StatusCode})
		case *EveryStatement:
			sched := IRSchedule{}
			if s.CronExpr != "" {
				sched.Kind = "cron"
				sched.CronExpr = s.CronExpr
				ir.Features.HasCron = true
			} else {
				sched.Kind = "interval"
				sched.Interval = s.Interval
			}
			ir.Schedules = append(ir.Schedules, sched)
		}
	}
	drivers, hasDB, hasSQL, hasMongo := detectIRDBFeatures(ir.TopLevel)
	ir.Features.DBDrivers = drivers
	ir.Features.HasDB = hasDB
	ir.Features.HasSQL = hasSQL
	ir.Features.HasMongo = hasMongo
	return ir
}

func classifyTopLevel(stmt Statement) IRTopLevelKind {
	switch stmt.(type) {
	case *RouteStatement:
		return IRTopLevelRoute
	case *FnStatement:
		return IRTopLevelFunction
	case *ServerStatement:
		return IRTopLevelServer
	case *GroupStatement:
		return IRTopLevelGroup
	case *BeforeStatement:
		return IRTopLevelBefore
	case *AfterStatement:
		return IRTopLevelAfter
	case *InitStatement:
		return IRTopLevelInit
	case *ShutdownStatement:
		return IRTopLevelShutdown
	case *HelpStatement:
		return IRTopLevelHelp
	case *ErrorStatement:
		return IRTopLevelError
	case *EveryStatement:
		return IRTopLevelEvery
	default:
		return IRTopLevelUnknown
	}
}

func detectIRDBFeatures(nodes []IRTopLevelNode) (map[string]bool, bool, bool, bool) {
	program := &Program{Statements: make([]Statement, 0, len(nodes))}
	for _, node := range nodes {
		if node.Statement != nil {
			program.Statements = append(program.Statements, node.Statement)
		}
	}
	drivers := DetectDBDrivers(program)
	hasDB := len(drivers) > 0
	hasSQL := drivers["sqlite"] || drivers["postgres"] || drivers["mysql"]
	hasMongo := drivers["mongo"]
	return drivers, hasDB, hasSQL, hasMongo
}

func routeHasBuiltin(block *BlockStatement, name string) bool {
	if block == nil {
		return false
	}
	for _, stmt := range block.Statements {
		if stmtHasBuiltin(stmt, name) {
			return true
		}
	}
	return false
}

func stmtHasBuiltin(stmt Statement, name string) bool {
	switch s := stmt.(type) {
	case *ExpressionStatement:
		return exprHasBuiltin(s.Expression, name)
	case *AssignStatement:
		for _, v := range s.Values {
			if exprHasBuiltin(v, name) {
				return true
			}
		}
	case *IfStatement:
		if exprHasBuiltin(s.Condition, name) || routeHasBuiltin(s.Consequence, name) {
			return true
		}
		if s.Alternative != nil {
			switch alt := s.Alternative.(type) {
			case *BlockStatement:
				return routeHasBuiltin(alt, name)
			case *IfStatement:
				return stmtHasBuiltin(alt, name)
			}
		}
	case *WhileStatement:
		return exprHasBuiltin(s.Condition, name) || routeHasBuiltin(s.Body, name)
	case *EachStatement:
		return exprHasBuiltin(s.Iterable, name) || routeHasBuiltin(s.Body, name)
	case *ReturnStatement:
		for _, v := range s.Values {
			if exprHasBuiltin(v, name) {
				return true
			}
		}
	case *TryCatchStatement:
		return routeHasBuiltin(s.Try, name) || routeHasBuiltin(s.Catch, name)
	case *ThrowStatement:
		return exprHasBuiltin(s.Value, name)
	}
	return false
}

func exprHasBuiltin(expr Expression, name string) bool {
	switch e := expr.(type) {
	case *CallExpression:
		if id, ok := e.Function.(*Identifier); ok && id.Value == name {
			return true
		}
		if exprHasBuiltin(e.Function, name) {
			return true
		}
		for _, arg := range e.Arguments {
			if exprHasBuiltin(arg, name) {
				return true
			}
		}
	case *PrefixExpression:
		return exprHasBuiltin(e.Right, name)
	case *InfixExpression:
		return exprHasBuiltin(e.Left, name) || exprHasBuiltin(e.Right, name)
	case *TernaryExpression:
		return exprHasBuiltin(e.Condition, name) || exprHasBuiltin(e.Consequence, name) || exprHasBuiltin(e.Alternative, name)
	case *DotExpression:
		return exprHasBuiltin(e.Left, name)
	case *IndexExpression:
		return exprHasBuiltin(e.Left, name) || exprHasBuiltin(e.Index, name)
	case *ArrayLiteral:
		for _, el := range e.Elements {
			if exprHasBuiltin(el, name) {
				return true
			}
		}
	case *HashLiteral:
		for _, pair := range e.Pairs {
			if exprHasBuiltin(pair.Key, name) || exprHasBuiltin(pair.Value, name) {
				return true
			}
		}
	case *AsyncExpression:
		return exprHasBuiltin(e.Expression, name)
	}
	return false
}

// ValidateIR enforces backend-level invariants before emission.
func ValidateIR(ir *IRProgram) []string {
	var errs []string
	if ir == nil {
		return []string{"ir: missing program"}
	}

	for _, r := range ir.Routes {
		if r.HasDisconnect && r.Method != "SSE" {
			errs = append(errs, fmt.Sprintf("route %q: disconnect block only valid for SSE method", r.Path))
		}
	}

	// Mirror top-level constraints defensively in IR validation.
	for _, node := range ir.TopLevel {
		if node.Statement == nil {
			errs = append(errs, "ir top-level node has nil statement")
			continue
		}
		switch node.Kind {
		case IRTopLevelRoute,
			IRTopLevelFunction,
			IRTopLevelServer,
			IRTopLevelGroup,
			IRTopLevelBefore,
			IRTopLevelAfter,
			IRTopLevelInit,
			IRTopLevelShutdown,
			IRTopLevelHelp,
			IRTopLevelError,
			IRTopLevelEvery:
			continue
		default:
			line, col := statementLocationForIR(node.Statement)
			errs = append(errs, fmt.Sprintf("line %d, col %d: invalid top-level statement in IR", line, col))
		}
	}

	return errs
}

func statementLocationForIR(stmt Statement) (int, int) {
	switch s := stmt.(type) {
	case *RouteStatement:
		return s.Token.Line, s.Token.Column
	case *FnStatement:
		return s.Token.Line, s.Token.Column
	case *ServerStatement:
		return s.Token.Line, s.Token.Column
	case *GroupStatement:
		return s.Token.Line, s.Token.Column
	case *BeforeStatement:
		return s.Token.Line, s.Token.Column
	case *AfterStatement:
		return s.Token.Line, s.Token.Column
	case *InitStatement:
		return s.Token.Line, s.Token.Column
	case *ShutdownStatement:
		return s.Token.Line, s.Token.Column
	case *HelpStatement:
		return s.Token.Line, s.Token.Column
	case *ErrorStatement:
		return s.Token.Line, s.Token.Column
	case *EveryStatement:
		return s.Token.Line, s.Token.Column
	case *AssignStatement:
		return s.Token.Line, s.Token.Column
	case *CompoundAssignStatement:
		return s.Token.Line, s.Token.Column
	case *IndexAssignStatement:
		return s.Token.Line, s.Token.Column
	case *ExpressionStatement:
		return s.Token.Line, s.Token.Column
	case *ObjectDestructureStatement:
		return s.Token.Line, s.Token.Column
	case *ArrayDestructureStatement:
		return s.Token.Line, s.Token.Column
	default:
		return 0, 0
	}
}
