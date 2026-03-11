# Language Reference

Language-level specification for HTTPDSL syntax and semantics. This spec is runtime/transpiler agnostic.

## Notes

- This file is intended to be machine-readable and AI-ingestible.
- Compiler implementation currently lives in compiler/parser.go and main.go validation.

## Top Level

Allowed top-level blocks:

- `server { ... }`: Server configuration block.
- `route METHOD "/path" [json|text|form] { ... } [else { ... }]`: HTTP route definition.
- `group "/prefix" { route ... before { ... } after { ... } }`: Route grouping with scoped hooks.
- `fn name(arg1, arg2) { ... }`: Named function declaration.
- `init { ... }`: Startup block for global initialization.
- `before { ... }`: Global pre-route hook.
- `after { ... }`: Global post-response hook.
- `shutdown { ... }`: Graceful shutdown hook.
- `error STATUS_CODE { ... }`: Error-page handler by HTTP status code.
- `every 30 s { ... } OR every "0 3 * * *" { ... }`: Scheduler block (interval or cron).
- `help \`...\``: Help text shown in CLI output.

Forbidden rules:

- `no-top-level-assignment`: Top-level assignments are not allowed. Place assignments in init {}.
- `no-top-level-expression-statements`: Arbitrary expression statements are not allowed at top level.

## Lexical

### Keywords

`route`, `fn`, `return`, `if`, `else`, `while`, `each`, `in`, `server`, `json`, `text`, `true`, `false`, `null`, `env`, `file`, `db`, `break`, `continue`, `try`, `catch`, `throw`, `async`, `group`, `jwt`, `switch`, `case`, `default`

### Operators

`=`, `+`, `-`, `*`, `/`, `%`, `!`, `==`, `!=`, `<`, `>`, `<=`, `>=`, `&&`, `||`, `??`, `? :`, `+=`, `-=`

### Delimiters

`,`, `.`, `:`, `(`, `)`, `{`, `}`, `[`, `]`

### Literals

- integer
- float
- string
- template string (backticks)
- boolean
- null
- array
- object/hash

## Statements

### Route

- ID: `route`
- Contexts: `top-level`, `group`
- Syntax: `route METHOD "/path" [json|text|form] { ... } [else { ... }] [disconnect { ... }]`
- Summary: Defines a request handler.
- Notes:
  - Route-level directives inside body: timeout <seconds>, csrf false.
  - disconnect is only valid for SSE routes.
  - Path params use :name; wildcard params use *name.

Examples:

```httpdsl
route GET "/users/:id" { ... }
route POST "/submit" json { ... } else { ... }
route SSE "/events" { ... } disconnect { ... }
```

### Function Declaration

- ID: `function`
- Contexts: `top-level`
- Syntax: `fn name(a, b) { ... }`
- Summary: Declares a named function.

Examples:

```httpdsl
fn add(a, b) { return a + b }
```

### Return

- ID: `return`
- Contexts: `function`, `route`, `hook`
- Syntax: `return | return expr[, expr...]`
- Summary: Returns from the current function/block context.
- Notes:
  - Route handlers return through generated response flow.
  - SSE route return exits handler without writing regular response body.

### Assignment

- ID: `assignment`
- Contexts: `function`, `route`, `hook`
- Syntax: `x = expr | x, y = expr[, expr...] | x += expr | x -= expr`
- Summary: Variable assignment and compound assignment.
- Notes:
  - Not allowed at top level.
  - Object/array destructuring assignment is supported: {a,b}=obj, [a,b]=arr.

### If/Else

- ID: `if`
- Contexts: `function`, `route`, `hook`
- Syntax: `if condition { ... } [else if condition { ... }] [else { ... }]`
- Summary: Conditional branching.

### Switch

- ID: `switch`
- Contexts: `function`, `route`, `hook`
- Syntax: `switch expr { case v1[, v2...] { ... } default { ... } }`
- Summary: Multi-branch dispatch with case values.

