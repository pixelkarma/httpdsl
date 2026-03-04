package compiler

import (
	"fmt"
	"strconv"
	"strings"
)

// Precedence levels
const (
	_ int = iota
	PREC_LOWEST
	PREC_OR      // ||
	PREC_AND     // &&
	PREC_EQUALS  // == !=
	PREC_COMPARE // < > <= >=
	PREC_SUM     // + -
	PREC_PRODUCT // * / %
	PREC_PREFIX  // -x !x
	PREC_CALL    // fn() obj.field obj[idx]
)

var precedences = map[TokenType]int{
	TOKEN_OR:       PREC_OR,
	TOKEN_AND:      PREC_AND,
	TOKEN_EQ:       PREC_EQUALS,
	TOKEN_NEQ:      PREC_EQUALS,
	TOKEN_LT:       PREC_COMPARE,
	TOKEN_GT:       PREC_COMPARE,
	TOKEN_LTE:      PREC_COMPARE,
	TOKEN_GTE:      PREC_COMPARE,
	TOKEN_PLUS:     PREC_SUM,
	TOKEN_MINUS:    PREC_SUM,
	TOKEN_STAR:     PREC_PRODUCT,
	TOKEN_SLASH:    PREC_PRODUCT,
	TOKEN_PERCENT:  PREC_PRODUCT,
	TOKEN_LPAREN:   PREC_CALL,
	TOKEN_DOT:      PREC_CALL,
	TOKEN_LBRACKET: PREC_CALL,
}

type Parser struct {
	lexer   *Lexer
	curTok  Token
	peekTok Token
	errors  []string
}

func NewParser(l *Lexer) *Parser {
	p := &Parser{lexer: l}
	p.nextToken()
	p.nextToken()
	return p
}

func (p *Parser) Errors() []string {
	return p.errors
}

func (p *Parser) addError(format string, args ...interface{}) {
	msg := fmt.Sprintf("line %d, col %d: %s", p.curTok.Line, p.curTok.Column, fmt.Sprintf(format, args...))
	p.errors = append(p.errors, msg)
}

func (p *Parser) nextToken() {
	p.curTok = p.peekTok
	p.peekTok = p.lexer.NextToken()
}

func (p *Parser) curTokenIs(t TokenType) bool  { return p.curTok.Type == t }
func (p *Parser) peekTokenIs(t TokenType) bool { return p.peekTok.Type == t }

func (p *Parser) expectPeek(t TokenType) bool {
	if p.peekTokenIs(t) {
		p.nextToken()
		return true
	}
	p.addError("expected %s, got %s", t, p.peekTok.Type)
	return false
}

func (p *Parser) peekPrecedence() int {
	if p, ok := precedences[p.peekTok.Type]; ok {
		return p
	}
	return PREC_LOWEST
}

func (p *Parser) curPrecedence() int {
	if p, ok := precedences[p.curTok.Type]; ok {
		return p
	}
	return PREC_LOWEST
}

// ParseProgram is the entry point
func (p *Parser) ParseProgram() *Program {
	program := &Program{}
	for !p.curTokenIs(TOKEN_EOF) {
		stmt := p.parseStatement()
		if stmt != nil {
			program.Statements = append(program.Statements, stmt)
		}
	}
	return program
}

func (p *Parser) parseStatement() Statement {
	switch p.curTok.Type {
	case TOKEN_ROUTE:
		return p.parseRouteStatement()
	case TOKEN_FN:
		return p.parseFnStatement()
	case TOKEN_RETURN:
		return p.parseReturnStatement()
	case TOKEN_IF:
		return p.parseIfStatement()
	case TOKEN_WHILE:
		return p.parseWhileStatement()
	case TOKEN_EACH:
		return p.parseEachStatement()
	case TOKEN_SERVER:
		return p.parseServerStatement()
	case TOKEN_BREAK:
		stmt := &BreakStatement{Token: p.curTok}
		p.nextToken()
		return stmt
	case TOKEN_CONTINUE:
		stmt := &ContinueStatement{Token: p.curTok}
		p.nextToken()
		return stmt
	case TOKEN_TRY:
		return p.parseTryCatchStatement()
	case TOKEN_THROW:
		return p.parseThrowStatement()
	case TOKEN_IDENT:
		// Could be assignment (x = ...) or expression statement (fn call)
		return p.parseIdentStartStatement()
	default:
		return p.parseExpressionStatement()
	}
}

