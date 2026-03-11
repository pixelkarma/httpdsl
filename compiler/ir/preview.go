package ir

import (
	"fmt"
	"sort"
	"strings"

	front "httpdsl/compiler"
)

// EmitPreview walks IR summaries and emits normalized statement/expression strings.
// It acts as a preflight check that lowering covered core language surfaces.
func EmitPreview(ir *Program) (string, error) {
	if ir == nil {
		return "", fmt.Errorf("ir is nil")
	}
	var b strings.Builder
	for _, fn := range ir.Functions {
		b.WriteString("fn ")
		b.WriteString(fn.Name)
		b.WriteString("(")
		b.WriteString(strings.Join(fn.Params, ","))
		b.WriteString(")\n")
		for _, line := range fn.BodyPreview {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	for _, route := range ir.Routes {
		b.WriteString("route ")
		b.WriteString(route.Method)
		b.WriteString(" ")
		b.WriteString(route.Path)
		b.WriteByte('\n')
		for _, line := range route.BodyPreview {
			b.WriteString(line)
			b.WriteByte('\n')
		}
		for _, line := range route.ElsePreview {
			b.WriteString(line)
			b.WriteByte('\n')
		}
		for _, line := range route.DiscPreview {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return b.String(), nil
}

func previewBlockLines(block *front.BlockStatement) []string {
	if block == nil {
		return nil
	}
	out := make([]string, 0, len(block.Statements))
	for _, stmt := range block.Statements {
		out = append(out, previewStmt(stmt))
	}
	return out
}

func previewStmt(stmt front.Statement) string {
	switch s := stmt.(type) {
	case *front.AssignStatement:
		vals := make([]string, 0, len(s.Values))
		for _, v := range s.Values {
			vals = append(vals, previewExpr(v))
		}
		return fmt.Sprintf("assign %s = %s", strings.Join(s.Names, ","), strings.Join(vals, ","))
	case *front.CompoundAssignStatement:
		return fmt.Sprintf("assign %s %s %s", s.Name, s.Operator, previewExpr(s.Value))
	case *front.IndexAssignStatement:
		return fmt.Sprintf("assign %s[%s] = %s", previewExpr(s.Left), previewExpr(s.Index), previewExpr(s.Value))
	case *front.ObjectDestructureStatement:
		return fmt.Sprintf("destructure {%s} = %s", strings.Join(s.Keys, ","), previewExpr(s.Value))
	case *front.ArrayDestructureStatement:
		return fmt.Sprintf("destructure [%s] = %s", strings.Join(s.Names, ","), previewExpr(s.Value))
	case *front.ReturnStatement:
		vals := make([]string, 0, len(s.Values))
		for _, v := range s.Values {
			vals = append(vals, previewExpr(v))
		}
		return "return " + strings.Join(vals, ",")
	case *front.ExpressionStatement:
		return previewExpr(s.Expression)
	case *front.IfStatement:
		out := "if " + previewExpr(s.Condition)
		if s.Consequence != nil {
			out += " {..}"
		}
		if s.Alternative != nil {
			out += " else {..}"
		}
		return out
	case *front.SwitchStatement:
		return "switch " + previewExpr(s.Subject)
	case *front.WhileStatement:
		return "while " + previewExpr(s.Condition)
	case *front.EachStatement:
		if s.Index != "" {
			return fmt.Sprintf("each %s,%s in %s", s.Value, s.Index, previewExpr(s.Iterable))
		}
		return fmt.Sprintf("each %s in %s", s.Value, previewExpr(s.Iterable))
	case *front.BreakStatement:
		return "break"
	case *front.ContinueStatement:
		return "continue"
	case *front.TryCatchStatement:
		return "try catch(" + s.CatchVar + ")"
	case *front.ThrowStatement:
		return "throw " + previewExpr(s.Value)
	case *front.BeforeStatement:
		return "before"
	case *front.AfterStatement:
		return "after"
	case *front.InitStatement:
		return "init"
	case *front.ShutdownStatement:
		return "shutdown"
	case *front.EveryStatement:
		if s.CronExpr != "" {
			return "every cron " + s.CronExpr
		}
		return fmt.Sprintf("every interval %d", s.Interval)
	case *front.ErrorStatement:
		return fmt.Sprintf("error %d", s.StatusCode)
	default:
		return fmt.Sprintf("stmt<%T>", stmt)
	}
}

func previewExpr(expr front.Expression) string {
	switch e := expr.(type) {
	case *front.Identifier:
		return e.Value
	case *front.IntegerLiteral:
		return fmt.Sprintf("%d", e.Value)
	case *front.FloatLiteral:
		return fmt.Sprintf("%g", e.Value)
	case *front.StringLiteral:
		return fmt.Sprintf("%q", e.Value)
	case *front.BooleanLiteral:
		if e.Value {
			return "true"
		}
		return "false"
	case *front.NullLiteral:
		return "null"
	case *front.ArrayLiteral:
		parts := make([]string, 0, len(e.Elements))
		for _, el := range e.Elements {
			parts = append(parts, previewExpr(el))
		}
		return "[" + strings.Join(parts, ",") + "]"
	case *front.HashLiteral:
		parts := make([]string, 0, len(e.Pairs))
		for _, p := range e.Pairs {
			parts = append(parts, previewExpr(p.Key)+":"+previewExpr(p.Value))
		}
		sort.Strings(parts)
		return "{" + strings.Join(parts, ",") + "}"
	case *front.PrefixExpression:
		return e.Operator + previewExpr(e.Right)
	case *front.InfixExpression:
		return "(" + previewExpr(e.Left) + " " + e.Operator + " " + previewExpr(e.Right) + ")"
	case *front.TernaryExpression:
		return "(" + previewExpr(e.Condition) + " ? " + previewExpr(e.Consequence) + " : " + previewExpr(e.Alternative) + ")"
	case *front.CallExpression:
		args := make([]string, 0, len(e.Arguments))
		for _, arg := range e.Arguments {
			args = append(args, previewExpr(arg))
		}
		return previewExpr(e.Function) + "(" + strings.Join(args, ",") + ")"
	case *front.IndexExpression:
		return previewExpr(e.Left) + "[" + previewExpr(e.Index) + "]"
	case *front.DotExpression:
		return previewExpr(e.Left) + "." + e.Field
	case *front.FunctionLiteral:
		return "fn(" + strings.Join(e.Params, ",") + ") {..}"
	case *front.AsyncExpression:
		return "async " + previewExpr(e.Expression)
	default:
		return fmt.Sprintf("expr<%T>", expr)
	}
}
