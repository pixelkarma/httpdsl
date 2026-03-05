package compiler

// Node interfaces
type Node interface {
	TokenLiteral() string
}

type Statement interface {
	Node
	statementNode()
}

type Expression interface {
	Node
	expressionNode()
}

// Program is the root AST node
type Program struct {
	Statements []Statement
}

func (p *Program) TokenLiteral() string {
	if len(p.Statements) > 0 {
		return p.Statements[0].TokenLiteral()
	}
	return ""
}

// --- Statements ---

type RouteStatement struct {
	Token     Token
	Method    string
	Path      string
	Params    []string        // extracted :param names from path
	TypeCheck string          // "json", "text", "form", or "" (auto-detect)
	Timeout   int             // per-route timeout in seconds (0 = use server default)
	Body      *BlockStatement
	ElseBlock *BlockStatement // runs on type mismatch or uncaught errors
}

func (s *RouteStatement) statementNode()       {}
func (s *RouteStatement) TokenLiteral() string { return s.Token.Literal }

type FnStatement struct {
	Token  Token
	Name   string
	Params []string
	Body   *BlockStatement
}

func (s *FnStatement) statementNode()       {}
func (s *FnStatement) TokenLiteral() string { return s.Token.Literal }

type ReturnStatement struct {
	Token  Token
	Values []Expression
}

func (s *ReturnStatement) statementNode()       {}
func (s *ReturnStatement) TokenLiteral() string { return s.Token.Literal }

type TryCatchStatement struct {
	Token    Token
	Try      *BlockStatement
	CatchVar string // variable name for error (e.g. "err")
	Catch    *BlockStatement
}

func (s *TryCatchStatement) statementNode()       {}
func (s *TryCatchStatement) TokenLiteral() string { return s.Token.Literal }

type ThrowStatement struct {
	Token Token
	Value Expression // the error value (string, object, anything)
}

func (s *ThrowStatement) statementNode()       {}
func (s *ThrowStatement) TokenLiteral() string { return s.Token.Literal }

type AssignStatement struct {
	Token  Token
	Names  []string
	Values []Expression
}

func (s *AssignStatement) statementNode()       {}
func (s *AssignStatement) TokenLiteral() string { return s.Token.Literal }

type IndexAssignStatement struct {
	Token Token
	Left  Expression // the object/array expression
	Index Expression // the key/index
	Value Expression
}

func (s *IndexAssignStatement) statementNode()       {}
func (s *IndexAssignStatement) TokenLiteral() string { return s.Token.Literal }

type CompoundAssignStatement struct {
	Token    Token
	Name     string
	Operator string // "+=" or "-="
	Value    Expression
}

func (s *CompoundAssignStatement) statementNode()       {}
func (s *CompoundAssignStatement) TokenLiteral() string { return s.Token.Literal }

type IfStatement struct {
	Token       Token
	Condition   Expression
	Consequence *BlockStatement
	Alternative Statement // *BlockStatement or *IfStatement (else if)
}

func (s *IfStatement) statementNode()       {}
func (s *IfStatement) TokenLiteral() string { return s.Token.Literal }

type WhileStatement struct {
	Token     Token
	Condition Expression
	Body      *BlockStatement
}

func (s *WhileStatement) statementNode()       {}
func (s *WhileStatement) TokenLiteral() string { return s.Token.Literal }

type EachStatement struct {
	Token    Token
	Value    string // item variable name
	Index    string // index variable name (optional)
	Iterable Expression
	Body     *BlockStatement
}

func (s *EachStatement) statementNode()       {}
func (s *EachStatement) TokenLiteral() string { return s.Token.Literal }

type ServerStatement struct {
	Token        Token
	Settings     map[string]Expression
	StaticMounts []StaticMountDef
}

type StaticMountDef struct {
	Prefix string
	Dir    string
}

func (s *ServerStatement) statementNode()       {}
func (s *ServerStatement) TokenLiteral() string { return s.Token.Literal }

type ExpressionStatement struct {
	Token      Token
	Expression Expression
}

func (s *ExpressionStatement) statementNode()       {}
func (s *ExpressionStatement) TokenLiteral() string { return s.Token.Literal }

type BlockStatement struct {
	Token      Token
	Statements []Statement
}

func (s *BlockStatement) statementNode()       {}
func (s *BlockStatement) TokenLiteral() string { return s.Token.Literal }

type BreakStatement struct {
	Token Token
}

func (s *BreakStatement) statementNode()       {}
func (s *BreakStatement) TokenLiteral() string { return s.Token.Literal }

type ContinueStatement struct {
	Token Token
}

func (s *ContinueStatement) statementNode()       {}
func (s *ContinueStatement) TokenLiteral() string { return s.Token.Literal }

