package goemit

import (
	"fmt"
	"strconv"
	"strings"
)

func (c *NativeCompiler) expr(e Expression) string {
	if e == nil {
		return "null"
	}
	switch ex := e.(type) {
	case *IntegerLiteral:
		return fmt.Sprintf("Value(int64(%d))", ex.Value)
	case *FloatLiteral:
		return fmt.Sprintf("Value(float64(%s))", strconv.FormatFloat(ex.Value, 'f', -1, 64))
	case *StringLiteral:
		return fmt.Sprintf("Value(%q)", ex.Value)
	case *BooleanLiteral:
		if ex.Value {
			return "Value(true)"
		}
		return "Value(false)"
	case *NullLiteral:
		return "null"
	case *Identifier:
		return c.identExpr(ex.Value)
	case *PrefixExpression:
		return c.prefixExpr(ex)
	case *InfixExpression:
		return c.infixExpr(ex)
	case *TernaryExpression:
		return fmt.Sprintf("ternary(%s, %s, %s)", c.expr(ex.Condition), c.expr(ex.Consequence), c.expr(ex.Alternative))
	case *CallExpression:
		return c.callExpr(ex)
	case *DotExpression:
		return c.dotExpr(ex)
	case *IndexExpression:
		return fmt.Sprintf("indexValue(%s, %s)", c.expr(ex.Left), c.expr(ex.Index))
	case *ArrayLiteral:
		if len(ex.Elements) == 0 {
			return "Value([]Value{})"
		}
		elems := make([]string, len(ex.Elements))
		for i, el := range ex.Elements {
			elems[i] = c.expr(el)
		}
		return fmt.Sprintf("Value([]Value{%s})", strings.Join(elems, ", "))
	case *HashLiteral:
		if len(ex.Pairs) == 0 {
			return "Value(map[string]Value{})"
		}
		pairs := make([]string, len(ex.Pairs))
		for i, p := range ex.Pairs {
			key := c.hashKeyStr(p.Key)
			pairs[i] = fmt.Sprintf("%s: %s", key, c.expr(p.Value))
		}
		return fmt.Sprintf("Value(map[string]Value{%s})", strings.Join(pairs, ", "))
	case *FunctionLiteral:
		// Anonymous function — always variadic so callValue can invoke
		var fb strings.Builder
		old := c.b
		c.b = fb
		// Unpack args into named params
		for i, p := range ex.Params {
			c.lnf("var %s Value = null", safeIdent(p))
			c.lnf("if len(_args) > %d { %s = _args[%d] }", i, safeIdent(p), i)
		}
		c.emitBlock(ex.Body, false)
		c.b.WriteString(strings.Repeat("\t", c.indent+1))
		c.b.WriteString("return null\n")
		body := c.b.String()
		c.b = old
		return fmt.Sprintf("Value(func(_args ...Value) Value {\n%s%s})", body, strings.Repeat("\t", c.indent))
	case *AsyncExpression:
		inner := c.expr(ex.Expression)
		tmp := c.tmp()
		return fmt.Sprintf("func() Value { %s := &dslFuture{ch: make(chan Value, 1)}; go func() { %s.ch <- %s }(); return %s }()", tmp, tmp, inner, tmp)
	}
	return "null"
}

func (c *NativeCompiler) hashKeyStr(e Expression) string {
	switch k := e.(type) {
	case *StringLiteral:
		return fmt.Sprintf("%q", k.Value)
	case *Identifier:
		return fmt.Sprintf("%q", k.Value)
	default:
		return fmt.Sprintf("valueToString(%s)", c.expr(e))
	}
}

