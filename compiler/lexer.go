package compiler

type Lexer struct {
	input   string
	pos     int  // current position
	readPos int  // next read position
	ch      byte // current char
	line    int
	col     int
}

func NewLexer(input string) *Lexer {
	l := &Lexer{input: input, line: 1, col: 0}
	l.readChar()
	return l
}

func (l *Lexer) readChar() {
	if l.readPos >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.readPos]
	}
	l.pos = l.readPos
	l.readPos++
	l.col++
}

func (l *Lexer) peekChar() byte {
	if l.readPos >= len(l.input) {
		return 0
	}
	return l.input[l.readPos]
}

func (l *Lexer) NextToken() Token {
	l.skipWhitespaceAndComments()

	tok := Token{Line: l.line, Column: l.col}

	switch l.ch {
	case '=':
		if l.peekChar() == '=' {
			l.readChar()
			tok.Type = TOKEN_EQ
			tok.Literal = "=="
		} else {
			tok.Type = TOKEN_ASSIGN
			tok.Literal = "="
		}
	case '+':
		if l.peekChar() == '=' {
			l.readChar()
			tok.Type = TOKEN_PLUS_EQ
			tok.Literal = "+="
		} else {
			tok.Type = TOKEN_PLUS
			tok.Literal = "+"
		}
	case '-':
		if l.peekChar() == '=' {
			l.readChar()
			tok.Type = TOKEN_MINUS_EQ
			tok.Literal = "-="
		} else {
			tok.Type = TOKEN_MINUS
			tok.Literal = "-"
		}
	case '*':
		tok.Type = TOKEN_STAR
		tok.Literal = "*"
	case '/':
		tok.Type = TOKEN_SLASH
		tok.Literal = "/"
	case '%':
		tok.Type = TOKEN_PERCENT
		tok.Literal = "%"
	case '!':
		if l.peekChar() == '=' {
			l.readChar()
			tok.Type = TOKEN_NEQ
			tok.Literal = "!="
		} else {
			tok.Type = TOKEN_BANG
			tok.Literal = "!"
		}
	case '<':
		if l.peekChar() == '=' {
			l.readChar()
			tok.Type = TOKEN_LTE
			tok.Literal = "<="
		} else {
			tok.Type = TOKEN_LT
			tok.Literal = "<"
		}
	case '>':
		if l.peekChar() == '=' {
			l.readChar()
			tok.Type = TOKEN_GTE
			tok.Literal = ">="
		} else {
			tok.Type = TOKEN_GT
			tok.Literal = ">"
		}
	case '&':
		if l.peekChar() == '&' {
			l.readChar()
			tok.Type = TOKEN_AND
			tok.Literal = "&&"
		} else {
			tok.Type = TOKEN_ILLEGAL
			tok.Literal = string(l.ch)
		}
	case '|':
		if l.peekChar() == '|' {
			l.readChar()
			tok.Type = TOKEN_OR
			tok.Literal = "||"
		} else {
			tok.Type = TOKEN_ILLEGAL
			tok.Literal = string(l.ch)
		}
	case ',':
		tok.Type = TOKEN_COMMA
		tok.Literal = ","
	case '.':
		tok.Type = TOKEN_DOT
		tok.Literal = "."
	case ':':
		tok.Type = TOKEN_COLON
		tok.Literal = ":"
	case '(':
		tok.Type = TOKEN_LPAREN
		tok.Literal = "("
	case ')':
		tok.Type = TOKEN_RPAREN
		tok.Literal = ")"
	case '{':
		tok.Type = TOKEN_LBRACE
		tok.Literal = "{"
	case '}':
		tok.Type = TOKEN_RBRACE
		tok.Literal = "}"
	case '[':
		tok.Type = TOKEN_LBRACKET
		tok.Literal = "["
	case ']':
		tok.Type = TOKEN_RBRACKET
		tok.Literal = "]"
	case '"':
		tok.Type = TOKEN_STRING
		tok.Literal = l.readString()
	case 0:
		tok.Type = TOKEN_EOF
		tok.Literal = ""
	default:
		if isLetter(l.ch) {
			lit := l.readIdentifier()
			tok.Type = LookupIdent(lit)
			tok.Literal = lit
			return tok // readIdentifier already advanced past
		} else if isDigit(l.ch) {
			lit, isFloat := l.readNumber()
			if isFloat {
				tok.Type = TOKEN_FLOAT
			} else {
				tok.Type = TOKEN_INT
			}
			tok.Literal = lit
			return tok // readNumber already advanced past
		} else {
			tok.Type = TOKEN_ILLEGAL
			tok.Literal = string(l.ch)
		}
	}

	l.readChar()
	return tok
}

func (l *Lexer) skipWhitespaceAndComments() {
	for {
		// Skip whitespace
		for l.ch == ' ' || l.ch == '\t' || l.ch == '\r' || l.ch == '\n' {
			if l.ch == '\n' {
				l.line++
				l.col = 0
			}
			l.readChar()
		}

		// Skip line comments
		if l.ch == '/' && l.peekChar() == '/' {
			for l.ch != '\n' && l.ch != 0 {
				l.readChar()
			}
			continue
		}

		// Skip block comments
		if l.ch == '/' && l.peekChar() == '*' {
			l.readChar() // skip /
			l.readChar() // skip *
			for {
				if l.ch == 0 {
					break
				}
				if l.ch == '\n' {
					l.line++
					l.col = 0
				}
				if l.ch == '*' && l.peekChar() == '/' {
					l.readChar() // skip *
					l.readChar() // skip /
					break
				}
				l.readChar()
			}
			continue
		}

		break
	}
}

func (l *Lexer) readIdentifier() string {
	start := l.pos
	for isLetter(l.ch) || isDigit(l.ch) {
		l.readChar()
	}
	return l.input[start:l.pos]
}

func (l *Lexer) readNumber() (string, bool) {
	start := l.pos
	isFloat := false
	for isDigit(l.ch) {
		l.readChar()
	}
	if l.ch == '.' && isDigit(l.peekChar()) {
		isFloat = true
		l.readChar() // skip .
		for isDigit(l.ch) {
			l.readChar()
		}
	}
	return l.input[start:l.pos], isFloat
}

func (l *Lexer) readString() string {
	l.readChar() // skip opening "
	start := l.pos
	for l.ch != '"' && l.ch != 0 {
		if l.ch == '\\' {
			l.readChar() // skip escaped char
		}
		if l.ch == '\n' {
			l.line++
			l.col = 0
		}
		l.readChar()
	}
	result := l.input[start:l.pos]
	// don't readChar here — NextToken will advance past the closing "
	return result
}

func isLetter(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}