type ObjectDestructureStatement struct {
	Token Token
	Keys  []string   // the field names to extract
	Value Expression // the right-hand side
}

func (s *ObjectDestructureStatement) statementNode()       {}
func (s *ObjectDestructureStatement) TokenLiteral() string { return s.Token.Literal }

type ArrayDestructureStatement struct {
	Token Token
	Names []string   // variable names
	Value Expression // the right-hand side
}

func (s *ArrayDestructureStatement) statementNode()       {}
func (s *ArrayDestructureStatement) TokenLiteral() string { return s.Token.Literal }

// --- Expressions ---

type Identifier struct {
	Token Token
	Value string
}

func (e *Identifier) expressionNode()      {}
func (e *Identifier) TokenLiteral() string { return e.Token.Literal }

type IntegerLiteral struct {
	Token Token
	Value int64
}

func (e *IntegerLiteral) expressionNode()      {}
func (e *IntegerLiteral) TokenLiteral() string { return e.Token.Literal }

type FloatLiteral struct {
	Token Token
	Value float64
}

func (e *FloatLiteral) expressionNode()      {}
func (e *FloatLiteral) TokenLiteral() string { return e.Token.Literal }

type StringLiteral struct {
	Token Token
	Value string
}

func (e *StringLiteral) expressionNode()      {}
func (e *StringLiteral) TokenLiteral() string { return e.Token.Literal }

type BooleanLiteral struct {
	Token Token
	Value bool
}

func (e *BooleanLiteral) expressionNode()      {}
func (e *BooleanLiteral) TokenLiteral() string { return e.Token.Literal }

type NullLiteral struct {
	Token Token
}

func (e *NullLiteral) expressionNode()      {}
func (e *NullLiteral) TokenLiteral() string { return e.Token.Literal }

type ArrayLiteral struct {
	Token    Token
	Elements []Expression
}

func (e *ArrayLiteral) expressionNode()      {}
func (e *ArrayLiteral) TokenLiteral() string { return e.Token.Literal }

type HashPair struct {
	Key   Expression
	Value Expression
}

type HashLiteral struct {
	Token Token
	Pairs []HashPair
}

func (e *HashLiteral) expressionNode()      {}
func (e *HashLiteral) TokenLiteral() string { return e.Token.Literal }

type PrefixExpression struct {
	Token    Token
	Operator string
	Right    Expression
}

func (e *PrefixExpression) expressionNode()      {}
func (e *PrefixExpression) TokenLiteral() string { return e.Token.Literal }

type InfixExpression struct {
	Token    Token
	Left     Expression
	Operator string
	Right    Expression
}

func (e *InfixExpression) expressionNode()      {}
func (e *InfixExpression) TokenLiteral() string { return e.Token.Literal }

type CallExpression struct {
	Token     Token
	Function  Expression
	Arguments []Expression
}

func (e *CallExpression) expressionNode()      {}
func (e *CallExpression) TokenLiteral() string { return e.Token.Literal }

type IndexExpression struct {
	Token Token
	Left  Expression
	Index Expression
}

func (e *IndexExpression) expressionNode()      {}
func (e *IndexExpression) TokenLiteral() string { return e.Token.Literal }

type DotExpression struct {
	Token Token
	Left  Expression
	Field string
}

func (e *DotExpression) expressionNode()      {}
func (e *DotExpression) TokenLiteral() string { return e.Token.Literal }

type FunctionLiteral struct {
	Token  Token
	Params []string
	Body   *BlockStatement
}

func (e *FunctionLiteral) expressionNode()      {}
func (e *FunctionLiteral) TokenLiteral() string { return e.Token.Literal }

type AsyncExpression struct {
	Token      Token
	Expression Expression
}

func (e *AsyncExpression) expressionNode()      {}
func (e *AsyncExpression) TokenLiteral() string { return e.Token.Literal }

type GroupStatement struct {
	Token  Token
	Prefix string
	Routes []*RouteStatement
	Before []*BlockStatement
	After  []*BlockStatement
}

func (s *GroupStatement) statementNode()       {}
func (s *GroupStatement) TokenLiteral() string { return s.Token.Literal }

type BeforeStatement struct {
	Token Token
	Body  *BlockStatement
}

func (s *BeforeStatement) statementNode()       {}
func (s *BeforeStatement) TokenLiteral() string { return s.Token.Literal }

type AfterStatement struct {
	Token Token
	Body  *BlockStatement
}

func (s *AfterStatement) statementNode()       {}
func (s *AfterStatement) TokenLiteral() string { return s.Token.Literal }

type ErrorStatement struct {
	Token      Token
	StatusCode int
	Body       *BlockStatement
}

func (s *ErrorStatement) statementNode()       {}
func (s *ErrorStatement) TokenLiteral() string { return s.Token.Literal }
