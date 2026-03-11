# HTTPDSL for Visual Studio Code

Syntax highlighting and formatting for [HTTPDSL](https://github.com/pixelkarma/httpdsl) — a DSL that compiles to standalone Go HTTP server binaries.

## Features

- Full syntax highlighting for `.httpdsl` files
- Code completion for keywords, snippets, route methods, and core object members (`request.*`, `response.*`, `store.*`, etc.)
- Route definitions with method highlighting (`GET`, `POST`, `PUT`, `SSE`, etc.)
- Built-in function recognition (100+ functions)
- Template string interpolation (`${...}` inside backticks)
- `request`/`response`/`args` variable highlighting (including `request.data`, `request.session`, auth fields)
- `db.*`, `store.*`, `jwt.*`, `file.*`, `stream.*`, `sse.*` method highlighting
- Server block, init/shutdown, before/after, error handlers, scheduled tasks
- Document formatting support (`Format Document` / format-on-save)
- Bracket matching and auto-closing pairs
- Code folding

## Installation

### From Source (symlink)

The quickest way to install during development:

```bash
# macOS / Linux
ln -s /path/to/httpdsl/vscode-httpdsl ~/.vscode/extensions/pixelkarma.httpdsl-0.1.2

# Then reload VS Code (Cmd+Shift+P → "Developer: Reload Window")
```

### Manual Install (copy)

```bash
cp -r /path/to/httpdsl/vscode-httpdsl ~/.vscode/extensions/pixelkarma.httpdsl-0.1.2
```

Reload VS Code after copying.

### Package as VSIX

If you have `vsce` installed:

```bash
cd vscode-httpdsl
npx @vscode/vsce package
# Produces httpdsl-0.1.2.vsix
```

Then install via: `code --install-extension httpdsl-0.1.2.vsix`

## Formatting

- Run `Format Document` (`Shift+Option+F` on macOS) in `.httpdsl` files.
- HTTPDSL sets itself as the default formatter for `[httpdsl]`.
- If another formatter still takes over, run command: `HTTPDSL: Format Document`.
- Formatting normalizes indentation and common spacing (assignment/comparison operators, commas, control-flow keyword spacing).
- To enable format-on-save:

```json
"[httpdsl]": {
  "editor.defaultFormatter": "pixelkarma.httpdsl",
  "editor.formatOnSave": true
}
```

## What Gets Highlighted

| Element | Scope | Example |
|---------|-------|---------|
| Keywords | `keyword.control` | `route`, `if`, `fn`, `each`, `try`, `catch` |
| HTTP methods | `constant.language` | `GET`, `POST`, `PUT`, `DELETE` |
| Block keywords | `keyword.control` | `init`, `shutdown`, `before`, `after` |
| Built-in functions | `support.function` | `len()`, `fetch()`, `validate()` |
| Special variables | `variable.language` | `request`, `response`, `args` |
| Object methods | `support.function` | `db.query()`, `store.get()`, `jwt.sign()` |
| Function definitions | `entity.name.function` | `fn myFunc(a, b)` |
| Function calls | `entity.name.function.call` | `myFunc(x)` |
| Strings | `string.quoted.double` | `"hello"` |
| Template strings | `string.quoted.template` | `` `hello ${name}` `` |
| Interpolation | `punctuation.definition.interpolation` | `${...}` |
| Numbers | `constant.numeric` | `42`, `3.14`, `30s`, `24h` |
| Booleans/null | `constant.language` | `true`, `false`, `null` |
| Comments | `comment.line` | `// comment` |
| Operators | `keyword.operator` | `==`, `&&`, `??`, `+=` |

## Screenshot

Open `examples/sample.httpdsl` in VS Code to see the highlighting in action.
