package goemit

func (c *NativeCompiler) collectVars(block *BlockStatement) map[string]bool {
	vars := make(map[string]bool)
	c.collectVarsFromBlock(block, vars)
	return vars
}

// collectRefs finds all identifier names referenced (read) in a block
func (c *NativeCompiler) collectRefs(block *BlockStatement) map[string]bool {
	refs := make(map[string]bool)
	c.collectRefsFromBlock(block, refs)
	return refs
}

func (c *NativeCompiler) collectRefsFromBlock(block *BlockStatement, refs map[string]bool) {
	if block == nil {
		return
	}
	for _, stmt := range block.Statements {
		c.collectRefsFromStmt(stmt, refs)
	}
}

func (c *NativeCompiler) collectRefsFromStmt(stmt Statement, refs map[string]bool) {
	if stmt == nil {
		return
	}
	switch s := stmt.(type) {
	case *AssignStatement:
		for _, v := range s.Values {
			c.collectRefsFromExpr(v, refs)
		}
	case *CompoundAssignStatement:
		refs[s.Name] = true
		c.collectRefsFromExpr(s.Value, refs)
	case *ExpressionStatement:
		c.collectRefsFromExpr(s.Expression, refs)
	case *ReturnStatement:
		for _, v := range s.Values {
			c.collectRefsFromExpr(v, refs)
		}
	case *IfStatement:
		c.collectRefsFromExpr(s.Condition, refs)
		c.collectRefsFromBlock(s.Consequence, refs)
		if alt, ok := s.Alternative.(*BlockStatement); ok {
			c.collectRefsFromBlock(alt, refs)
		}
		if alt, ok := s.Alternative.(*IfStatement); ok {
			c.collectRefsFromStmt(alt, refs)
		}
	case *WhileStatement:
		c.collectRefsFromExpr(s.Condition, refs)
		c.collectRefsFromBlock(s.Body, refs)
	case *EachStatement:
		c.collectRefsFromExpr(s.Iterable, refs)
		c.collectRefsFromBlock(s.Body, refs)
	case *SwitchStatement:
		c.collectRefsFromExpr(s.Subject, refs)
		for _, cs := range s.Cases {
			c.collectRefsFromBlock(cs.Body, refs)
		}
		if s.Default != nil {
			c.collectRefsFromBlock(s.Default, refs)
		}
	case *BlockStatement:
		c.collectRefsFromBlock(s, refs)
	case *TryCatchStatement:
		c.collectRefsFromBlock(s.Try, refs)
		c.collectRefsFromBlock(s.Catch, refs)
	case *ThrowStatement:
		c.collectRefsFromExpr(s.Value, refs)
	case *IndexAssignStatement:
		c.collectRefsFromExpr(s.Left, refs)
		c.collectRefsFromExpr(s.Index, refs)
		c.collectRefsFromExpr(s.Value, refs)
	}
}

func (c *NativeCompiler) collectRefsFromExpr(expr Expression, refs map[string]bool) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *Identifier:
		refs[e.Value] = true
	case *InfixExpression:
		c.collectRefsFromExpr(e.Left, refs)
		c.collectRefsFromExpr(e.Right, refs)
	case *PrefixExpression:
		c.collectRefsFromExpr(e.Right, refs)
	case *CallExpression:
		c.collectRefsFromExpr(e.Function, refs)
		for _, a := range e.Arguments {
			c.collectRefsFromExpr(a, refs)
		}
	case *DotExpression:
		c.collectRefsFromExpr(e.Left, refs)
	case *IndexExpression:
		c.collectRefsFromExpr(e.Left, refs)
		c.collectRefsFromExpr(e.Index, refs)
	case *ArrayLiteral:
		for _, el := range e.Elements {
			c.collectRefsFromExpr(el, refs)
		}
	case *HashLiteral:
		for _, p := range e.Pairs {
			c.collectRefsFromExpr(p.Key, refs)
			c.collectRefsFromExpr(p.Value, refs)
		}
	case *TernaryExpression:
		c.collectRefsFromExpr(e.Condition, refs)
		c.collectRefsFromExpr(e.Consequence, refs)
		c.collectRefsFromExpr(e.Alternative, refs)
	}
}

func (c *NativeCompiler) collectVarsFromBlock(block *BlockStatement, vars map[string]bool) {
	for _, stmt := range block.Statements {
		c.collectVarsFromStmt(stmt, vars)
	}
}