func (c *NativeCompiler) identExpr(name string) string {
	// Known builtins map to builtin_xxx functions
	builtinNames := map[string]bool{
		"print": true, "len": true, "str": true, "int": true, "float": true,
		"bool": true, "type": true, "append": true, "push": true, "keys": true, "values": true,
		"contains": true, "trim": true, "split": true,
		"join": true, "upper": true, "lower": true, "replace": true,
		"starts_with": true, "ends_with": true, "slice": true, "reverse": true,
		"unique": true, "merge": true, "patch": true, "delete": true, "index_of": true,
		"repeat": true, "flat": true, "sort": true, "sort_by": true,
		"regex_match": true, "regex_replace": true, "rand": true,
		"uuid": true, "cuid2": true, "abs": true, "ceil": true, "floor": true, "round": true,
		"base64_encode": true, "base64_decode": true, "url_encode": true, "url_decode": true,
		"hash": true, "hmac_hash": true, "hash_password": true, "verify_password": true,
		"validate": true, "is_email": true, "is_url": true, "is_uuid": true, "is_numeric": true,
		"log": true, "log_info": true, "log_warn": true, "log_error": true,
		"map": true, "filter": true, "reduce": true,
		"find": true, "some": true, "every": true, "count": true,
		"pluck": true, "group_by": true, "chunk": true, "range": true,
		"sum": true, "min": true, "max": true, "clamp": true,
		"pad_left": true, "pad_right": true, "truncate": true, "capitalize": true,
		"date": true, "date_format": true, "date_parse": true, "strtotime": true,
		"redirect":          true,
		"server_stats":      true,
		"set_session_store": true,
		"csrf_token":        true,
		"csrf_field":        true,
		"render":            true,

		"exec": true,
	}
	if builtinNames[name] {
		// Return as a callable value - but since Go can't store these directly,
		// we handle them at call sites instead.
		// For non-call references, wrap in a func
		return fmt.Sprintf("builtin_%s", safeIdent(name))
	}
	// User functions
	for _, fn := range c.functions {
		if fn.Name == name {
			return "fn_" + safeIdent(name)
		}
	}
	return safeIdent(name)
}

func (c *NativeCompiler) prefixExpr(e *PrefixExpression) string {
	switch e.Operator {
	case "-":
		return fmt.Sprintf("subtractValues(int64(0), %s)", c.expr(e.Right))
	case "!":
		return fmt.Sprintf("Value(!isTruthy(%s))", c.expr(e.Right))
	}
	return c.expr(e.Right)
}

func (c *NativeCompiler) infixExpr(e *InfixExpression) string {
	l := c.expr(e.Left)
	r := c.expr(e.Right)
	switch e.Operator {
	case "+":
		return fmt.Sprintf("addValues(%s, %s)", l, r)
	case "-":
		return fmt.Sprintf("subtractValues(%s, %s)", l, r)
	case "*":
		return fmt.Sprintf("multiplyValues(%s, %s)", l, r)
	case "/":
		return fmt.Sprintf("divideValues(%s, %s)", l, r)
	case "%":
		return fmt.Sprintf("modValues(%s, %s)", l, r)
	case "==":
		return fmt.Sprintf("Value(valuesEqual(%s, %s))", l, r)
	case "!=":
		return fmt.Sprintf("Value(!valuesEqual(%s, %s))", l, r)
	case "<":
		return fmt.Sprintf("Value(compareLess(%s, %s))", l, r)
	case ">":
		return fmt.Sprintf("Value(compareLess(%s, %s))", r, l)
	case "<=":
		return fmt.Sprintf("Value(compareLess(%s, %s) || valuesEqual(%s, %s))", l, r, l, r)
	case ">=":
		return fmt.Sprintf("Value(compareLess(%s, %s) || valuesEqual(%s, %s))", r, l, l, r)
	case "&&":
		return fmt.Sprintf("logicalAnd(%s, %s)", l, r)
	case "||":
		return fmt.Sprintf("logicalOr(%s, %s)", l, r)
	case "??":
		return fmt.Sprintf("nullCoalesce(%s, %s)", l, r)
	}
	return "null"
}

