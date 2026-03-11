package goemit

import front "httpdsl/compiler"

type Program = front.Program

type Statement = front.Statement
type Expression = front.Expression

type Token = front.Token

type RouteStatement = front.RouteStatement
type FnStatement = front.FnStatement
type ReturnStatement = front.ReturnStatement
type TryCatchStatement = front.TryCatchStatement
type ThrowStatement = front.ThrowStatement
type AssignStatement = front.AssignStatement
type IndexAssignStatement = front.IndexAssignStatement
type CompoundAssignStatement = front.CompoundAssignStatement
type IfStatement = front.IfStatement
type EveryStatement = front.EveryStatement
type SwitchStatement = front.SwitchStatement
type CaseClause = front.CaseClause
type WhileStatement = front.WhileStatement
type EachStatement = front.EachStatement
type ServerStatement = front.ServerStatement
type StaticMountDef = front.StaticMountDef
type ExpressionStatement = front.ExpressionStatement
type BlockStatement = front.BlockStatement
type BreakStatement = front.BreakStatement
type ContinueStatement = front.ContinueStatement
type ObjectDestructureStatement = front.ObjectDestructureStatement
type ArrayDestructureStatement = front.ArrayDestructureStatement
type GroupStatement = front.GroupStatement
type BeforeStatement = front.BeforeStatement
type AfterStatement = front.AfterStatement
type InitStatement = front.InitStatement
type ShutdownStatement = front.ShutdownStatement
type HelpStatement = front.HelpStatement
type ErrorStatement = front.ErrorStatement

type Identifier = front.Identifier
type IntegerLiteral = front.IntegerLiteral
type FloatLiteral = front.FloatLiteral
type StringLiteral = front.StringLiteral
type BooleanLiteral = front.BooleanLiteral
type NullLiteral = front.NullLiteral
type ArrayLiteral = front.ArrayLiteral
type HashPair = front.HashPair
type HashLiteral = front.HashLiteral
type PrefixExpression = front.PrefixExpression
type InfixExpression = front.InfixExpression
type TernaryExpression = front.TernaryExpression
type CallExpression = front.CallExpression
type IndexExpression = front.IndexExpression
type DotExpression = front.DotExpression
type FunctionLiteral = front.FunctionLiteral
type AsyncExpression = front.AsyncExpression

type TypeInfo = front.TypeInfo
type TypeEnv = front.TypeEnv

const (
	TypeUnknown = front.TypeUnknown
	TypeInt     = front.TypeInt
	TypeFloat   = front.TypeFloat
	TypeString  = front.TypeString
	TypeBool    = front.TypeBool
	TypeArray   = front.TypeArray
	TypeMap     = front.TypeMap
	TypeDynamic = front.TypeDynamic
)

var InferFunctionTypes = front.InferFunctionTypes
