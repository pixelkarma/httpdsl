package compiler

import (
	"fmt"
	"sort"
	"strings"
)

// EmitIRPreview walks IR-backed AST nodes and emits normalized statement/expression
// strings. It is used as an IR backend preflight to ensure we have coverage across
// the language surface before final Go emission.
func EmitIRPreview(ir *IRProgram) (string, error) {
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

func emitPreviewBlock(b *strings.Builder, block *BlockStatement) {
	for _, line := range previewBlockLines(block) {
		b.WriteString(line)
		b.WriteByte('\n')
	}
}

func previewBlockLines(block *BlockStatement) []string {
	if block == nil {
		return nil
	}
	out := make([]string, 0, len(block.Statements))
	for _, stmt := range block.Statements {
		out = append(out, previewStmt(stmt))
	}
	return out
}

func previewStmt(stmt Statement) string {
	switch s := stmt.(type) {
	case *AssignStatement:
		vals := make([]string, 0, len(s.Values))
		for _, v := range s.Values {
			vals = append(vals, previewExpr(v))
		}
		return fmt.Sprintf("assign %s = %s", strings.Join(s.Names, ","), strings.Join(vals, ","))
	case *CompoundAssignStatement:
		return fmt.Sprintf("assign %s %s %s", s.Name, s.Operator, previewExpr(s.Value))
	case *IndexAssignStatement:
		return fmt.Sprintf("assign %s[%s] = %s", previewExpr(s.Left), previewExpr(s.Index), previewExpr(s.Value))
	case *ObjectDestructureStatement:
		return fmt.Sprintf("destructure {%s} = %s", strings.Join(s.Keys, ","), previewExpr(s.Value))
	case *ArrayDestructureStatement:
		return fmt.Sprintf("destructure [%s] = %s", strings.Join(s.Names, ","), previewExpr(s.Value))
	case *ReturnStatement:
		vals := make([]string, 0, len(s.Values))
		for _, v := range s.Values {
			vals = append(vals, previewExpr(v))
		}
		return "return " + strings.Join(vals, ",")
	case *ExpressionStatement:
		return previewExpr(s.Expression)
	case *IfStatement:
		out := "if " + previewExpr(s.Condition)
		if s.Consequence != nil {
			out += " {..}"
		}
		if s.Alternative != nil {
			out += " else {..}"
		}
		return out
	case *SwitchStatement:
		return "switch " + previewExpr(s.Subject)
	case *WhileStatement:
		return "while " + previewExpr(s.Condition)
	case *EachStatement:
		if s.Index != "" {
			return fmt.Sprintf("each %s,%s in %s", s.Value, s.Index, previewExpr(s.Iterable))
		}
		return fmt.Sprintf("each %s in %s", s.Value, previewExpr(s.Iterable))
	case *BreakStatement:
		return "break"
	case *ContinueStatement:
		return "continue"
	case *TryCatchStatement:
		return "try catch(" + s.CatchVar + ")"
	case *ThrowStatement:
		return "throw " + previewExpr(s.Value)
	case *BeforeStatement:
		return "before"
	case *AfterStatement:
		return "after"
	case *InitStatement:
		return "init"
	case *ShutdownStatement:
		return "shutdown"
	case *EveryStatement:
		if s.CronExpr != "" {
			return "every cron " + s.CronExpr
		}
		return fmt.Sprintf("every interval %d", s.Interval)
	case *ErrorStatement:
		return fmt.Sprintf("error %d", s.StatusCode)
	default:
		return fmt.Sprintf("stmt<%T>", stmt)
	}
}

func previewExpr(expr Expression) string {
	switch e := expr.(type) {
	case *Identifier:
		return e.Value
	case *IntegerLiteral:
		return fmt.Sprintf("%d", e.Value)
	case *FloatLiteral:
		return fmt.Sprintf("%g", e.Value)
	case *StringLiteral:
		return fmt.Sprintf("%q", e.Value)
	case *BooleanLiteral:
		if e.Value {
			return "true"
		}
		return "false"
	case *NullLiteral:
		return "null"
	case *ArrayLiteral:
		parts := make([]string, 0, len(e.Elements))
		for _, el := range e.Elements {
			parts = append(parts, previewExpr(el))
		}
		return "[" + strings.Join(parts, ",") + "]"
	case *HashLiteral:
		parts := make([]string, 0, len(e.Pairs))
		for _, p := range e.Pairs {
			parts = append(parts, previewExpr(p.Key)+":"+previewExpr(p.Value))
		}
		sort.Strings(parts)
		return "{" + strings.Join(parts, ",") + "}"
	case *PrefixExpression:
		return e.Operator + previewExpr(e.Right)
	case *InfixExpression:
		return "(" + previewExpr(e.Left) + " " + e.Operator + " " + previewExpr(e.Right) + ")"
	case *TernaryExpression:
		return "(" + previewExpr(e.Condition) + " ? " + previewExpr(e.Consequence) + " : " + previewExpr(e.Alternative) + ")"
	case *CallExpression:
		args := make([]string, 0, len(e.Arguments))
		for _, arg := range e.Arguments {
			args = append(args, previewExpr(arg))
		}
		return previewExpr(e.Function) + "(" + strings.Join(args, ",") + ")"
	case *IndexExpression:
		return previewExpr(e.Left) + "[" + previewExpr(e.Index) + "]"
	case *DotExpression:
		return previewExpr(e.Left) + "." + e.Field
	case *FunctionLiteral:
		return "fn(" + strings.Join(e.Params, ",") + ") {..}"
	case *AsyncExpression:
		return "async " + previewExpr(e.Expression)
	default:
		return fmt.Sprintf("expr<%T>", expr)
	}
}