### While

- ID: `while`
- Contexts: `function`, `route`, `hook`
- Syntax: `while condition { ... }`
- Summary: Loop while condition is truthy.

### Each

- ID: `each`
- Contexts: `function`, `route`, `hook`
- Syntax: `each value[, index] in iterable { ... }`
- Summary: Iterates arrays/maps.
- Notes:
  - break and continue are valid inside loops.

### Try/Catch

- ID: `try-catch`
- Contexts: `function`, `route`, `hook`
- Syntax: `try { ... } catch(err) { ... }`
- Summary: Exception handling for throw/panic paths.

### Throw

- ID: `throw`
- Contexts: `function`, `route`, `hook`
- Syntax: `throw expr`
- Summary: Raises an error value.

### Before Hook

- ID: `before`
- Contexts: `top-level`, `group`
- Syntax: `before { ... }`
- Summary: Runs before matching route body.

### After Hook

- ID: `after`
- Contexts: `top-level`, `group`
- Syntax: `after { ... }`
- Summary: Runs asynchronously after response has been written.
- Notes:
  - Mutating response in after {} has no effect on client output.

### Init Hook

- ID: `init`
- Contexts: `top-level`
- Syntax: `init { ... }`
- Summary: Runs during startup; defines global variables.

### Shutdown Hook

- ID: `shutdown`
- Contexts: `top-level`
- Syntax: `shutdown { ... }`
- Summary: Runs when process is terminating.

### Error Handler

- ID: `error`
- Contexts: `top-level`
- Syntax: `error 404 { ... }`
- Summary: Custom handler for specific HTTP status codes.

### Scheduler

- ID: `every`
- Contexts: `top-level`
- Syntax: `every 5 m { ... } OR every "0 3 * * *" { ... }`
- Summary: Runs scheduled jobs by interval or cron.
- Notes:
  - Cron expression is validated during parse/compile; invalid syntax is rejected.

### Server Block

- ID: `server`
- Contexts: `top-level`
- Syntax: `server { key value ... }`
- Summary: Configures runtime server behavior.
- Notes:
  - Supports nested blocks: cors { ... }, session { ... }.
  - Supports static mounts: static "/prefix" "./dir".

## Expressions

- `name`: Variable/function reference.
- `123, 3.14, "text", \`tmpl ${expr}\``: Primitive and template literals.
- `[a, b], {a: 1, b}, {a, b}`: Collection literals, including object shorthand.
- `fn(a, b)`: Function call.
- `obj.field, obj[idx]`: Property and indexed access.
- `fn(a, b) { ... }`: Function literal expression.
- `async expr`: Marks expression for asynchronous execution context.
- `cond ? yes : no`: Conditional expression.

## Precedence

- L1 (right): `?:`
- L2 (left): `||`
- L3 (left): `??`
- L4 (left): `&&`
- L5 (left): `==`, `!=`
- L6 (left): `<`, `>`, `<=`, `>=`
- L7 (left): `+`, `-`
- L8 (left): `*`, `/`, `%`
- L9 (left): `call`, `dot`, `index`

## Semantics

- `init-global-vars`: Variables assigned in init {} become global variables available in route and function execution. (`compiler/native.go:collectVars/initBlocks`)
- `after-is-post-response`: after {} executes after writeResponse and cannot change the already-sent client response. (`compiler/native.go:emitRouteHandler`)
- `route-type-check`: route ... json/text/form performs request type check before main route body. (`compiler/parser.go:parseRouteStatement`)
- `every-cron-validated`: every "cron" uses 5-field cron syntax with compile-time field validation. (`compiler/parser.go:validateCronExpr`)

## Known Limits

- `contextual-keywords`: init/before/after/shutdown/help/every/error are contextual and parsed from identifier tokens. (`compiler/parser.go:parseStatement`)
- `cron-fields`: Cron supports numeric ranges, lists, wildcards, and steps in five fields only. (`compiler/parser.go:validateCronField`)