func (c *NativeCompiler) collectVarsFromStmt(stmt Statement, vars map[string]bool) {
	switch s := stmt.(type) {
	case *AssignStatement:
		for _, n := range s.Names {
			vars[n] = true
		}
	case *CompoundAssignStatement:
		vars[s.Name] = true
	case *EachStatement:
		vars[s.Value] = true
		if s.Index != "" {
			vars[s.Index] = true
		}
		c.collectVarsFromBlock(s.Body, vars)
	case *IfStatement:
		c.collectVarsFromBlock(s.Consequence, vars)
		if s.Alternative != nil {
			switch alt := s.Alternative.(type) {
			case *BlockStatement:
				c.collectVarsFromBlock(alt, vars)
			case *IfStatement:
				c.collectVarsFromStmt(alt, vars)
			}
		}
	case *SwitchStatement:
		for _, cs := range s.Cases {
			c.collectVarsFromBlock(cs.Body, vars)
		}
		if s.Default != nil {
			c.collectVarsFromBlock(s.Default, vars)
		}
	case *WhileStatement:
		c.collectVarsFromBlock(s.Body, vars)
	case *BlockStatement:
		c.collectVarsFromBlock(s, vars)
	case *FnStatement:
		vars[s.Name] = true
		c.collectVarsFromBlock(s.Body, vars)
	case *TryCatchStatement:
		// Hoist both try and catch block vars so Go closures capture them.
		c.collectVarsFromBlock(s.Try, vars)
		c.collectVarsFromBlock(s.Catch, vars)
	case *ObjectDestructureStatement:
		for _, key := range s.Keys {
			vars[key] = true
		}
	case *ArrayDestructureStatement:
		for _, name := range s.Names {
			vars[name] = true
		}
	}
}
func (c *NativeCompiler) emitSSERoute(route *RouteStatement) {
	c.inSSERoute = true
	defer func() { c.inSSERoute = false }()
	c.lnf(`rt.add("GET", %q, func(_w http.ResponseWriter, _r *http.Request) {`, route.Path)
	c.indent++

	// SSE headers
	c.ln(`_flusher, _ok := _w.(http.Flusher)`)
	c.ln(`if !_ok { http.Error(_w, "SSE not supported", http.StatusInternalServerError); return }`)
	c.ln(`_w.Header().Set("Content-Type", "text/event-stream")`)
	c.ln(`_w.Header().Set("Cache-Control", "no-cache")`)
	c.ln(`_w.Header().Set("Connection", "keep-alive")`)
	if c.corsOrigins != "" {
		c.lnf(`_w.Header().Set("Access-Control-Allow-Origin", %q)`, c.corsOrigins)
	}

	// Create SSE stream handle
	c.ln(`_stream := _sseHub.register()`)
	c.ln(`stream := Value(_stream)`)
	c.ln(`_ = stream`)
	c.ln(`defer func() {`)
	c.indent++
	// Disconnect block runs AFTER channel removal but BEFORE metadata cleanup
	if route.DisconnectBlock != nil {
		// Collect and declare disconnect block variables
		disconnectVars := c.collectVars(route.DisconnectBlock)
		for name := range disconnectVars {
			if name != "request" && name != "stream" && !c.globalVars[name] {
				c.lnf("var %s Value = null", safeIdent(name))
			}
		}
		for name := range disconnectVars {
			if name != "request" && name != "stream" && !c.globalVars[name] {
				c.lnf("_ = %s", safeIdent(name))
			}
		}
		c.emitBlock(route.DisconnectBlock, true)
	}
	c.ln(`_sseHub.unregister(_stream.stream)`)
	c.indent--
	c.ln(`}()`)
	c.ln(``)

	// Parse path params, query, headers for request object
	c.ln(`_pathParams := getParams(_r)`)
	c.ln(`_queryMap := make(map[string]Value)`)
	c.ln(`for _k, _v := range _r.URL.Query() {`)
	c.indent++
	c.ln(`if len(_v) == 1 { _queryMap[_k] = Value(_v[0]) } else {`)
	c.indent++
	c.ln(`_arr := make([]Value, len(_v))`)
	c.ln(`for _i, _s := range _v { _arr[_i] = Value(_s) }`)
	c.ln(`_queryMap[_k] = Value(_arr)`)
	c.indent--
	c.ln(`}`)
	c.indent--
	c.ln(`}`)
	c.ln(`_reqHeaders := make(map[string]Value)`)
	c.ln(`for _k, _v := range _r.Header {`)
	c.indent++
	c.ln(`if len(_v) > 0 { _reqHeaders[strings.ToLower(_k)] = Value(_v[0]) }`)
	c.indent--
	c.ln(`}`)
	c.ln(`_reqCookies := make(map[string]Value)`)
	c.ln(`for _, _c := range _r.Cookies() { _reqCookies[_c.Name] = Value(_c.Value) }`)

	// Bearer/basic auth
	c.ln(`var _bearer Value = Value("")`)
	c.ln(`var _basic Value = null`)
	c.ln(`if _authH := _r.Header.Get("Authorization"); _authH != "" {`)
	c.indent++
	c.ln(`if strings.HasPrefix(_authH, "Bearer ") { _bearer = Value(_authH[7:]) }`)
	c.ln(`if strings.HasPrefix(_authH, "Basic ") {`)
	c.indent++
	c.ln(`if _decoded, _err := base64.StdEncoding.DecodeString(_authH[6:]); _err == nil {`)
	c.indent++
	c.ln(`if _idx := strings.IndexByte(string(_decoded), ':'); _idx >= 0 {`)
	c.indent++
	c.ln(`_basic = Value(map[string]Value{"username": Value(string(_decoded[:_idx])), "password": Value(string(_decoded[_idx+1:]))})`)
	c.indent--
	c.ln(`}`)
	c.indent--
	c.ln(`}`)
	c.indent--
	c.ln(`}`)
	c.indent--
	c.ln(`}`)

	// Session loading (for SSE auth)
	if c.sessionEnabled {
		c.ln(`_sessID, _sessData, _ := sessionLoad(_r)`)
		c.ln(`_sessDestroyed := false`)
		c.ln(`_ = _sessID`)
		c.ln(`_ = _sessDestroyed`)
	}

	// Build request object
	c.ln(`request := Value(map[string]Value{`)
	c.indent++
	c.ln(`"method":  Value("SSE"),`)
	c.ln(`"path":    Value(_r.URL.Path),`)
	c.ln(`"params":  Value(_pathParams),`)
	c.ln(`"query":   Value(_queryMap),`)
	c.ln(`"headers": Value(_reqHeaders),`)
	c.ln(`"cookies": Value(_reqCookies),`)
	c.ln(`"bearer":  _bearer,`)
	c.ln(`"basic":   _basic,`)
	if c.sessionEnabled {
		c.ln(`"session": Value(_sessData),`)
	}
	c.indent--
	c.ln(`})`)
	c.ln(`_ = request`)
	c.ln(``)

	// Declare user variables
	vars := c.collectVars(route.Body)
	for name := range vars {
		if name != "request" && name != "stream" && !c.globalVars[name] {
			c.lnf("var %s Value = null", safeIdent(name))
		}
	}

	// Run route body (initial events, auth, joins)
	c.emitBlock(route.Body, true)
	c.ln(``)

	// Heartbeat to keep connection alive through proxies
	c.ln(`_heartbeat := time.NewTicker(30 * time.Second)`)
	c.ln(`defer _heartbeat.Stop()`)
	c.ln(``)

	// Event loop: block on stream channel, heartbeat, server-side close, or client disconnect
	c.ln(`for {`)
	c.indent++
	c.ln(`select {`)
	c.ln(`case _evt, _ok := <-_stream.stream.ch:`)
	c.indent++
	c.ln(`if !_ok { return }`)
	c.ln(`if _evt.event != "" {`)
	c.indent++
	c.ln(`fmt.Fprintf(_w, "event: %s\n", _evt.event)`)
	c.indent--
	c.ln(`}`)
	c.ln(`fmt.Fprintf(_w, "data: %s\n\n", _evt.data)`)
	c.ln(`_flusher.Flush()`)
	c.indent--
	c.ln(`case <-_heartbeat.C:`)
	c.indent++
	c.ln(`fmt.Fprintf(_w, ": keepalive\n\n")`)
	c.ln(`_flusher.Flush()`)
	c.indent--
	c.ln(`case <-_stream.stream.done:`)
	c.indent++
	c.ln(`return`)
	c.indent--
	c.ln(`case <-_r.Context().Done():`)
	c.indent++
	c.ln(`return`)
	c.indent--
	c.ln(`}`)
	c.indent--
	c.ln(`}`)

	c.indent--
	c.ln(`})`)
	c.ln(``)
}