// route GET "/path" { ... }
func (p *Parser) parseRouteStatement() Statement {
	stmt := &RouteStatement{Token: p.curTok}
	p.nextToken() // skip 'route'

	// Method: expect an identifier like GET, POST, PUT, DELETE, PATCH
	if !p.curTokenIs(TOKEN_IDENT) {
		p.addError("expected HTTP method, got %s", p.curTok.Type)
		return nil
	}
	stmt.Method = strings.ToUpper(p.curTok.Literal)
	p.nextToken()

	// Path
	if !p.curTokenIs(TOKEN_STRING) {
		p.addError("expected path string, got %s", p.curTok.Type)
		return nil
	}
	stmt.Path = p.curTok.Literal
	// Extract :params from path
	for _, seg := range strings.Split(stmt.Path, "/") {
		if strings.HasPrefix(seg, ":") {
			stmt.Params = append(stmt.Params, seg[1:])
		}
	}
	p.nextToken()

	// Optional type check: json, text, or form
	if p.curTokenIs(TOKEN_JSON) {
		stmt.TypeCheck = "json"
		p.nextToken()
	} else if p.curTokenIs(TOKEN_TEXT) {
		stmt.TypeCheck = "text"
		p.nextToken()
	} else if p.curTokenIs(TOKEN_IDENT) && p.curTok.Literal == "form" {
		stmt.TypeCheck = "form"
		p.nextToken()
	}

	// Body block
	if !p.curTokenIs(TOKEN_LBRACE) {
		p.addError("expected '{', got %s", p.curTok.Type)
		return nil
	}
	stmt.Body = p.parseBlockStatement()

	// Optional else block
	if p.curTokenIs(TOKEN_ELSE) {
		p.nextToken() // skip 'else'
		if !p.curTokenIs(TOKEN_LBRACE) {
			p.addError("expected '{' after else, got %s", p.curTok.Type)
			return nil
		}
		stmt.ElseBlock = p.parseBlockStatement()
	}

	return stmt
}

// fn name(a, b) { ... }
func (p *Parser) parseFnStatement() Statement {
	stmt := &FnStatement{Token: p.curTok}
	p.nextToken() // skip 'fn'

	if !p.curTokenIs(TOKEN_IDENT) {
		p.addError("expected function name, got %s", p.curTok.Type)
		return nil
	}
	stmt.Name = p.curTok.Literal
	p.nextToken()

	if !p.curTokenIs(TOKEN_LPAREN) {
		p.addError("expected '(', got %s", p.curTok.Type)
		return nil
	}
	stmt.Params = p.parseFnParams()

	if !p.curTokenIs(TOKEN_LBRACE) {
		p.addError("expected '{', got %s", p.curTok.Type)
		return nil
	}
	stmt.Body = p.parseBlockStatement()
	return stmt
}

func (p *Parser) parseFnParams() []string {
	p.nextToken() // skip '('
	var params []string
	if p.curTokenIs(TOKEN_RPAREN) {
		p.nextToken()
		return params
	}
	if p.curTokenIs(TOKEN_IDENT) {
		params = append(params, p.curTok.Literal)
		p.nextToken()
	}
	for p.curTokenIs(TOKEN_COMMA) {
		p.nextToken() // skip comma
		if p.curTokenIs(TOKEN_IDENT) {
			params = append(params, p.curTok.Literal)
			p.nextToken()
		}
	}
	if p.curTokenIs(TOKEN_RPAREN) {
		p.nextToken()
	}
	return params
}

