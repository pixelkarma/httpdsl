package compiler

type TokenType int

const (
	TOKEN_ILLEGAL TokenType = iota
	TOKEN_EOF

	// Literals
	TOKEN_IDENT
	TOKEN_INT
	TOKEN_FLOAT
	TOKEN_STRING
	TOKEN_TEMPLATE_STRING // backtick template

	// Operators
	TOKEN_ASSIGN   // =
	TOKEN_PLUS     // +
	TOKEN_MINUS    // -
	TOKEN_STAR     // *
	TOKEN_SLASH    // /
	TOKEN_PERCENT  // %
	TOKEN_BANG     // !
	TOKEN_EQ       // ==
	TOKEN_NEQ      // !=
	TOKEN_LT       // <
	TOKEN_GT       // >
	TOKEN_LTE      // <=
	TOKEN_GTE      // >=
	TOKEN_AND      // &&
	TOKEN_OR       // ||
	TOKEN_PLUS_EQ  // +=
	TOKEN_MINUS_EQ // -=

	// Delimiters
	TOKEN_COMMA    // ,
	TOKEN_DOT      // .
	TOKEN_COLON    // :
	TOKEN_LPAREN   // (
	TOKEN_RPAREN   // )
	TOKEN_LBRACE   // {
	TOKEN_RBRACE   // }
	TOKEN_LBRACKET // [
	TOKEN_RBRACKET // ]

	// Keywords
	TOKEN_ROUTE
	TOKEN_FN
	TOKEN_RETURN
	TOKEN_IF
	TOKEN_ELSE
	TOKEN_WHILE
	TOKEN_EACH
	TOKEN_IN
	TOKEN_SERVER
	TOKEN_JSON
	TOKEN_TEXT
	TOKEN_TRUE
	TOKEN_FALSE
	TOKEN_NULL
	TOKEN_ENV
	TOKEN_FILE
	TOKEN_DB
	TOKEN_BREAK
	TOKEN_CONTINUE
	TOKEN_TRY
	TOKEN_CATCH
	TOKEN_THROW
	TOKEN_ASYNC
)

var keywords = map[string]TokenType{
	"route":    TOKEN_ROUTE,
	"fn":       TOKEN_FN,
	"return":   TOKEN_RETURN,
	"if":       TOKEN_IF,
	"else":     TOKEN_ELSE,
	"while":    TOKEN_WHILE,
	"each":     TOKEN_EACH,
	"in":       TOKEN_IN,
	"server":   TOKEN_SERVER,
	"json":     TOKEN_JSON,
	"text":     TOKEN_TEXT,
	"true":     TOKEN_TRUE,
	"false":    TOKEN_FALSE,
	"null":     TOKEN_NULL,
	"env":      TOKEN_ENV,
	"file":     TOKEN_FILE,
	"db":       TOKEN_DB,
	"break":    TOKEN_BREAK,
	"continue": TOKEN_CONTINUE,
	"try":      TOKEN_TRY,
	"catch":    TOKEN_CATCH,
	"throw":    TOKEN_THROW,
	"async":    TOKEN_ASYNC,
}

type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Column  int
}

func LookupIdent(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return TOKEN_IDENT
}

func (t TokenType) String() string {
	switch t {
	case TOKEN_ILLEGAL:
		return "ILLEGAL"
	case TOKEN_EOF:
		return "EOF"
	case TOKEN_IDENT:
		return "IDENT"
	case TOKEN_INT:
		return "INT"
	case TOKEN_FLOAT:
		return "FLOAT"
	case TOKEN_STRING:
		return "STRING"
	case TOKEN_ASSIGN:
		return "="
	case TOKEN_PLUS:
		return "+"
	case TOKEN_MINUS:
		return "-"
	case TOKEN_STAR:
		return "*"
	case TOKEN_SLASH:
		return "/"
	case TOKEN_PERCENT:
		return "%"
	case TOKEN_BANG:
		return "!"
	case TOKEN_EQ:
		return "=="
	case TOKEN_NEQ:
		return "!="
	case TOKEN_LT:
		return "<"
	case TOKEN_GT:
		return ">"
	case TOKEN_LTE:
		return "<="
	case TOKEN_GTE:
		return ">="
	case TOKEN_AND:
		return "&&"
	case TOKEN_OR:
		return "||"
	case TOKEN_PLUS_EQ:
		return "+="
	case TOKEN_MINUS_EQ:
		return "-="
	case TOKEN_COMMA:
		return ","
	case TOKEN_DOT:
		return "."
	case TOKEN_COLON:
		return ":"
	case TOKEN_LPAREN:
		return "("
	case TOKEN_RPAREN:
		return ")"
	case TOKEN_LBRACE:
		return "{"
	case TOKEN_RBRACE:
		return "}"
	case TOKEN_LBRACKET:
		return "["
	case TOKEN_RBRACKET:
		return "]"
	case TOKEN_ROUTE:
		return "ROUTE"
	case TOKEN_FN:
		return "FN"
	case TOKEN_RETURN:
		return "RETURN"
	case TOKEN_IF:
		return "IF"
	case TOKEN_ELSE:
		return "ELSE"
	case TOKEN_WHILE:
		return "WHILE"
	case TOKEN_EACH:
		return "EACH"
	case TOKEN_IN:
		return "IN"
	case TOKEN_SERVER:
		return "SERVER"
	case TOKEN_JSON:
		return "JSON"
	case TOKEN_TEXT:
		return "TEXT"
	case TOKEN_TRUE:
		return "TRUE"
	case TOKEN_FALSE:
		return "FALSE"
	case TOKEN_NULL:
		return "NULL"
	case TOKEN_ENV:
		return "ENV"
	case TOKEN_FILE:
		return "FILE"
	case TOKEN_DB:
		return "DB"
	case TOKEN_BREAK:
		return "BREAK"
	case TOKEN_CONTINUE:
		return "CONTINUE"
	case TOKEN_TRY:
		return "TRY"
	case TOKEN_CATCH:
		return "CATCH"
	case TOKEN_THROW:
		return "THROW"
	case TOKEN_ASYNC:
		return "ASYNC"
	case TOKEN_TEMPLATE_STRING:
		return "TEMPLATE_STRING"
	default:
		return "UNKNOWN"
	}
}