func (c *NativeCompiler) emitRoute(route *RouteStatement) {
	c.lnf("rt.add(%q, %q, func(_w http.ResponseWriter, _r *http.Request) {", route.Method, route.Path)
	c.indent++
	c.inRouteHandler = true
	defer func() { c.inRouteHandler = false }()

	// Request timeout
	timeout := route.Timeout
	if timeout == 0 {
		timeout = c.defaultTimeout
	}
	if timeout > 0 {
		c.lnf("_ctx, _cancel := context.WithTimeout(_r.Context(), %d*time.Second)", timeout)
		c.ln("defer _cancel()")
		c.ln("_r = _r.WithContext(_ctx)")
	}

	// Handle redirects
	c.ln("defer func() {")
	c.indent++
	c.ln("if _rv := recover(); _rv != nil {")
	c.indent++
	c.ln("if _rs, ok := _rv.(*redirectSignal); ok {")
	c.indent++
	c.ln("http.Redirect(_w, _r, _rs.url, _rs.status)")
	c.indent--
	c.ln("} else { panic(_rv) }")
	c.indent--
	c.ln("}")
	c.indent--
	c.ln("}()")

	// Read request basics
	c.ln("_pathParams := getParams(_r)")
	c.ln("_bodyBytes, _ := io.ReadAll(_r.Body)")
	c.ln("defer _r.Body.Close()")

	// Parse query params
	c.ln("_queryMap := make(map[string]Value)")
	c.ln("for _k, _v := range _r.URL.Query() {")
	c.indent++
	c.ln("if len(_v) == 1 { _queryMap[_k] = Value(_v[0]) } else {")
	c.indent++
	c.ln("_arr := make([]Value, len(_v))")
	c.ln("for _i, _s := range _v { _arr[_i] = Value(_s) }")
	c.ln("_queryMap[_k] = Value(_arr)")
	c.indent--
	c.ln("}")
	c.indent--
	c.ln("}")

	// Parse request headers
	c.ln("_reqHeaders := make(map[string]Value)")
	c.ln("for _k, _v := range _r.Header {")
	c.indent++
	c.ln("_hk := strings.ToLower(_k)")
	c.ln("if len(_v) == 1 { _reqHeaders[_hk] = Value(_v[0]) } else {")
	c.indent++
	c.ln("_arr := make([]Value, len(_v))")
	c.ln("for _i, _s := range _v { _arr[_i] = Value(_s) }")
	c.ln("_reqHeaders[_hk] = Value(_arr)")
	c.indent--
	c.ln("}")
	c.indent--
	c.ln("}")

	// Parse request cookies
	c.ln("_reqCookies := make(map[string]Value)")
	c.ln("for _, _c := range _r.Cookies() {")
	c.indent++
	c.ln("_reqCookies[_c.Name] = Value(_c.Value)")
	c.indent--
	c.ln("}")

	// Client IP
	c.ln(`_clientIP := _r.RemoteAddr`)
	c.ln(`if _xff := _r.Header.Get("X-Forwarded-For"); _xff != "" { _clientIP = strings.Split(_xff, ",")[0] }`)

	// Content type and body parsing
	c.ln("_contentType := _r.Header.Get(\"Content-Type\")")
	c.ln("var _reqData Value = null")
	c.ln("_bodyStr := string(_bodyBytes)")
	c.ln("var _reqFiles Value = Value([]Value{})")

	typeCheck := route.TypeCheck
	hasTypeCheck := typeCheck != ""

	if !hasTypeCheck {
		// Auto-detect content type
		c.ln("if strings.Contains(_contentType, \"application/json\") {")
		c.indent++
		c.ln("_reqData = parseJSONBody(_bodyStr)")
		c.indent--
		c.ln("} else if strings.Contains(_contentType, \"application/x-www-form-urlencoded\") {")
		c.indent++
		c.ln("_reqData = parseFormBody(_bodyStr)")
		c.indent--
		c.ln("} else if strings.Contains(_contentType, \"multipart/form-data\") {")
		c.indent++
		c.ln("_r.Body = io.NopCloser(bytes.NewReader(_bodyBytes))")
		c.ln("_reqData, _reqFiles = parseMultipartBody(_r)")
		c.indent--
		c.ln("} else if len(_bodyBytes) > 0 {")
		c.indent++
		c.ln("_reqData = parseJSONBody(_bodyStr)")
		c.ln("if _reqData == null { _reqData = Value(_bodyStr) }")
		c.indent--
		c.ln("}")
	} else {
		c.lnf("// Enforced type: %s", typeCheck)
		c.ln("var _typeError Value = null")
		switch typeCheck {
		case "json":
			c.ln("if strings.Contains(_contentType, \"application/json\") || _contentType == \"\" {")
			c.indent++
			c.ln("if len(_bodyBytes) > 0 {")
			c.indent++
			c.ln("_reqData = parseJSONBody(_bodyStr)")
			c.ln("if _reqData == null { _typeError = Value(\"invalid JSON body\") }")
			c.indent--
			c.ln("}")
			c.indent--
			c.ln("} else {")
			c.indent++
			c.ln("_typeError = Value(\"expected Content-Type application/json, got \" + _contentType)")
			c.indent--
			c.ln("}")
		case "text":
			c.ln("_reqData = Value(_bodyStr)")
		case "form":
			c.ln("if strings.Contains(_contentType, \"application/x-www-form-urlencoded\") {")
			c.indent++
			c.ln("_reqData = parseFormBody(_bodyStr)")
			c.indent--
			c.ln("} else if strings.Contains(_contentType, \"multipart/form-data\") {")
			c.indent++
			c.ln("_r.Body = io.NopCloser(bytes.NewReader(_bodyBytes))")
			c.ln("_reqData, _reqFiles = parseMultipartBody(_r)")
			c.indent--
			c.ln("} else {")
			c.indent++
			c.ln("_typeError = Value(\"expected form data, got \" + _contentType)")
			c.indent--
			c.ln("}")
		}
	}

	// Session loading
	if c.sessionEnabled {
		c.ln("_sessID, _sessData, _ := sessionLoad(_r)")
		c.ln("_sessDestroyed := false")
	}

	// CSRF validation
	if c.csrfEnabled && !route.CSRFDisabled {
		c.ln(`// CSRF protection`)
		c.ln(`if !csrfValidate(_r, _sessData, _bodyStr) {`)
		c.indent++
		c.ln(`_w.Header().Set("Content-Type", "application/json")`)
		c.ln(`_w.WriteHeader(403)`)
		c.ln(`_w.Write([]byte("{\"error\":\"CSRF token mismatch\"}"))`)
		c.ln(`return`)
		c.indent--
		c.ln(`}`)
	}

	// Parse Authorization header
	c.ln(`var _bearer Value = Value("")`)
	c.ln(`var _basic Value = null`)
	c.ln(`if _authH := _r.Header.Get("Authorization"); _authH != "" {`)
	c.indent++
	c.ln(`if strings.HasPrefix(_authH, "Bearer ") { _bearer = Value(_authH[7:]) }`)
	c.ln(`if strings.HasPrefix(_authH, "Basic ") {`)
	c.indent++
	c.ln(`if _decoded, _err := base64.StdEncoding.DecodeString(_authH[6:]); _err == nil {`)
	c.indent++
	c.ln(`if _idx := strings.IndexByte(string(_decoded), ':'); _idx >= 0 {`)
	c.indent++
	c.ln(`_basic = Value(map[string]Value{"username": Value(string(_decoded[:_idx])), "password": Value(string(_decoded[_idx+1:]))})`)
	c.indent--
	c.ln(`}`)
	c.indent--
	c.ln(`}`)
	c.indent--
	c.ln(`}`)
	c.indent--
	c.ln(`}`)

	// Build request object
	c.ln("request := Value(map[string]Value{")
	c.indent++
	c.ln("\"method\":  Value(_r.Method),")
	c.ln("\"path\":    Value(_r.URL.Path),")
	c.ln("\"body\":    Value(_bodyStr),")
	c.ln("\"data\":    _reqData,")
	c.ln("\"params\":  Value(_pathParams),")
	c.ln("\"query\":   Value(_queryMap),")
	c.ln("\"headers\": Value(_reqHeaders),")
	c.ln("\"cookies\": Value(_reqCookies),")
	c.ln("\"ip\":      Value(_clientIP),")
	c.ln("\"files\":   _reqFiles,")
	c.ln("\"bearer\":  _bearer,")
	c.ln("\"basic\":   _basic,")
	if c.sessionEnabled {
		c.ln("\"session\": Value(_sessData),")
	}
	c.indent--
	c.ln("})")

	// Build response object
	c.ln("response := Value(map[string]Value{")
	c.indent++
	c.ln("\"status\":  Value(int64(200)),")
	c.ln("\"type\":    Value(\"json\"),")
	c.ln("\"body\":    Value(map[string]Value{}),")
	c.ln("\"headers\": Value(map[string]Value{}),")
	c.ln("\"cookies\": Value(map[string]Value{}),")
	c.indent--
	c.ln("})")

	// Handle type check failure
	if hasTypeCheck {
		if route.ElseBlock != nil {
			c.ln("if _typeError != null {")
			c.indent++
			c.ln("error := _typeError")
			c.ln("_ = error")
			c.ln("_ = request")
			c.ln("_ = response")

			elseVars := c.collectVars(route.ElseBlock)
			for name := range elseVars {
				if name != "request" && name != "response" && name != "error" && !c.globalVars[name] {
					c.lnf("var %s Value = null", safeIdent(name))
				}
			}
			c.emitBlock(route.ElseBlock, true)
			c.ln("writeResponse(_w, response)")
			c.ln("return")
			c.indent--
			c.ln("}")
		} else {
			c.ln("if _typeError != null {")
			c.indent++
			c.ln(`_w.Header().Set("Content-Type", "application/json")`)
			c.ln("_w.WriteHeader(400)")
			c.ln(`json.NewEncoder(_w).Encode(map[string]interface{}{"error": valueToGo(_typeError)})`)
			c.ln("return")
			c.indent--
			c.ln("}")
		}
	}

	// Collect all before/after blocks for this route
	var beforeBlocks []*BlockStatement
	beforeBlocks = append(beforeBlocks, c.globalBefore...)
	beforeBlocks = append(beforeBlocks, c.routeBeforeMap[route]...)
	var afterBlocks []*BlockStatement
	afterBlocks = append(afterBlocks, c.globalAfter...)
	afterBlocks = append(afterBlocks, c.routeAfterMap[route]...)

	// Declare all variables (before + body + after)
	vars := c.collectVars(route.Body)
	for _, bb := range beforeBlocks {
		for k, v := range c.collectVars(bb) {
			vars[k] = v
		}
	}
	for _, ab := range afterBlocks {
		for k, v := range c.collectVars(ab) {
			vars[k] = v
		}
	}
	for name := range vars {
		if name != "request" && name != "response" && !c.globalVars[name] {
			c.lnf("var %s Value = null", safeIdent(name))
		}
	}
	c.ln("_ = request")
	c.ln("_ = response")
	c.ln("")

	// Wrap before + body in a closure so route-level return exits this closure
	// and still flows through session save + single response write.
	c.ln("func() {")
	c.indent++
	for _, bb := range beforeBlocks {
		c.emitBlock(bb, true)
	}

	// Wrap route body in recover for throw — catch goes to else block
	if route.ElseBlock != nil {
		c.ln("func() {")
		c.indent++
		c.ln("defer func() {")
		c.indent++
		c.ln("if _r := recover(); _r != nil {")
		c.indent++
		c.ln("if _tv, ok := _r.(*throwValue); ok {")
		c.indent++
		c.ln("error := _tv.value")
		c.ln("_ = error")
		elseVars := c.collectVars(route.ElseBlock)
		for name := range elseVars {
			if name != "request" && name != "response" && name != "error" && !c.globalVars[name] {
				c.lnf("var %s Value = null", safeIdent(name))
			}
		}
		c.emitBlock(route.ElseBlock, true)
		c.indent--
		c.ln("} else { panic(_r) }")
		c.indent--
		c.ln("}")
		c.indent--
		c.ln("}()")
	}

	// Emit route body
	c.emitBlock(route.Body, true)

	if route.ElseBlock != nil {
		c.indent--
		c.ln("}()")
	}

	c.indent--
	c.ln("}()")

	// Session save
	if c.sessionEnabled {
		c.ln("// Save session")
		c.ln("if _rm, ok := request.(map[string]Value); ok {")
		c.indent++
		c.ln("if _sd, ok := _rm[\"session\"].(map[string]Value); ok {")
		c.indent++
		c.ln("sessionSave(_w, _sessID, _sd, _sessDestroyed)")
		c.indent--
		c.ln("} else if _rm[\"session\"] == null || _rm[\"session\"] == nil {")
		c.indent++
		c.ln("sessionSave(_w, _sessID, nil, true)")
		c.indent--
		c.ln("}")
		c.indent--
		c.ln("}")
	}

	// Auto-return response
	timeoutActive := route.Timeout > 0 || c.defaultTimeout > 0
	if timeoutActive {
		c.ln("if _r.Context().Err() == context.DeadlineExceeded {")
		c.indent++
		c.ln(`_w.Header().Set("Content-Type", "application/json")`)
		c.ln("_w.WriteHeader(504)")
		c.ln(`_w.Write([]byte("{\"error\":\"request timeout\"}"))`)
		c.indent--
		c.ln("} else {")
		c.indent++
		c.ln("writeResponse(_w, response)")
		c.indent--
		c.ln("}")
	} else {
		c.ln("writeResponse(_w, response)")
	}

	// After blocks — fire and forget goroutine
	if len(afterBlocks) > 0 {
		c.ln("go func(_afterReq, _afterResp Value) {")
		c.indent++
		c.ln("defer func() { recover() }() // never crash from after block")
		// Shadow request/response with copies
		c.ln("request := _afterReq")
		c.ln("response := _afterResp")
		c.ln("_ = request")
		c.ln("_ = response")
		for _, ab := range afterBlocks {
			c.emitBlock(ab, true)
		}
		c.indent--
		c.ln("}(request, response)")
	}

	c.indent--
	c.ln("})")
	c.ln("")
}