// return | return expr, expr
func (p *Parser) parseReturnStatement() Statement {
	tok := p.curTok
	p.nextToken() // skip 'return'

	// Regular return with expression list
	stmt := &ReturnStatement{Token: tok}
	if p.curTokenIs(TOKEN_RBRACE) || p.curTokenIs(TOKEN_EOF) {
		return stmt
	}
	stmt.Values = append(stmt.Values, p.parseExpression(PREC_LOWEST))
	for p.curTokenIs(TOKEN_COMMA) {
		p.nextToken()
		stmt.Values = append(stmt.Values, p.parseExpression(PREC_LOWEST))
	}
	return stmt
}

// try { ... } catch(varname) { ... }
func (p *Parser) parseTryCatchStatement() Statement {
	stmt := &TryCatchStatement{Token: p.curTok}
	p.nextToken() // skip 'try'

	if !p.curTokenIs(TOKEN_LBRACE) {
		p.addError("expected '{' after try, got %s", p.curTok.Type)
		return nil
	}
	stmt.Try = p.parseBlockStatement()

	if !p.curTokenIs(TOKEN_CATCH) {
		p.addError("expected 'catch' after try block, got %s", p.curTok.Type)
		return nil
	}
	p.nextToken() // skip 'catch'

	if !p.curTokenIs(TOKEN_LPAREN) {
		p.addError("expected '(' after catch, got %s", p.curTok.Type)
		return nil
	}
	p.nextToken() // skip '('

	if !p.curTokenIs(TOKEN_IDENT) {
		p.addError("expected variable name in catch, got %s", p.curTok.Type)
		return nil
	}
	stmt.CatchVar = p.curTok.Literal
	p.nextToken()

	if !p.curTokenIs(TOKEN_RPAREN) {
		p.addError("expected ')' after catch variable, got %s", p.curTok.Type)
		return nil
	}
	p.nextToken() // skip ')'

	if !p.curTokenIs(TOKEN_LBRACE) {
		p.addError("expected '{' after catch(...), got %s", p.curTok.Type)
		return nil
	}
	stmt.Catch = p.parseBlockStatement()

	return stmt
}

// throw expression
func (p *Parser) parseThrowStatement() Statement {
	stmt := &ThrowStatement{Token: p.curTok}
	p.nextToken() // skip 'throw'
	stmt.Value = p.parseExpression(PREC_LOWEST)
	return stmt
}

func (p *Parser) parseIfStatement() Statement {
	stmt := &IfStatement{Token: p.curTok}
	p.nextToken() // skip 'if'

	stmt.Condition = p.parseExpression(PREC_LOWEST)

	if !p.curTokenIs(TOKEN_LBRACE) {
		p.addError("expected '{', got %s", p.curTok.Type)
		return nil
	}
	stmt.Consequence = p.parseBlockStatement()

	if p.curTokenIs(TOKEN_ELSE) {
		p.nextToken()
		if p.curTokenIs(TOKEN_IF) {
			stmt.Alternative = p.parseIfStatement()
		} else if p.curTokenIs(TOKEN_LBRACE) {
			stmt.Alternative = p.parseBlockStatement()
		} else {
			p.addError("expected '{' or 'if' after else")
		}
	}
	return stmt
}

// while(expr) { ... }
func (p *Parser) parseWhileStatement() Statement {
	stmt := &WhileStatement{Token: p.curTok}
	p.nextToken() // skip 'while'

	if p.curTokenIs(TOKEN_LPAREN) {
		p.nextToken() // skip (
		stmt.Condition = p.parseExpression(PREC_LOWEST)
		if p.curTokenIs(TOKEN_RPAREN) {
			p.nextToken() // skip )
		}
	} else {
		stmt.Condition = p.parseExpression(PREC_LOWEST)
	}

	if !p.curTokenIs(TOKEN_LBRACE) {
		p.addError("expected '{', got %s", p.curTok.Type)
		return nil
	}
	stmt.Body = p.parseBlockStatement()
	return stmt
}

