package ir

import (
	"fmt"

	front "httpdsl/compiler"
)

// Lower converts AST to a normalized IR snapshot.
func Lower(program *front.Program) *Program {
	ir := &Program{}
	if program == nil {
		return ir
	}
	for _, stmt := range program.Statements {
		line, col := statementLocation(stmt)
		ir.TopLevel = append(ir.TopLevel, TopLevelNode{
			Kind:   classifyTopLevel(stmt),
			Line:   line,
			Column: col,
		})
		switch s := stmt.(type) {
		case *front.ServerStatement:
			if ir.Server == nil {
				ir.Server = &Server{}
			}
			ir.Server.HasServerBlock = true
			if pe, ok := s.Settings["port"]; ok {
				if lit, ok := pe.(*front.IntegerLiteral); ok {
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
		case *front.RouteStatement:
			r := Route{
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
		case *front.GroupStatement:
			g := Group{
				Prefix:      s.Prefix,
				RouteCount:  len(s.Routes),
				BeforeCount: len(s.Before),
				AfterCount:  len(s.After),
			}
			ir.Groups = append(ir.Groups, g)
			for _, route := range s.Routes {
				r := Route{
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
		case *front.FnStatement:
			ir.Functions = append(ir.Functions, Function{
				Name:        s.Name,
				Params:      append([]string(nil), s.Params...),
				BodyPreview: previewBlockLines(s.Body),
			})
		case *front.BeforeStatement:
			ir.Hooks.BeforeCount++
		case *front.AfterStatement:
			ir.Hooks.AfterCount++
		case *front.InitStatement:
			ir.Hooks.InitCount++
		case *front.ShutdownStatement:
			ir.Hooks.ShutdownCount++
		case *front.ErrorStatement:
			ir.Errors = append(ir.Errors, ErrorHandler{StatusCode: s.StatusCode})
		case *front.EveryStatement:
			sched := Schedule{}
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
	drivers, hasDB, hasSQL, hasMongo := detectDBFeatures(program)
	ir.Features.DBDrivers = drivers
	ir.Features.HasDB = hasDB
	ir.Features.HasSQL = hasSQL
	ir.Features.HasMongo = hasMongo
	return ir
}

func classifyTopLevel(stmt front.Statement) TopLevelKind {
	switch stmt.(type) {
	case *front.RouteStatement:
		return TopLevelRoute
	case *front.FnStatement:
		return TopLevelFunction
	case *front.ServerStatement:
		return TopLevelServer
	case *front.GroupStatement:
		return TopLevelGroup
	case *front.BeforeStatement:
		return TopLevelBefore
	case *front.AfterStatement:
		return TopLevelAfter
	case *front.InitStatement:
		return TopLevelInit
	case *front.ShutdownStatement:
		return TopLevelShutdown
	case *front.HelpStatement:
		return TopLevelHelp
	case *front.ErrorStatement:
		return TopLevelError
	case *front.EveryStatement:
		return TopLevelEvery
	default:
		return TopLevelUnknown
	}
}

func detectDBFeatures(program *front.Program) (map[string]bool, bool, bool, bool) {
	drivers := front.DetectDBDrivers(program)
	hasDB := len(drivers) > 0
	hasSQL := drivers["sqlite"] || drivers["postgres"] || drivers["mysql"]
	hasMongo := drivers["mongo"]
	return drivers, hasDB, hasSQL, hasMongo
}

func routeHasBuiltin(block *front.BlockStatement, name string) bool {
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

func stmtHasBuiltin(stmt front.Statement, name string) bool {
	switch s := stmt.(type) {
	case *front.ExpressionStatement:
		return exprHasBuiltin(s.Expression, name)
	case *front.AssignStatement:
		for _, v := range s.Values {
			if exprHasBuiltin(v, name) {
				return true
			}
		}
	case *front.IfStatement:
		if exprHasBuiltin(s.Condition, name) || routeHasBuiltin(s.Consequence, name) {
			return true
		}
		if s.Alternative != nil {
			switch alt := s.Alternative.(type) {
			case *front.BlockStatement:
				return routeHasBuiltin(alt, name)
			case *front.IfStatement:
				return stmtHasBuiltin(alt, name)
			}
		}
	case *front.WhileStatement:
		return exprHasBuiltin(s.Condition, name) || routeHasBuiltin(s.Body, name)
	case *front.EachStatement:
		return exprHasBuiltin(s.Iterable, name) || routeHasBuiltin(s.Body, name)
	case *front.ReturnStatement:
		for _, v := range s.Values {
			if exprHasBuiltin(v, name) {
				return true
			}
		}
	case *front.TryCatchStatement:
		return routeHasBuiltin(s.Try, name) || routeHasBuiltin(s.Catch, name)
	case *front.ThrowStatement:
		return exprHasBuiltin(s.Value, name)
	}
	return false
}

func exprHasBuiltin(expr front.Expression, name string) bool {
	switch e := expr.(type) {
	case *front.CallExpression:
		if id, ok := e.Function.(*front.Identifier); ok && id.Value == name {
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
	case *front.PrefixExpression:
		return exprHasBuiltin(e.Right, name)
	case *front.InfixExpression:
		return exprHasBuiltin(e.Left, name) || exprHasBuiltin(e.Right, name)
	case *front.TernaryExpression:
		return exprHasBuiltin(e.Condition, name) || exprHasBuiltin(e.Consequence, name) || exprHasBuiltin(e.Alternative, name)
	case *front.DotExpression:
		return exprHasBuiltin(e.Left, name)
	case *front.IndexExpression:
		return exprHasBuiltin(e.Left, name) || exprHasBuiltin(e.Index, name)
	case *front.ArrayLiteral:
		for _, el := range e.Elements {
			if exprHasBuiltin(el, name) {
				return true
			}
		}
	case *front.HashLiteral:
		for _, pair := range e.Pairs {
			if exprHasBuiltin(pair.Key, name) || exprHasBuiltin(pair.Value, name) {
				return true
			}
		}
	case *front.AsyncExpression:
		return exprHasBuiltin(e.Expression, name)
	}
	return false
}

// Validate enforces backend-level invariants before emission.
func Validate(ir *Program) []string {
	var errs []string
	if ir == nil {
		return []string{"ir: missing program"}
	}

	serverCount := 0
	firstServerLine, firstServerCol := 0, 0

	for _, r := range ir.Routes {
		if r.HasDisconnect && r.Method != "SSE" {
			errs = append(errs, fmt.Sprintf("route %q: disconnect block only valid for SSE method", r.Path))
		}
	}

	// Mirror top-level constraints defensively in IR validation.
	for _, node := range ir.TopLevel {
		validKind := false
		switch node.Kind {
		case TopLevelRoute,
			TopLevelFunction,
			TopLevelServer,
			TopLevelGroup,
			TopLevelBefore,
			TopLevelAfter,
			TopLevelInit,
			TopLevelShutdown,
			TopLevelHelp,
			TopLevelError,
			TopLevelEvery:
			validKind = true
		default:
			errs = append(errs, fmt.Sprintf("line %d, col %d: invalid top-level statement in IR", node.Line, node.Column))
		}

		if validKind && node.Kind == TopLevelServer {
			serverCount++
			if serverCount == 1 {
				firstServerLine, firstServerCol = node.Line, node.Column
				continue
			}
			errs = append(
				errs,
				fmt.Sprintf(
					"line %d, col %d: duplicate server block; first server block defined at line %d, col %d (only one server {} block is allowed per project)",
					node.Line, node.Column, firstServerLine, firstServerCol,
				),
			)
		}
	}

	return errs
}

func statementLocation(stmt front.Statement) (int, int) {
	switch s := stmt.(type) {
	case *front.RouteStatement:
		return s.Token.Line, s.Token.Column
	case *front.FnStatement:
		return s.Token.Line, s.Token.Column
	case *front.ServerStatement:
		return s.Token.Line, s.Token.Column
	case *front.GroupStatement:
		return s.Token.Line, s.Token.Column
	case *front.BeforeStatement:
		return s.Token.Line, s.Token.Column
	case *front.AfterStatement:
		return s.Token.Line, s.Token.Column
	case *front.InitStatement:
		return s.Token.Line, s.Token.Column
	case *front.ShutdownStatement:
		return s.Token.Line, s.Token.Column
	case *front.HelpStatement:
		return s.Token.Line, s.Token.Column
	case *front.ErrorStatement:
		return s.Token.Line, s.Token.Column
	case *front.EveryStatement:
		return s.Token.Line, s.Token.Column
	case *front.AssignStatement:
		return s.Token.Line, s.Token.Column
	case *front.CompoundAssignStatement:
		return s.Token.Line, s.Token.Column
	case *front.IndexAssignStatement:
		return s.Token.Line, s.Token.Column
	case *front.ExpressionStatement:
		return s.Token.Line, s.Token.Column
	case *front.ObjectDestructureStatement:
		return s.Token.Line, s.Token.Column
	case *front.ArrayDestructureStatement:
		return s.Token.Line, s.Token.Column
	default:
		return 0, 0
	}
}