func (c *NativeCompiler) emitErrorHandler(eh *ErrorStatement) {
	c.lnf("rt.errorHandlers[%d] = func(_w http.ResponseWriter, _r *http.Request) {", eh.StatusCode)
	c.indent++
	c.inRouteHandler = true
	defer func() { c.inRouteHandler = false }()

	// Build minimal request object (no body parsing needed for error pages)
	c.ln("_pathParams := make(map[string]Value)")
	c.ln("_queryMap := make(map[string]Value)")
	c.ln("for _k, _v := range _r.URL.Query() {")
	c.indent++
	c.ln("if len(_v) == 1 { _queryMap[_k] = Value(_v[0]) } else {")
	c.indent++
	c.ln("_arr := make([]Value, len(_v))")
	c.ln("for _i, _s := range _v { _arr[_i] = Value(_s) }")
	c.ln("_queryMap[_k] = Value(_arr)")
	c.indent--
	c.ln("}")
	c.indent--
	c.ln("}")

	c.ln("_reqHeaders := make(map[string]Value)")
	c.ln("for _k, _v := range _r.Header { _reqHeaders[strings.ToLower(_k)] = Value(_v[0]) }")

	c.ln("request := Value(map[string]Value{")
	c.indent++
	c.ln(`"method":  Value(_r.Method),`)
	c.ln(`"path":    Value(_r.URL.Path),`)
	c.ln(`"params":  Value(_pathParams),`)
	c.ln(`"query":   Value(_queryMap),`)
	c.ln(`"headers": Value(_reqHeaders),`)
	c.indent--
	c.ln("})")

	// Build response object with the error status code as default
	c.ln("response := Value(map[string]Value{")
	c.indent++
	c.lnf(`"status":  Value(int64(%d)),`, eh.StatusCode)
	c.ln(`"type":    Value("json"),`)
	c.ln(`"body":    Value(map[string]Value{}),`)
	c.ln(`"headers": Value(map[string]Value{}),`)
	c.ln(`"cookies": Value(map[string]Value{}),`)
	c.indent--
	c.ln("})")

	// Declare variables
	vars := c.collectVars(eh.Body)
	for name := range vars {
		if name != "request" && name != "response" && !c.globalVars[name] {
			c.lnf("var %s Value = null", safeIdent(name))
		}
	}

	c.ln("_ = request")

	// Emit body
	c.emitBlock(eh.Body, true)

	// Write response
	c.ln("writeResponse(_w, response)")

	c.indent--
	c.ln("}")
	c.ln("")
}