// each item, index in expr { ... }
func (p *Parser) parseEachStatement() Statement {
	stmt := &EachStatement{Token: p.curTok}
	p.nextToken() // skip 'each'

	if !p.curTokenIs(TOKEN_IDENT) {
		p.addError("expected variable name after 'each'")
		return nil
	}
	stmt.Value = p.curTok.Literal
	p.nextToken()

	// Optional index
	if p.curTokenIs(TOKEN_COMMA) {
		p.nextToken()
		if p.curTokenIs(TOKEN_IDENT) {
			stmt.Index = p.curTok.Literal
			p.nextToken()
		}
	}

	if !p.curTokenIs(TOKEN_IN) {
		p.addError("expected 'in' keyword")
		return nil
	}
	p.nextToken() // skip 'in'

	stmt.Iterable = p.parseExpression(PREC_LOWEST)

	if !p.curTokenIs(TOKEN_LBRACE) {
		p.addError("expected '{', got %s", p.curTok.Type)
		return nil
	}
	stmt.Body = p.parseBlockStatement()
	return stmt
}

// server { port 3000 }
func (p *Parser) parseServerStatement() Statement {
	stmt := &ServerStatement{Token: p.curTok, Settings: make(map[string]Expression)}
	p.nextToken() // skip 'server'

	if !p.curTokenIs(TOKEN_LBRACE) {
		p.addError("expected '{', got %s", p.curTok.Type)
		return nil
	}
	p.nextToken() // skip '{'

	for !p.curTokenIs(TOKEN_RBRACE) && !p.curTokenIs(TOKEN_EOF) {
		if p.curTokenIs(TOKEN_IDENT) {
			key := p.curTok.Literal
			p.nextToken()
			val := p.parseExpression(PREC_LOWEST)
			stmt.Settings[key] = val
		} else {
			p.addError("expected setting name in server block")
			p.nextToken()
		}
	}
	if p.curTokenIs(TOKEN_RBRACE) {
		p.nextToken()
	}
	return stmt
}

// Starts with identifier — could be assignment or expression
func (p *Parser) parseIdentStartStatement() Statement {
	// Look ahead to see if this is an assignment pattern
	// x = ...
	// x, y = ...
	// x += ...
	// x -= ...
	// or just an expression like fn()

	// Save state: we need to peek ahead
	firstIdent := p.curTok

	// Check for compound assignment: x += expr, x -= expr
	if p.peekTokenIs(TOKEN_PLUS_EQ) || p.peekTokenIs(TOKEN_MINUS_EQ) {
		p.nextToken() // move to += or -=
		op := p.curTok
		p.nextToken() // skip operator
		val := p.parseExpression(PREC_LOWEST)
		return &CompoundAssignStatement{
			Token:    firstIdent,
			Name:     firstIdent.Literal,
			Operator: op.Literal,
			Value:    val,
		}
	}

	if p.peekTokenIs(TOKEN_ASSIGN) {
		// Simple assignment: x = expr
		p.nextToken() // move to =
		p.nextToken() // skip =
		stmt := &AssignStatement{Token: firstIdent}
		stmt.Names = []string{firstIdent.Literal}
		stmt.Values = append(stmt.Values, p.parseExpression(PREC_LOWEST))
		for p.curTokenIs(TOKEN_COMMA) {
			p.nextToken()
			stmt.Values = append(stmt.Values, p.parseExpression(PREC_LOWEST))
		}
		return stmt
	}

	if p.peekTokenIs(TOKEN_COMMA) {
		// Multi-assignment: x, y = fn()
		names := []string{firstIdent.Literal}
		p.nextToken() // move to comma
		for p.curTokenIs(TOKEN_COMMA) {
			p.nextToken() // skip comma
			if p.curTokenIs(TOKEN_IDENT) {
				names = append(names, p.curTok.Literal)
				p.nextToken()
			}
		}
		if p.curTokenIs(TOKEN_ASSIGN) {
			p.nextToken() // skip =
			stmt := &AssignStatement{Token: firstIdent, Names: names}
			stmt.Values = append(stmt.Values, p.parseExpression(PREC_LOWEST))
			for p.curTokenIs(TOKEN_COMMA) {
				p.nextToken()
				stmt.Values = append(stmt.Values, p.parseExpression(PREC_LOWEST))
			}
			return stmt
		}
		// Not an assignment after all — parse error
		p.addError("expected '=' in multi-assignment")
		return nil
	}

	// Otherwise it's an expression statement
	return p.parseExpressionStatement()
}

