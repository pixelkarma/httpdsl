package goemit

import (
	"fmt"
	"strconv"
	"strings"
)

func (c *NativeCompiler) emitMain() {
	c.ln("// ===== Main =====")
	c.ln("func runGeneratedMain() {")
	c.indent++

	// CLI flags and env loading
	c.ln("// CLI flags")
	c.ln(`_envFile := ".env"`)
	c.ln("_showHelp := false")
	c.ln("_showVersion := false")
	c.ln(`_portFlag := ""`)
	c.ln(`_autocertFlag := ""`)
	c.ln(`_autocertDirFlag := ""`)
	if len(c.staticMounts) > 0 {
		c.ln(`_staticFlag := ""`)
	}
	c.ln("{")
	c.indent++
	c.ln("i := 1")
	c.ln("for i < len(os.Args) {")
	c.indent++
	c.ln("a := os.Args[i]")
	c.ln(`switch a {`)
	c.ln(`case "-h":`)
	c.indent++
	c.ln("_showHelp = true")
	c.ln("i++")
	c.indent--
	c.ln(`case "-v":`)
	c.indent++
	c.ln("_showVersion = true")
	c.ln("i++")
	c.indent--
	c.ln(`case "-e":`)
	c.indent++
	c.ln("if i+1 < len(os.Args) { _envFile = os.Args[i+1]; i += 2 } else { i++ }")
	c.indent--
	c.ln(`case "-p":`)
	c.indent++
	c.ln("if i+1 < len(os.Args) { _portFlag = os.Args[i+1]; i += 2 } else { i++ }")
	c.indent--
	if len(c.staticMounts) > 0 {
		c.ln(`case "-s":`)
		c.indent++
		c.ln("if i+1 < len(os.Args) { _staticFlag = os.Args[i+1]; i += 2 } else { i++ }")
		c.indent--
	}
	c.ln(`case "-a":`)
	c.indent++
	c.ln(`if i+1 < len(os.Args) { _autocertFlag = os.Args[i+1]; i += 2 } else { i++ }`)
	c.indent--
	c.ln(`case "-ad":`)
	c.indent++
	c.ln(`if i+1 < len(os.Args) { _autocertDirFlag = os.Args[i+1]; i += 2 } else { i++ }`)
	c.indent--
	c.ln("default:")
	c.indent++
	c.ln(`if strings.HasPrefix(a, "--") && len(a) > 2 {`)
	c.indent++
	c.ln("key := a[2:]")
	c.ln(`if i+1 < len(os.Args) { _argsMap[key] = Value(os.Args[i+1]); i += 2 } else { _argsMap[key] = Value(true); i++ }`)
	c.indent--
	c.ln("} else { i++ }")
	c.indent--
	c.ln("}") // end switch
	c.indent--
	c.ln("}") // end for
	c.indent--
	c.ln("}") // end block scope
	c.ln("")

	// Handle -v
	c.ln("if _showVersion {")
	c.indent++
	c.ln(`fmt.Println("Built with httpdsl")`)
	c.ln("os.Exit(0)")
	c.indent--
	c.ln("}")
	c.ln("")

	// Handle -h
	c.ln("if _showHelp {")
	c.indent++
	if c.helpText != "" {
		c.lnf("fmt.Println(%s)", strconv.Quote(c.helpText))
		c.ln(`fmt.Println("")`)
	}
	c.ln(`fmt.Println("Flags:")`)
	c.lnf(`fmt.Println("  -p <port>   Listen port (default: %d, or PORT env var)")`, c.port)
	if len(c.staticMounts) > 0 {
		c.lnf("fmt.Println(\"  -s <dir>    Static file directory (default: %s)\")", c.staticMounts[0].Dir)
	}
	c.ln(`fmt.Println("  -e <path>   Load env file (default: .env, \"none\" to skip)")`)
	c.ln(`fmt.Println("  -a <domain> Let's Encrypt autocert for domain")`)
	c.ln(`fmt.Println("  -ad <dir>   Autocert cache directory (default: .autocert)")`)
	c.ln(`fmt.Println("  -v          Show version")`)
	c.ln(`fmt.Println("  -h          Show this help")`)
	c.ln(`fmt.Println("")`)
	c.ln(`fmt.Println("Environment variables:")`)
	c.ln(`fmt.Println("  PORT             Override listen port")`)
	c.ln(`fmt.Println("  SSL_CERT         Path to TLS certificate file")`)
	c.ln(`fmt.Println("  SSL_KEY          Path to TLS private key file")`)
	c.ln(`fmt.Println("  AUTOCERT_DOMAIN  Enable Let's Encrypt for domain")`)
	c.ln(`fmt.Println("  AUTOCERT_DIR     Autocert cache directory")`)
	c.ln(`fmt.Println("  HTTPS_REDIRECT   Redirect HTTP to HTTPS (true/false, default: true when TLS active)")`)
	c.ln(`fmt.Println("  WWW_REDIRECT     Redirect non-www to www (true/false, default: false)")`)

	c.ln("os.Exit(0)")
	c.indent--
	c.ln("}")
	c.ln("")

	// Load .env file
	c.ln(`if _envFile != "none" { _loadEnvFile(_envFile) }`)
	c.ln("")

	// Run init blocks
	if len(c.initBlocks) > 0 {
		c.ln("// init blocks")
		for _, block := range c.initBlocks {
			c.emitBlock(block, false)
		}
		c.ln("")
	}

	if len(c.templateFiles) > 0 {
		c.ln("_initTemplates()")
		c.ln("")
	}
	c.ln("rt := &dslRouter{errorHandlers: make(map[int]http.HandlerFunc)}")
	if c.throttleRPS > 0 {
		c.lnf("rt.limiter = newRateLimiter(%d)", c.throttleRPS)
	}
	if len(c.staticMounts) > 0 {
		// Static directory override: -s flag → compiled default
		for i, sm := range c.staticMounts {
			prefix := sm.Prefix
			if !strings.HasSuffix(prefix, "/") {
				prefix += "/"
			}
			if i == 0 {
				// First mount can be overridden with -s flag
				c.lnf(`_staticDir := %q`, sm.Dir)
				c.ln(`if _staticFlag != "" { _staticDir = _staticFlag }`)
				c.lnf("rt.statics = append(rt.statics, staticMountEntry{")
				c.indent++
				c.lnf("prefix: %q,", prefix)
				c.lnf("handler: http.StripPrefix(%q, http.FileServer(noDirFS{http.Dir(filepath.Clean(_staticDir))})),", prefix)
				c.indent--
				c.ln("})")
			} else {
				c.lnf("rt.statics = append(rt.statics, staticMountEntry{")
				c.indent++
				c.lnf("prefix: %q,", prefix)
				c.lnf("handler: http.StripPrefix(%q, http.FileServer(noDirFS{http.Dir(filepath.Clean(%q))})),", prefix, sm.Dir)
				c.indent--
				c.ln("})")
			}
		}
	}
	c.ln("")

	for _, route := range c.routes {
		if route.Method == "SSE" {
			c.emitSSERoute(route)
		} else {
			c.emitRoute(route)
		}
	}

	for _, eh := range c.errorHandlers {
		c.emitErrorHandler(eh)
	}

	// Scheduled tasks
	for i, ev := range c.everyBlocks {
		vars := c.collectVars(ev.Body)
		if ev.CronExpr != "" {
			// Cron expression
			c.lnf("cronRun(%q, func() { // cron task %d", ev.CronExpr, i)
			c.indent++
			c.ln("defer func() { if r := recover(); r != nil { fmt.Fprintf(os.Stderr, \"cron task panic: %v\\n\", r) } }()")
			for name := range vars {
				if !c.globalVars[name] {
					c.lnf("var %s Value = null", safeIdent(name))
				}
			}
			c.emitBlock(ev.Body, false)
			c.indent--
			c.ln("})")
		} else {
			// Interval
			c.lnf("go func() { // every %ds", ev.Interval)
			c.indent++
			c.lnf("_ticker := time.NewTicker(%d * time.Second)", ev.Interval)
			c.ln("defer _ticker.Stop()")
			c.ln("for range _ticker.C {")
			c.indent++
			c.lnf("func() { // task %d", i)
			c.indent++
			c.ln("defer func() { if r := recover(); r != nil { fmt.Fprintf(os.Stderr, \"scheduled task panic: %v\\n\", r) } }()")
			for name := range vars {
				if !c.globalVars[name] {
					c.lnf("var %s Value = null", safeIdent(name))
				}
			}
			c.emitBlock(ev.Body, false)
			c.indent--
			c.ln("}()")
			c.indent--
			c.ln("}")
			c.indent--
			c.ln("}()")
		}
	}

	// Port resolution: -p flag → PORT env → compiled default
	c.lnf(`_port := "%d"`, c.port)
	c.ln(`if _envPort := os.Getenv("PORT"); _envPort != "" { _port = _envPort }`)
	c.ln(`if _portFlag != "" { _port = _portFlag }`)
	c.ln(`addr := ":" + _port`)
	c.ln("")

	// SSL resolution: SSL_CERT/SSL_KEY env → compiled default → no TLS
	hasCompiledSSL := (c.sslCert != "" && c.sslKey != "") || (c.sslCertExpr != nil && c.sslKeyExpr != nil)
	if hasCompiledSSL {
		// Compiled default from server block
		certExpr := fmt.Sprintf("%q", c.sslCert)
		if c.sslCertExpr != nil {
			certExpr = fmt.Sprintf("valueToString(%s)", c.expr(c.sslCertExpr))
		}
		keyExpr := fmt.Sprintf("%q", c.sslKey)
		if c.sslKeyExpr != nil {
			keyExpr = fmt.Sprintf("valueToString(%s)", c.expr(c.sslKeyExpr))
		}
		c.lnf("_sslCert := %s", certExpr)
		c.lnf("_sslKey := %s", keyExpr)
	} else {
		c.ln(`_sslCert := ""`)
		c.ln(`_sslKey := ""`)
	}
	c.ln(`if _envCert := os.Getenv("SSL_CERT"); _envCert != "" { _sslCert = _envCert }`)
	c.ln(`if _envKey := os.Getenv("SSL_KEY"); _envKey != "" { _sslKey = _envKey }`)
	c.ln("")

	// Autocert resolution: --autocert flag → AUTOCERT_DOMAIN env → compiled default
	hasCompiledAutocert := c.autocertDomain != "" || c.autocertDomainExpr != nil
	if hasCompiledAutocert {
		domainExpr := fmt.Sprintf("%q", c.autocertDomain)
		if c.autocertDomainExpr != nil {
			domainExpr = fmt.Sprintf("valueToString(%s)", c.expr(c.autocertDomainExpr))
		}
		c.lnf("_autocertDomain := %s", domainExpr)
		dirExpr := fmt.Sprintf("%q", c.autocertDir)
		if c.autocertDirExpr != nil {
			dirExpr = fmt.Sprintf("valueToString(%s)", c.expr(c.autocertDirExpr))
		}
		c.lnf("_autocertDir := %s", dirExpr)
	} else {
		c.ln(`_autocertDomain := ""`)
		c.ln(`_autocertDir := ""`)
	}
	c.ln(`if _envDomain := os.Getenv("AUTOCERT_DOMAIN"); _envDomain != "" { _autocertDomain = _envDomain }`)
	c.ln(`if _envDir := os.Getenv("AUTOCERT_DIR"); _envDir != "" { _autocertDir = _envDir }`)
	c.ln(`if _autocertFlag != "" { _autocertDomain = _autocertFlag }`)
	c.ln(`if _autocertDirFlag != "" { _autocertDir = _autocertDirFlag }`)
	c.ln(`if _autocertDir == "" && _autocertDomain != "" { _autocertDir = ".autocert" }`)
	c.ln("")

	// Redirect settings: compiled default -> env override
	httpsDefault := `""`
	if c.httpsRedirect != "" {
		httpsDefault = fmt.Sprintf("%q", c.httpsRedirect)
	}
	wwwDefault := `"false"`
	if c.wwwRedirect != "" {
		wwwDefault = fmt.Sprintf("%q", c.wwwRedirect)
	}
	c.lnf("_httpsRedirect := %s", httpsDefault)
	c.lnf("_wwwRedirect := %s", wwwDefault)
	c.ln(`if v := os.Getenv("HTTPS_REDIRECT"); v != "" { _httpsRedirect = strings.ToLower(v) }`)
	c.ln(`if v := os.Getenv("WWW_REDIRECT"); v != "" { _wwwRedirect = strings.ToLower(v) }`)
	c.ln(`// Default: https_redirect is true when TLS is active`)
	c.ln(`_doHttpsRedirect := _httpsRedirect == "true" || (_httpsRedirect == "" && (_autocertDomain != "" || (_sslCert != "" && _sslKey != "")))`)
	c.ln(`_doWwwRedirect := _wwwRedirect == "true"`)
	c.ln("")

	// Print startup info
	c.ln(`if _autocertDomain != "" {`)
	c.indent++
	c.ln(`fmt.Printf("httpdsl native server (autocert: %s) on %s\n", _autocertDomain, addr)`)
	c.indent--
	c.ln(`} else if _sslCert != "" && _sslKey != "" {`)
	c.indent++
	c.ln(`fmt.Printf("httpdsl native server (TLS) on %s\n", addr)`)
	c.indent--
	c.ln("} else {")
	c.indent++
	c.ln(`fmt.Printf("httpdsl native server on %s\n", addr)`)
	c.indent--
	c.ln("}")

	if c.corsOrigins != "" {
		c.ln("var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {")
		c.indent++
		c.lnf("w.Header().Set(\"Access-Control-Allow-Origin\", %q)", c.corsOrigins)
		methods := c.corsMethods
		if methods == "" {
			methods = "GET, POST, PUT, PATCH, DELETE, OPTIONS"
		}
		c.lnf("w.Header().Set(\"Access-Control-Allow-Methods\", %q)", methods)
		headers := c.corsHeaders
		if headers == "" {
			headers = "Content-Type, Authorization"
		}
		c.lnf("w.Header().Set(\"Access-Control-Allow-Headers\", %q)", headers)
		c.ln("if r.Method == \"OPTIONS\" { w.WriteHeader(204); return }")
		c.ln("rt.ServeHTTP(w, r)")
		c.indent--
		c.ln("})")
	} else {
		c.ln("var handler http.Handler = rt")
	}
	if c.gzipEnabled {
		c.ln("handler = gzipMiddleware(handler)")
	}

	// www redirect on main handler (HTTPS or plain HTTP)
	c.ln(`if _doWwwRedirect {`)
	c.indent++
	c.ln(`_inner := handler`)
	c.ln(`handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {`)
	c.indent++
	c.ln(`host := r.Host`)
	c.ln(`if i := strings.IndexByte(host, ':'); i >= 0 { host = host[:i] }`)
	c.ln(`if !strings.HasPrefix(host, "www.") {`)
	c.indent++
	c.ln(`scheme := "https"`)
	c.ln(`if r.TLS == nil { scheme = "http" }`)
	c.ln(`http.Redirect(w, r, scheme + "://www." + r.Host + r.RequestURI, 301)`)
	c.ln(`return`)
	c.indent--
	c.ln(`}`)
	c.ln(`_inner.ServeHTTP(w, r)`)
	c.indent--
	c.ln(`})`)
	c.indent--
	c.ln(`}`)
	c.ln("")

	// Graceful shutdown handler
	if len(c.shutdownBlocks) > 0 || c.usedBuiltins["store"] || c.sessionEnabled {
		c.ln("// Graceful shutdown")
		c.ln("go func() {")
		c.indent++
		c.ln("_sigCh := make(chan os.Signal, 1)")
		c.ln("signal.Notify(_sigCh, os.Interrupt, syscall.SIGTERM)")
		c.ln("<-_sigCh")
		c.ln(`fmt.Println("\nShutting down...")`)
		// Emit user shutdown blocks
		if len(c.shutdownBlocks) > 0 {
			for _, block := range c.shutdownBlocks {
				vars := c.collectVars(block)
				for name := range vars {
					if !c.globalVars[name] {
						c.lnf("var %s Value = null", safeIdent(name))
					}
				}
				c.emitBlock(block, false)
			}
		}
		// Flush store if synced
		if c.sessionEnabled {
			c.ln("sessionFlush()")
		}
		if c.usedBuiltins["store"] {
			c.ln("if _storeFlush != nil { _storeFlush() }")
		}
		c.ln("os.Exit(0)")
		c.indent--
		c.ln("}()")
		c.ln("")
	}

	c.ln(`if _autocertDomain != "" {`)
	c.indent++
	c.ln(`_autocertDomains := strings.Split(_autocertDomain, ",")`)
	c.ln(`for i := range _autocertDomains { _autocertDomains[i] = strings.TrimSpace(_autocertDomains[i]) }`)
	c.ln(`fmt.Printf("  autocert domains: %s\n  cache dir: %s\n", strings.Join(_autocertDomains, ", "), _autocertDir)`)
	c.ln(`m := &autocert.Manager{`)
	c.indent++
	c.ln(`Prompt: autocert.AcceptTOS,`)
	c.ln(`HostPolicy: autocert.HostWhitelist(_autocertDomains...),`)
	c.ln(`Cache: autocert.DirCache(_autocertDir),`)
	c.indent--
	c.ln(`}`)
	c.ln(`srv := &http.Server{`)
	c.indent++
	c.ln(`Addr: addr,`)
	c.ln(`Handler: handler,`)
	c.ln(`TLSConfig: &tls.Config{GetCertificate: m.GetCertificate},`)
	c.indent--
	c.ln(`}`)
	c.ln(`// HTTP :80 — ACME challenges + redirect`)
	c.ln(`_acmeHandler := m.HTTPHandler(nil)`)
	c.ln(`go func() {`)
	c.indent++
	c.ln(`httpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {`)
	c.indent++
	c.ln(`if strings.HasPrefix(r.URL.Path, "/.well-known/acme-challenge/") {`)
	c.indent++
	c.ln(`_acmeHandler.ServeHTTP(w, r)`)
	c.ln(`return`)
	c.indent--
	c.ln(`}`)
	c.ln(`host := r.Host`)
	c.ln(`if _doWwwRedirect {`)
	c.indent++
	c.ln(`h := host`)
	c.ln(`if i := strings.IndexByte(h, ':'); i >= 0 { h = h[:i] }`)
	c.ln(`if !strings.HasPrefix(h, "www.") { host = "www." + host }`)
	c.indent--
	c.ln(`}`)
	c.ln(`http.Redirect(w, r, "https://" + host + r.RequestURI, 301)`)
	c.indent--
	c.ln(`})`)
	c.ln(`fmt.Println("  HTTP :80 → HTTPS redirect")`)
	c.ln(`if err := http.ListenAndServe(":80", httpHandler); err != nil {`)
	c.indent++
	c.ln(`fmt.Printf("HTTP redirect server error: %s\n", err)`)
	c.indent--
	c.ln(`}`)
	c.indent--
	c.ln(`}()`)
	c.ln(`if err := srv.ListenAndServeTLS("", ""); err != nil {`)
	c.indent++
	c.ln(`fmt.Printf("Server error: %s\n", err)`)
	c.indent--
	c.ln(`}`)
	c.indent--
	c.ln(`} else if _sslCert != "" && _sslKey != "" {`)
	c.indent++
	c.ln(`fmt.Printf("  TLS cert: %s\n", _sslCert)`)
	c.ln(`if _doHttpsRedirect {`)
	c.indent++
	c.ln(`go func() {`)
	c.indent++
	c.ln(`httpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {`)
	c.indent++
	c.ln(`host := r.Host`)
	c.ln(`if _doWwwRedirect {`)
	c.indent++
	c.ln(`h := host`)
	c.ln(`if i := strings.IndexByte(h, ':'); i >= 0 { h = h[:i] }`)
	c.ln(`if !strings.HasPrefix(h, "www.") { host = "www." + host }`)
	c.indent--
	c.ln(`}`)
	c.ln(`http.Redirect(w, r, "https://" + host + r.RequestURI, 301)`)
	c.indent--
	c.ln(`})`)
	c.ln(`fmt.Println("  HTTP :80 \u2192 HTTPS redirect")`)
	c.ln(`if err := http.ListenAndServe(":80", httpHandler); err != nil {`)
	c.indent++
	c.ln(`fmt.Printf("HTTP redirect server error: %s\n", err)`)
	c.indent--
	c.ln(`}`)
	c.indent--
	c.ln(`}()`)
	c.indent--
	c.ln(`}`)
	c.ln("if err := http.ListenAndServeTLS(addr, _sslCert, _sslKey, handler); err != nil {")
	c.indent++
	c.ln(`fmt.Printf("Server error: %s\n", err)`)
	c.indent--
	c.ln("}")
	c.indent--
	c.ln("} else {")
	c.indent++
	c.ln("if err := http.ListenAndServe(addr, handler); err != nil {")
	c.indent++
	c.ln(`fmt.Printf("Server error: %s\n", err)`)
	c.indent--
	c.ln("}")
	c.indent--
	c.ln("}")

	c.indent--
	c.ln("}")
}

// collectVars finds all variable names assigned in a block