func (c *NativeCompiler) callExpr(e *CallExpression) string {
	args := make([]string, len(e.Arguments))
	for i, a := range e.Arguments {
		args[i] = c.expr(a)
	}
	argStr := strings.Join(args, ", ")

	// Handle dot-call patterns: json.parse, json.stringify, store.*
	if dot, ok := e.Function.(*DotExpression); ok {
		// Nested dot: request.session.destroy()
		if c.sessionEnabled {
			if outerDot, ok := dot.Left.(*DotExpression); ok {
				if ident, ok := outerDot.Left.(*Identifier); ok && ident.Value == "request" && outerDot.Field == "session" && dot.Field == "destroy" {
					return "(func() Value { _sessDestroyed = true; return null })()"
				}
			}
		}
		if ident, ok := dot.Left.(*Identifier); ok {
			switch ident.Value {
			case "json":
				switch dot.Field {
				case "parse":
					return fmt.Sprintf("builtin_json_parse(%s)", argStr)
				case "stringify":
					return fmt.Sprintf("builtin_json_stringify(%s)", argStr)
				}
			case "file":
				switch dot.Field {
				case "open":
					return fmt.Sprintf("builtin_file_open(%s)", argStr)
				case "read":
					return fmt.Sprintf("builtin_file_read(%s)", argStr)
				case "write":
					return fmt.Sprintf("builtin_file_write(%s)", argStr)
				case "append":
					return fmt.Sprintf("builtin_file_append(%s)", argStr)
				case "read_json":
					return fmt.Sprintf("builtin_file_read_json(%s)", argStr)
				case "write_json":
					return fmt.Sprintf("builtin_file_write_json(%s)", argStr)
				case "exists":
					return fmt.Sprintf("builtin_file_exists(%s)", argStr)
				case "delete":
					return fmt.Sprintf("builtin_file_delete(%s)", argStr)
				case "list":
					return fmt.Sprintf("builtin_file_list(%s)", argStr)
				case "mkdir":
					return fmt.Sprintf("builtin_file_mkdir(%s)", argStr)
				case "chmod":
					return fmt.Sprintf("builtin_file_chmod(%s)", argStr)
				}
			case "db":
				switch dot.Field {
				case "open":
					return fmt.Sprintf("dslDBOpen(%s)", argStr)
				}
			case "jwt":
				c.usedBuiltins["jwt"] = true
				switch dot.Field {
				case "sign":
					return fmt.Sprintf("builtin_jwt_sign(%s)", argStr)
				case "verify":
					return fmt.Sprintf("builtin_jwt_verify(%s)", argStr)
				}
			case "store":
				switch dot.Field {
				case "get":
					if len(args) >= 2 {
						return fmt.Sprintf("storeGet(valueToString(%s), %s)", args[0], args[1])
					} else if len(args) == 1 {
						return fmt.Sprintf("storeGet(valueToString(%s), null)", args[0])
					}
					return "null"
				case "set":
					if len(args) >= 3 {
						return fmt.Sprintf("storeSet(valueToString(%s), %s, toInt64(%s))", args[0], args[1], args[2])
					} else if len(args) >= 2 {
						return fmt.Sprintf("storeSet(valueToString(%s), %s, 0)", args[0], args[1])
					}
					return "null"
				case "delete":
					if len(args) >= 1 {
						return fmt.Sprintf("(func() Value { storeDelete(valueToString(%s)); return null })()", args[0])
					}
					return "null"
				case "has":
					if len(args) >= 1 {
						return fmt.Sprintf("Value(storeHas(valueToString(%s)))", args[0])
					}
					return "Value(false)"
				case "all":
					return "storeAll()"
				case "incr":
					if len(args) >= 3 {
						return fmt.Sprintf("Value(storeIncr(valueToString(%s), toInt64(%s), toInt64(%s)))", args[0], args[1], args[2])
					} else if len(args) >= 2 {
						return fmt.Sprintf("Value(storeIncr(valueToString(%s), toInt64(%s), 0))", args[0], args[1])
					} else if len(args) == 1 {
						return fmt.Sprintf("Value(storeIncr(valueToString(%s), 1, 0))", args[0])
					}
					return "Value(int64(0))"
				case "sync":
					return fmt.Sprintf("storeSync(%s)", argStr)
				}
			case "stream":
				switch dot.Field {
				case "send":
					if len(args) >= 2 {
						return fmt.Sprintf("_sseStreamSend(stream, valueToString(%s), %s)", args[0], args[1])
					} else if len(args) == 1 {
						return fmt.Sprintf(`_sseStreamSend(stream, "message", %s)`, args[0])
					}
					return "null"
				case "join":
					if len(args) >= 1 {
						return fmt.Sprintf("_sseStreamJoin(stream, valueToString(%s))", args[0])
					}
					return "null"
				case "leave":
					if len(args) >= 1 {
						return fmt.Sprintf("_sseStreamLeave(stream, valueToString(%s))", args[0])
					}
					return "null"
				case "set":
					if len(args) >= 2 {
						return fmt.Sprintf("_sseStreamSet(stream, valueToString(%s), %s)", args[0], args[1])
					}
					return "null"
				case "get":
					if len(args) >= 1 {
						return fmt.Sprintf("_sseStreamGet(stream, valueToString(%s))", args[0])
					}
					return "null"
				case "close":
					return "_sseStreamClose(stream)"
				case "channels":
					return "_sseStreamChannels(stream)"
				}
			case "sse":
				switch dot.Field {
				case "find":
					if len(args) >= 1 {
						return fmt.Sprintf("_sseFind(%s)", args[0])
					}
					return "null"
				case "find_by":
					if len(args) >= 2 {
						return fmt.Sprintf("_sseFindBy(%s, %s)", args[0], args[1])
					}
					return "Value([]Value{})"
				case "channel":
					if len(args) >= 1 {
						return fmt.Sprintf("_sseChannel(%s)", args[0])
					}
					return "null"
				case "broadcast":
					return fmt.Sprintf("_sseBroadcast(%s)", argStr)
				case "count":
					return "_sseCount()"
				case "channels":
					return "_sseChannels()"
				}
			}
		}
		// File handle method calls: f.read(), f.write(), f.lines(), etc.
		// Must come before DB handle dispatch since both support .delete()
		if ident, ok := dot.Left.(*Identifier); ok {
			switch ident.Value {
			case "file", "json", "store", "db", "jwt", "stream", "sse", "request", "response":
				// skip — handled by namespace dispatch above
			default:
				objExpr := c.expr(dot.Left)
				switch dot.Field {
				case "read":
					return fmt.Sprintf("fileHandleRead(%s)", objExpr)
				case "write":
					return fmt.Sprintf("fileHandleWrite(%s, %s)", objExpr, argStr)
				case "append":
					return fmt.Sprintf("fileHandleAppend(%s, %s)", objExpr, argStr)
				case "lines":
					if argStr == "" {
						return fmt.Sprintf("fileHandleLines(%s)", objExpr)
					}
					return fmt.Sprintf("fileHandleLines(%s, %s)", objExpr, argStr)
				case "json":
					return fmt.Sprintf("fileHandleJSON(%s)", objExpr)
				case "write_json":
					return fmt.Sprintf("fileHandleWriteJSON(%s, %s)", objExpr, argStr)
				case "exists":
					return fmt.Sprintf("fileHandleExists(%s)", objExpr)
				case "size":
					return fmt.Sprintf("fileHandleSize(%s)", objExpr)
				case "delete":
					return fmt.Sprintf("fileHandleDelete(%s)", objExpr)
				case "path":
					return fmt.Sprintf("fileHandlePath(%s)", objExpr)
				}
			}
		}
		// Method calls on db handles: mydb.query(...) etc.
		if len(c.dbDrivers) > 0 {
			objExpr := c.expr(dot.Left)
			switch dot.Field {
			case "exec":
				return fmt.Sprintf("dslDBExec(%s, %s)", objExpr, argStr)
			case "query":
				return fmt.Sprintf("dslDBQuery(%s, %s)", objExpr, argStr)
			case "query_one":
				return fmt.Sprintf("dslDBQueryOne(%s, %s)", objExpr, argStr)
			case "query_value":
				return fmt.Sprintf("dslDBQueryValue(%s, %s)", objExpr, argStr)
			case "insert":
				return fmt.Sprintf("dslMongoInsert(%s, %s)", objExpr, argStr)
			case "insert_many":
				return fmt.Sprintf("dslMongoInsertMany(%s, %s)", objExpr, argStr)
			case "find":
				return fmt.Sprintf("dslMongoFind(%s, %s)", objExpr, argStr)
			case "find_one":
				return fmt.Sprintf("dslMongoFindOne(%s, %s)", objExpr, argStr)
			case "update":
				return fmt.Sprintf("dslMongoUpdate(%s, %s)", objExpr, argStr)
			case "delete":
				return fmt.Sprintf("dslMongoDelete(%s, %s)", objExpr, argStr)
			case "count":
				return fmt.Sprintf("dslMongoCount(%s, %s)", objExpr, argStr)
			case "close":
				return fmt.Sprintf("dslDBClose(%s)", objExpr)
			}
		}
		// SSE handle method dispatch (stream handles from sse.find, channel handles from sse.channel)
		if c.hasSSE {
			objExpr := c.expr(dot.Left)
			switch dot.Field {
			case "send":
				if len(args) >= 2 {
					return fmt.Sprintf("_sseStreamSend(%s, valueToString(%s), %s)", objExpr, args[0], args[1])
				} else if len(args) == 1 {
					return fmt.Sprintf(`_sseStreamSend(%s, "message", %s)`, objExpr, args[0])
				}
				return "null"
			case "set":
				if len(args) >= 2 {
					return fmt.Sprintf("_sseStreamSet(%s, valueToString(%s), %s)", objExpr, args[0], args[1])
				}
				return "null"
			case "get":
				if len(args) >= 1 {
					return fmt.Sprintf("_sseStreamGet(%s, valueToString(%s))", objExpr, args[0])
				}
				return "null"
			case "join":
				if len(args) >= 1 {
					return fmt.Sprintf("_sseStreamJoin(%s, valueToString(%s))", objExpr, args[0])
				}
				return "null"
			case "leave":
				if len(args) >= 1 {
					return fmt.Sprintf("_sseStreamLeave(%s, valueToString(%s))", objExpr, args[0])
				}
				return "null"
			case "close":
				return fmt.Sprintf("_sseStreamClose(%s)", objExpr)
			case "channels":
				return fmt.Sprintf("_sseStreamChannels(%s)", objExpr)
			case "streams":
				return fmt.Sprintf("_sseChannelStreams(%s)", objExpr)
			case "count":
				return fmt.Sprintf("_sseChannelCount(%s)", objExpr)
			}
		}
	}

	// Handle known builtins by name
	if ident, ok := e.Function.(*Identifier); ok {
		switch ident.Value {
		case "print":
			return fmt.Sprintf("builtin_print(%s)", argStr)
		case "env":
			return fmt.Sprintf("builtin_env(%s)", argStr)
		case "await":
			return fmt.Sprintf("builtin_await(%s)", argStr)
		case "race":
			return fmt.Sprintf("builtin_race(%s)", argStr)
		case "sleep":
			return fmt.Sprintf("builtin_sleep(%s)", argStr)
		case "fetch":
			return fmt.Sprintf("builtin_fetch(%s)", argStr)
		case "now":
			return "Value(time.Now().Unix())"
		case "server_stats":
			return "builtin_server_stats()"
		case "set_session_store":
			return fmt.Sprintf("set_session_store(%s)", argStr)
		case "csrf_token":
			return "csrfToken(_sessData)"
		case "csrf_field":
			return "csrfField(_sessData)"
		case "render":
			if len(args) < 1 {
				return "null"
			}
			nameExpr := args[0]
			pageExpr := "Value(map[string]Value{})"
			if len(args) >= 2 {
				pageExpr = args[1]
			}
			if c.csrfEnabled && c.sessionEnabled {
				return fmt.Sprintf("_render(valueToString(%s), %s, request, _sessData)", nameExpr, pageExpr)
			}
			return fmt.Sprintf("_render(valueToString(%s), %s, request)", nameExpr, pageExpr)
		case "broadcast":
			if c.hasSSE {
				return fmt.Sprintf("_sseBroadcast(%s)", argStr)
			}
			return "null"
		case "exec":
			return fmt.Sprintf("builtin_exec(%s)", argStr)
		case "now_ms":
			return "Value(time.Now().UnixMilli())"
		case "len":
			return fmt.Sprintf("builtin_len(%s)", argStr)
		case "str":
			return fmt.Sprintf("builtin_str(%s)", argStr)
		case "int":
			return fmt.Sprintf("builtin_int(%s)", argStr)
		case "float":
			return fmt.Sprintf("builtin_float(%s)", argStr)
		case "bool":
			return fmt.Sprintf("builtin_bool(%s)", argStr)
		case "type":
			return fmt.Sprintf("builtin_type(%s)", argStr)
		case "append":
			return fmt.Sprintf("builtin_append(%s)", argStr)
		case "push":
			// As expression, push behaves like append (returns new array)
			return fmt.Sprintf("builtin_append(%s)", argStr)
		case "keys":
			return fmt.Sprintf("builtin_keys(%s)", argStr)
		case "values":
			return fmt.Sprintf("builtin_values(%s)", argStr)
		case "contains":
			return fmt.Sprintf("builtin_contains(%s)", argStr)
		case "trim":
			return fmt.Sprintf("builtin_trim(%s)", argStr)
		case "split":
			return fmt.Sprintf("builtin_split(%s)", argStr)
		case "join":
			return fmt.Sprintf("builtin_join(%s)", argStr)
		case "upper":
			return fmt.Sprintf("builtin_upper(%s)", argStr)
		case "lower":
			return fmt.Sprintf("builtin_lower(%s)", argStr)
		case "replace":
			return fmt.Sprintf("builtin_replace(%s)", argStr)
		case "starts_with":
			return fmt.Sprintf("builtin_starts_with(%s)", argStr)
		case "ends_with":
			return fmt.Sprintf("builtin_ends_with(%s)", argStr)
		case "slice":
			return fmt.Sprintf("builtin_slice(%s)", argStr)
		case "reverse":
			return fmt.Sprintf("builtin_reverse(%s)", argStr)
		case "unique":
			return fmt.Sprintf("builtin_unique(%s)", argStr)
		case "merge":
			return fmt.Sprintf("builtin_merge(%s)", argStr)
		case "patch":
			return fmt.Sprintf("builtin_patch(%s)", argStr)
		case "delete":
			return fmt.Sprintf("builtin_delete(%s)", argStr)
		case "index_of":
			return fmt.Sprintf("builtin_index_of(%s)", argStr)
		case "repeat":
			return fmt.Sprintf("builtin_repeat(%s)", argStr)
		case "flat":
			return fmt.Sprintf("builtin_flat(%s)", argStr)
		case "sort":
			return fmt.Sprintf("builtin_sort(%s)", argStr)
		case "sort_by":
			return fmt.Sprintf("builtin_sort_by(%s)", argStr)
		case "regex_match":
			return fmt.Sprintf("builtin_regex_match(%s)", argStr)
		case "regex_replace":
			return fmt.Sprintf("builtin_regex_replace(%s)", argStr)
		case "rand":
			return fmt.Sprintf("builtin_rand(%s)", argStr)
		case "uuid":
			return fmt.Sprintf("builtin_uuid(%s)", argStr)
		case "cuid2":
			return fmt.Sprintf("builtin_cuid2(%s)", argStr)
		case "abs":
			return fmt.Sprintf("builtin_abs(%s)", argStr)
		case "ceil":
			return fmt.Sprintf("builtin_ceil(%s)", argStr)
		case "floor":
			return fmt.Sprintf("builtin_floor(%s)", argStr)
		case "round":
			return fmt.Sprintf("builtin_round(%s)", argStr)
		case "base64_encode":
			return fmt.Sprintf("builtin_base64_encode(%s)", argStr)
		case "base64_decode":
			return fmt.Sprintf("builtin_base64_decode(%s)", argStr)
		case "url_encode":
			return fmt.Sprintf("builtin_url_encode(%s)", argStr)
		case "url_decode":
			return fmt.Sprintf("builtin_url_decode(%s)", argStr)
		case "hash":
			return fmt.Sprintf("builtin_hash(%s)", argStr)
		case "hmac_hash":
			return fmt.Sprintf("builtin_hmac_hash(%s)", argStr)
		case "hash_password":
			return fmt.Sprintf("builtin_hash_password(%s)", argStr)
		case "verify_password":
			return fmt.Sprintf("builtin_verify_password(%s)", argStr)
		case "validate":
			return fmt.Sprintf("builtin_validate(%s)", argStr)
		case "is_email":
			return fmt.Sprintf("builtin_is_email(%s)", argStr)
		case "is_url":
			return fmt.Sprintf("builtin_is_url(%s)", argStr)
		case "is_uuid":
			return fmt.Sprintf("builtin_is_uuid(%s)", argStr)
		case "is_numeric":
			return fmt.Sprintf("builtin_is_numeric(%s)", argStr)
		case "log":
			if c.inRouteHandler {
				return fmt.Sprintf("dslLog(\"\", _r, %s)", argStr)
			}
			return fmt.Sprintf("dslLog(\"\", nil, %s)", argStr)
		case "log_info":
			if c.inRouteHandler {
				return fmt.Sprintf("dslLog(\"INFO\", _r, %s)", argStr)
			}
			return fmt.Sprintf("dslLog(\"INFO\", nil, %s)", argStr)
		case "log_warn":
			if c.inRouteHandler {
				return fmt.Sprintf("dslLog(\"WARN\", _r, %s)", argStr)
			}
			return fmt.Sprintf("dslLog(\"WARN\", nil, %s)", argStr)
		case "log_error":
			if c.inRouteHandler {
				return fmt.Sprintf("dslLog(\"ERROR\", _r, %s)", argStr)
			}
			return fmt.Sprintf("dslLog(\"ERROR\", nil, %s)", argStr)
		case "map":
			return fmt.Sprintf("builtin_map(%s)", argStr)
		case "filter":
			return fmt.Sprintf("builtin_filter(%s)", argStr)
		case "reduce":
			return fmt.Sprintf("builtin_reduce(%s)", argStr)
		case "find":
			return fmt.Sprintf("builtin_find(%s)", argStr)
		case "some":
			return fmt.Sprintf("builtin_some(%s)", argStr)
		case "every":
			return fmt.Sprintf("builtin_every(%s)", argStr)
		case "count":
			return fmt.Sprintf("builtin_count(%s)", argStr)
		case "pluck":
			return fmt.Sprintf("builtin_pluck(%s)", argStr)
		case "group_by":
			return fmt.Sprintf("builtin_group_by(%s)", argStr)
		case "chunk":
			return fmt.Sprintf("builtin_chunk(%s)", argStr)
		case "range":
			return fmt.Sprintf("builtin_range(%s)", argStr)
		case "sum":
			return fmt.Sprintf("builtin_sum(%s)", argStr)
		case "min":
			return fmt.Sprintf("builtin_min(%s)", argStr)
		case "max":
			return fmt.Sprintf("builtin_max(%s)", argStr)
		case "clamp":
			return fmt.Sprintf("builtin_clamp(%s)", argStr)
		case "pad_left":
			return fmt.Sprintf("builtin_pad_left(%s)", argStr)
		case "pad_right":
			return fmt.Sprintf("builtin_pad_right(%s)", argStr)
		case "truncate":
			return fmt.Sprintf("builtin_truncate(%s)", argStr)
		case "capitalize":
			return fmt.Sprintf("builtin_capitalize(%s)", argStr)
		case "date":
			return fmt.Sprintf("builtin_date(%s)", argStr)
		case "date_format":
			return fmt.Sprintf("builtin_date_format(%s)", argStr)
		case "date_parse":
			return fmt.Sprintf("builtin_date_parse(%s)", argStr)
		case "strtotime":
			return fmt.Sprintf("builtin_strtotime(%s)", argStr)
		case "redirect":
			return fmt.Sprintf("builtin_redirect(%s)", argStr)
		}

		// User-defined function
		for _, fn := range c.functions {
			if fn.Name == ident.Value {
				return fmt.Sprintf("fn_%s(%s)", safeIdent(ident.Value), argStr)
			}
		}
		// Unknown - might be a variable holding a function
		if argStr == "" {
			return fmt.Sprintf("callValue(%s)", safeIdent(ident.Value))
		}
		return fmt.Sprintf("callValue(%s, %s)", safeIdent(ident.Value), argStr)
	}

	// General case: expression call (e.g. fn(x){ ... }(arg))
	fnExpr := c.expr(e.Function)
	if argStr == "" {
		return fmt.Sprintf("callValue(%s)", fnExpr)
	}
	return fmt.Sprintf("callValue(%s, %s)", fnExpr, argStr)
}

func (c *NativeCompiler) dotExpr(e *DotExpression) string {
	// json and store are handled at call sites
	if ident, ok := e.Left.(*Identifier); ok {
		if ident.Value == "json" || ident.Value == "store" || ident.Value == "file" || ident.Value == "db" || ident.Value == "jwt" || ident.Value == "sse" {
			// These are namespace objects; the actual call is handled in callExpr
			return fmt.Sprintf("dotValue(%s, %q)", safeIdent(ident.Value), e.Field)
		}
		// stream.id in SSE route — return the stream's UUID
		if ident.Value == "stream" && e.Field == "id" && c.inSSERoute {
			return "_sseStreamID(stream)"
		}
		if ident.Value == "stream" {
			// Other stream dot accesses handled in callExpr
			return fmt.Sprintf("dotValue(%s, %q)", safeIdent(ident.Value), e.Field)
		}
	}
	// For SSE handle property access (e.g. handle.id from sse.find())
	if c.hasSSE && e.Field == "id" {
		return fmt.Sprintf("_sseDotValue(%s, %q)", c.expr(e.Left), e.Field)
	}
	return fmt.Sprintf("dotValue(%s, %q)", c.expr(e.Left), e.Field)
}