func (p *Parser) parseExpressionStatement() Statement {
	tok := p.curTok
	expr := p.parseExpression(PREC_LOWEST)

	// Check if this is an index assignment: expr[key] = val or expr.field = val
	if p.curTokenIs(TOKEN_ASSIGN) {
		p.nextToken() // skip =
		val := p.parseExpression(PREC_LOWEST)
		switch e := expr.(type) {
		case *IndexExpression:
			return &IndexAssignStatement{Token: tok, Left: e.Left, Index: e.Index, Value: val}
		case *DotExpression:
			return &IndexAssignStatement{Token: tok, Left: e.Left, Index: &StringLiteral{Token: tok, Value: e.Field}, Value: val}
		}
	}

	stmt := &ExpressionStatement{Token: tok}
	stmt.Expression = expr
	return stmt
}

func (p *Parser) parseBlockStatement() *BlockStatement {
	block := &BlockStatement{Token: p.curTok}
	p.nextToken() // skip '{'
	for !p.curTokenIs(TOKEN_RBRACE) && !p.curTokenIs(TOKEN_EOF) {
		stmt := p.parseStatement()
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)
		}
	}
	if p.curTokenIs(TOKEN_RBRACE) {
		p.nextToken() // skip '}'
	}
	return block
}

// --- Expression parsing (Pratt parser) ---

func (p *Parser) parseExpression(precedence int) Expression {
	left := p.parsePrefixExpr()
	if left == nil {
		return nil
	}

	for !p.curTokenIs(TOKEN_EOF) && precedence < p.curPrecedence() {
		left = p.parseInfixExpr(left)
		if left == nil {
			return nil
		}
	}
	return left
}

func (p *Parser) parsePrefixExpr() Expression {
	switch p.curTok.Type {
	case TOKEN_IDENT:
		expr := &Identifier{Token: p.curTok, Value: p.curTok.Literal}
		p.nextToken()
		return expr
	case TOKEN_INT:
		val, _ := strconv.ParseInt(p.curTok.Literal, 10, 64)
		expr := &IntegerLiteral{Token: p.curTok, Value: val}
		p.nextToken()
		return expr
	case TOKEN_FLOAT:
		val, _ := strconv.ParseFloat(p.curTok.Literal, 64)
		expr := &FloatLiteral{Token: p.curTok, Value: val}
		p.nextToken()
		return expr
	case TOKEN_STRING:
		expr := &StringLiteral{Token: p.curTok, Value: p.curTok.Literal}
		p.nextToken()
		return expr
	case TOKEN_TRUE:
		expr := &BooleanLiteral{Token: p.curTok, Value: true}
		p.nextToken()
		return expr
	case TOKEN_FALSE:
		expr := &BooleanLiteral{Token: p.curTok, Value: false}
		p.nextToken()
		return expr
	case TOKEN_NULL:
		expr := &NullLiteral{Token: p.curTok}
		p.nextToken()
		return expr
	case TOKEN_BANG, TOKEN_MINUS:
		tok := p.curTok
		p.nextToken()
		right := p.parseExpression(PREC_PREFIX)
		return &PrefixExpression{Token: tok, Operator: tok.Literal, Right: right}
	case TOKEN_LPAREN:
		p.nextToken() // skip (
		expr := p.parseExpression(PREC_LOWEST)
		if p.curTokenIs(TOKEN_RPAREN) {
			p.nextToken()
		}
		return expr
	case TOKEN_LBRACKET:
		return p.parseArrayLiteral()
	case TOKEN_LBRACE:
		return p.parseHashLiteral()
	case TOKEN_ENV:
		// env("VAR") — treat as a call expression on identifier "env"
		expr := &Identifier{Token: p.curTok, Value: "env"}
		p.nextToken()
		return expr
	case TOKEN_JSON:
		// json.parse(...) — treat json as identifier for dot access
		expr := &Identifier{Token: p.curTok, Value: "json"}
		p.nextToken()
		return expr
	default:
		p.addError("unexpected token: %s (%q)", p.curTok.Type, p.curTok.Literal)
		p.nextToken()
		return nil
	}
}

func (p *Parser) parseInfixExpr(left Expression) Expression {
	switch p.curTok.Type {
	case TOKEN_PLUS, TOKEN_MINUS, TOKEN_STAR, TOKEN_SLASH, TOKEN_PERCENT,
		TOKEN_EQ, TOKEN_NEQ, TOKEN_LT, TOKEN_GT, TOKEN_LTE, TOKEN_GTE,
		TOKEN_AND, TOKEN_OR:
		tok := p.curTok
		prec := p.curPrecedence()
		p.nextToken()
		right := p.parseExpression(prec)
		return &InfixExpression{Token: tok, Left: left, Operator: tok.Literal, Right: right}
	case TOKEN_LPAREN:
		return p.parseCallExpression(left)
	case TOKEN_DOT:
		tok := p.curTok
		p.nextToken() // skip .
		if !p.curTokenIs(TOKEN_IDENT) && !p.curTokenIs(TOKEN_JSON) && !p.curTokenIs(TOKEN_TEXT) {
			p.addError("expected field name after '.', got %s", p.curTok.Type)
			return left
		}
		field := p.curTok.Literal
		p.nextToken()
		return &DotExpression{Token: tok, Left: left, Field: field}
	case TOKEN_LBRACKET:
		tok := p.curTok
		p.nextToken() // skip [
		idx := p.parseExpression(PREC_LOWEST)
		if p.curTokenIs(TOKEN_RBRACKET) {
			p.nextToken()
		}
		return &IndexExpression{Token: tok, Left: left, Index: idx}
	default:
		return left
	}
}

func (p *Parser) parseCallExpression(fn Expression) Expression {
	tok := p.curTok
	p.nextToken() // skip (
	var args []Expression
	if !p.curTokenIs(TOKEN_RPAREN) {
		args = append(args, p.parseExpression(PREC_LOWEST))
		for p.curTokenIs(TOKEN_COMMA) {
			p.nextToken()
			args = append(args, p.parseExpression(PREC_LOWEST))
		}
	}
	if p.curTokenIs(TOKEN_RPAREN) {
		p.nextToken()
	}
	return &CallExpression{Token: tok, Function: fn, Arguments: args}
}

func (p *Parser) parseArrayLiteral() Expression {
	tok := p.curTok
	p.nextToken() // skip [
	var elements []Expression
	if !p.curTokenIs(TOKEN_RBRACKET) {
		elements = append(elements, p.parseExpression(PREC_LOWEST))
		for p.curTokenIs(TOKEN_COMMA) {
			p.nextToken()
			if p.curTokenIs(TOKEN_RBRACKET) {
				break // trailing comma
			}
			elements = append(elements, p.parseExpression(PREC_LOWEST))
		}
	}
	if p.curTokenIs(TOKEN_RBRACKET) {
		p.nextToken()
	}
	return &ArrayLiteral{Token: tok, Elements: elements}
}

func (p *Parser) parseHashLiteral() Expression {
	tok := p.curTok
	p.nextToken() // skip {
	var pairs []HashPair
	if !p.curTokenIs(TOKEN_RBRACE) {
		for {
			key := p.parseExpression(PREC_LOWEST)
			if !p.curTokenIs(TOKEN_COLON) {
				p.addError("expected ':' in hash literal")
				break
			}
			p.nextToken() // skip :
			val := p.parseExpression(PREC_LOWEST)
			pairs = append(pairs, HashPair{Key: key, Value: val})
			if !p.curTokenIs(TOKEN_COMMA) {
				break
			}
			p.nextToken() // skip comma
			if p.curTokenIs(TOKEN_RBRACE) {
				break // trailing comma
			}
		}
	}
	if p.curTokenIs(TOKEN_RBRACE) {
		p.nextToken()
	}
	return &HashLiteral{Token: tok, Pairs: pairs}
}
