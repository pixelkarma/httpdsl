const vscode = require("vscode");

const ROUTE_METHODS = ["GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD", "SSE"];
const EVERY_UNITS = ["s", "m", "h"];
const KEYWORDS = [
    "route",
    "group",
    "server",
    "init",
    "shutdown",
    "before",
    "after",
    "error",
    "every",
    "fn",
    "return",
    "if",
    "else",
    "while",
    "each",
    "in",
    "break",
    "continue",
    "try",
    "catch",
    "throw",
    "switch",
    "case",
    "default",
    "async",
];
const BUILTINS = [
    "len", "type", "int", "float", "str", "bool",
    "print", "log", "log_info", "log_warn", "log_error",
    "keys", "values", "merge", "hash", "append", "push",
    "split", "join", "trim", "lower", "upper", "capitalize", "replace",
    "contains", "starts_with", "ends_with", "index_of", "repeat",
    "slice", "reverse", "sort", "sort_by", "filter", "map", "reduce",
    "find", "some", "every", "count", "flat", "unique", "chunk", "pluck", "group_by",
    "sum", "range", "abs", "ceil", "floor", "round", "min", "max", "clamp", "rand",
    "now", "now_ms", "date", "date_format", "date_parse", "strtotime", "sleep",
    "uuid", "cuid2",
    "hash_password", "verify_password", "hmac_hash",
    "env", "fetch", "redirect", "render", "broadcast", "exec", "server_stats",
    "validate", "is_email", "is_url", "is_uuid", "is_numeric",
    "url_encode", "url_decode", "base64_encode", "base64_decode",
    "set_session_store", "csrf_token", "csrf_field",
];
const OBJECT_COMPLETIONS = {
    request: [
        { label: "method", kind: "property" },
        { label: "path", kind: "property" },
        { label: "params", kind: "property" },
        { label: "query", kind: "property" },
        { label: "data", kind: "property" },
        { label: "headers", kind: "property" },
        { label: "cookies", kind: "property" },
        { label: "ip", kind: "property" },
        { label: "session", kind: "property" },
        { label: "bearer", kind: "property" },
        { label: "basic", kind: "property" },
    ],
    response: [
        { label: "status", kind: "property" },
        { label: "type", kind: "property" },
        { label: "headers", kind: "property" },
        { label: "cookies", kind: "property" },
        { label: "body", kind: "property" },
    ],
    store: [
        { label: "get", kind: "method", snippet: "get(${1:key}, ${2:null})" },
        { label: "set", kind: "method", snippet: "set(${1:key}, ${2:value})" },
        { label: "delete", kind: "method", snippet: "delete(${1:key})" },
        { label: "has", kind: "method", snippet: "has(${1:key})" },
        { label: "all", kind: "method", snippet: "all()" },
        { label: "incr", kind: "method", snippet: "incr(${1:key}, ${2:1})" },
        { label: "sync", kind: "method", snippet: "sync(${1:path_or_db})" },
    ],
    db: [
        { label: "open", kind: "method", snippet: "open(${1:\"sqlite\"}, ${2:\"./app.db\"})" },
    ],
    file: [
        { label: "open", kind: "method", snippet: "open(${1:\"./file.txt\"})" },
        { label: "read", kind: "method", snippet: "read(${1:\"./file.txt\"})" },
        { label: "write", kind: "method", snippet: "write(${1:\"./file.txt\"}, ${2:data})" },
        { label: "append", kind: "method", snippet: "append(${1:\"./file.txt\"}, ${2:data})" },
        { label: "read_json", kind: "method", snippet: "read_json(${1:\"./file.json\"})" },
        { label: "write_json", kind: "method", snippet: "write_json(${1:\"./file.json\"}, ${2:data})" },
        { label: "exists", kind: "method", snippet: "exists(${1:\"./file.txt\"})" },
        { label: "delete", kind: "method", snippet: "delete(${1:\"./file.txt\"})" },
        { label: "list", kind: "method", snippet: "list(${1:\"./dir\"})" },
        { label: "mkdir", kind: "method", snippet: "mkdir(${1:\"./dir\"})" },
        { label: "chmod", kind: "method", snippet: "chmod(${1:\"./file.txt\"}, ${2:644})" },
    ],
    jwt: [
        { label: "sign", kind: "method", snippet: "sign(${1:payload}, ${2:secret})" },
        { label: "verify", kind: "method", snippet: "verify(${1:token}, ${2:secret})" },
    ],
    stream: [
        { label: "id", kind: "property" },
        { label: "send", kind: "method", snippet: "send(${1:\"event\"}, ${2:data})" },
        { label: "join", kind: "method", snippet: "join(${1:\"channel\"})" },
        { label: "leave", kind: "method", snippet: "leave(${1:\"channel\"})" },
        { label: "set", kind: "method", snippet: "set(${1:\"key\"}, ${2:value})" },
        { label: "get", kind: "method", snippet: "get(${1:\"key\"})" },
        { label: "channels", kind: "method", snippet: "channels()" },
        { label: "close", kind: "method", snippet: "close()" },
    ],
    sse: [
        { label: "find", kind: "method", snippet: "find(${1:id})" },
        { label: "find_by", kind: "method", snippet: "find_by(${1:\"key\"}, ${2:value})" },
        { label: "channel", kind: "method", snippet: "channel(${1:\"name\"})" },
        { label: "broadcast", kind: "method", snippet: "broadcast(${1:\"event\"}, ${2:data})" },
        { label: "count", kind: "method", snippet: "count()" },
        { label: "channels", kind: "method", snippet: "channels()" },
    ],
};

function activate(context) {
    const out = vscode.window.createOutputChannel("HTTPDSL");
    out.appendLine("HTTPDSL extension activated");

    const selector = [
        { language: "httpdsl", scheme: "file" },
        { language: "httpdsl", scheme: "untitled" },
    ];

    const provider = {
        provideDocumentFormattingEdits(document, options) {
            out.appendLine(`Formatting document: ${document.uri.toString()}`);
            return formatDocumentEdits(document, options, out);
        },
        provideDocumentRangeFormattingEdits(document, _range, options) {
            // Range formatting formats the whole document for predictable brace indentation.
            out.appendLine(`Range format requested; formatting full document: ${document.uri.toString()}`);
            return formatDocumentEdits(document, options, out);
        },
    };

    const fullDocProvider = vscode.languages.registerDocumentFormattingEditProvider(selector, provider);
    const rangeProvider = vscode.languages.registerDocumentRangeFormattingEditProvider(selector, provider);
    const completionProvider = vscode.languages.registerCompletionItemProvider(
        selector,
        {
            provideCompletionItems(document, position) {
                return provideHTTPDSLCompletions(document, position);
            },
        },
        ".",
    );

    const formatCommand = vscode.commands.registerCommand("httpdsl.formatDocument", async () => {
        const editor = vscode.window.activeTextEditor;
        if (!editor) {
            vscode.window.showInformationMessage("No active editor.");
            return;
        }
        if (editor.document.languageId !== "httpdsl") {
            vscode.window.showWarningMessage("Active file is not HTTPDSL.");
            return;
        }
        const edits = formatDocumentEdits(editor.document, editor.options, out);
        if (!edits.length) {
            vscode.window.showInformationMessage("HTTPDSL: Document already formatted.");
            return;
        }
        await editor.edit((builder) => {
            for (const edit of edits) {
                builder.replace(edit.range, edit.newText);
            }
        });
        vscode.window.showInformationMessage("HTTPDSL: Formatted document.");
    });

    context.subscriptions.push(out, fullDocProvider, rangeProvider, completionProvider, formatCommand);
}

function deactivate() {}

function provideHTTPDSLCompletions(document, position) {
    const linePrefix = document.lineAt(position.line).text.slice(0, position.character);

    const dotMatch = linePrefix.match(/([A-Za-z_][A-Za-z0-9_]*)\.\s*([A-Za-z_]*)?$/);
    if (dotMatch) {
        const obj = dotMatch[1];
        if (OBJECT_COMPLETIONS[obj]) {
            return OBJECT_COMPLETIONS[obj].map((entry) => objectCompletionItem(entry));
        }
    }

    if (/\broute\s+[A-Za-z_]*$/.test(linePrefix)) {
        return ROUTE_METHODS.map((m) => {
            const item = new vscode.CompletionItem(m, vscode.CompletionItemKind.EnumMember);
            item.detail = "HTTPDSL route method";
            return item;
        });
    }

    if (/\bevery\s+\d+\s+[A-Za-z_]*$/.test(linePrefix)) {
        return EVERY_UNITS.map((u) => {
            const item = new vscode.CompletionItem(u, vscode.CompletionItemKind.EnumMember);
            item.detail = "HTTPDSL interval unit";
            return item;
        });
    }

    const items = [
        ...snippetCompletions(),
        ...KEYWORDS.map((k) => keywordCompletionItem(k)),
        ...BUILTINS.map((b) => builtinCompletionItem(b)),
    ];
    return items;
}

function snippetCompletions() {
    return [
        snippetItem("server", "server {\n\t$0\n}", "Server block"),
        snippetItem("init", "init {\n\t$0\n}", "Init block"),
        snippetItem("before", "before {\n\t$0\n}", "Before middleware"),
        snippetItem("after", "after {\n\t$0\n}", "After middleware"),
        snippetItem("shutdown", "shutdown {\n\t$0\n}", "Shutdown block"),
        snippetItem("group", "group \"${1:/prefix}\" {\n\t$0\n}", "Group block"),
        snippetItem("route", "route ${1|GET,POST,PUT,PATCH,DELETE,SSE|} \"${2:/path}\" {\n\t$0\n}", "Route block"),
        snippetItem("fn", "fn ${1:name}(${2:args}) {\n\t$0\n}", "Function declaration"),
        snippetItem("if", "if ${1:condition} {\n\t$0\n}", "If block"),
        snippetItem("ifelse", "if ${1:condition} {\n\t$2\n} else {\n\t$0\n}", "If/else block"),
        snippetItem("try", "try {\n\t$1\n} catch(${2:err}) {\n\t$0\n}", "Try/catch block"),
        snippetItem("switch", "switch ${1:value} {\n\tcase ${2:value} {\n\t\t$0\n\t}\n}", "Switch block"),
        snippetItem("error", "error ${1:404} {\n\t$0\n}", "Error handler"),
        snippetItem("every", "every ${1:30} ${2|s,m,h|} {\n\t$0\n}", "Scheduled interval"),
        snippetItem("everycron", "every \"${1:* * * * *}\" {\n\t$0\n}", "Scheduled cron"),
    ];
}

function snippetItem(label, snippet, detail) {
    const item = new vscode.CompletionItem(label, vscode.CompletionItemKind.Snippet);
    item.insertText = new vscode.SnippetString(snippet);
    item.detail = detail;
    item.sortText = `0_${label}`;
    return item;
}

function keywordCompletionItem(keyword) {
    const item = new vscode.CompletionItem(keyword, vscode.CompletionItemKind.Keyword);
    item.detail = "HTTPDSL keyword";
    item.sortText = `1_${keyword}`;
    return item;
}

function builtinCompletionItem(name) {
    const item = new vscode.CompletionItem(name, vscode.CompletionItemKind.Function);
    item.insertText = new vscode.SnippetString(`${name}($1)`);
    item.detail = "HTTPDSL builtin";
    item.sortText = `2_${name}`;
    return item;
}

function objectCompletionItem(entry) {
    const kind = entry.kind === "method" ? vscode.CompletionItemKind.Method : vscode.CompletionItemKind.Property;
    const item = new vscode.CompletionItem(entry.label, kind);
    if (entry.snippet) {
        item.insertText = new vscode.SnippetString(entry.snippet);
    }
    item.detail = "HTTPDSL member";
    return item;
}

function formatDocumentEdits(document, options, out) {
    const source = document.getText();
    const formatted = formatHTTPDSL(source, options);
    if (formatted === source) {
        if (out) {
            out.appendLine("No formatting changes");
        }
        return [];
    }
    const fullRange = new vscode.Range(
        document.positionAt(0),
        document.positionAt(source.length),
    );
    if (out) {
        out.appendLine("Formatting edits generated");
    }
    return [vscode.TextEdit.replace(fullRange, formatted)];
}

function formatHTTPDSL(src, options) {
    const indentSize = options && Number.isInteger(options.tabSize) && options.tabSize > 0 ? options.tabSize : 4;
    const indentUnit = options && options.insertSpaces === false ? "\t" : " ".repeat(indentSize);

    let normalized = src.replace(/\r\n/g, "\n").replace(/\r/g, "\n");
    const lines = normalized.split("\n");

    const out = [];
    let indentLevel = 0;
    let state = { inBlockComment: false, inTemplate: false, inString: false };

    for (const line of lines) {
        if (state.inTemplate || state.inString || state.inBlockComment) {
            out.push(line);
            const analysis = analyzeLine(line, state);
            state = analysis.endState;
            continue;
        }

        const noTrail = line.replace(/[ \t]+$/g, "");
        if (noTrail.trim() === "") {
            out.push("");
            continue;
        }

        const analysis = analyzeLine(noTrail, state);
        let indent = indentLevel;
        if (analysis.startsWithClosingBrace) {
            indent -= 1;
        }
        if (indent < 0) {
            indent = 0;
        }

        const rawContent = noTrail.replace(/^[ \t]+/g, "");
        const content = normalizeSpacing(rawContent);
        out.push(indentUnit.repeat(indent) + content);

        indentLevel += analysis.openBraces - analysis.closeBraces;
        if (indentLevel < 0) {
            indentLevel = 0;
        }
        state = analysis.endState;
    }

    while (out.length > 0 && out[out.length - 1] === "") {
        out.pop();
    }
    return out.join("\n") + "\n";
}

function normalizeSpacing(line) {
    const parts = splitCodeAndComment(line);
    let code = parts.code;
    const comment = parts.comment;

    code = code.replace(/\s*(\+=|-=|==|!=|<=|>=|&&|\|\||\?\?|=|<|>)\s*/g, " $1 ");
    code = code.replace(/\s*,\s*/g, ", ");

    // Tighten grouping punctuation and normalize common control-flow spacing.
    code = code.replace(/\(\s+/g, "(");
    code = code.replace(/\s+\)/g, ")");
    code = code.replace(/\[\s+/g, "[");
    code = code.replace(/\s+\]/g, "]");
    code = code.replace(/\b(if|while|switch|catch)\s*\(/g, "$1 (");
    code = code.replace(/\belse\s*\{/g, "else {");
    code = code.replace(/\}\s*else\b/g, "} else");
    code = code.replace(/\)\s*\{/g, ") {");

    code = code.replace(/[ \t]{2,}/g, " ").trim();
    if (!comment) {
        return code;
    }
    const trimmedComment = comment.replace(/^[ \t]+/g, "");
    if (!code) {
        return trimmedComment;
    }
    return `${code} ${trimmedComment}`;
}

function splitCodeAndComment(line) {
    let inString = false;
    let inTemplate = false;
    let stringEsc = false;
    let templateEsc = false;

    for (let i = 0; i < line.length; i += 1) {
        const ch = line[i];
        const next = i + 1 < line.length ? line[i + 1] : "";

        if (inString) {
            if (ch === '"' && !stringEsc) {
                inString = false;
            }
            if (ch === "\\" && !stringEsc) {
                stringEsc = true;
            } else {
                stringEsc = false;
            }
            continue;
        }

        if (inTemplate) {
            if (ch === "`" && !templateEsc) {
                inTemplate = false;
            }
            if (ch === "\\" && !templateEsc) {
                templateEsc = true;
            } else {
                templateEsc = false;
            }
            continue;
        }

        if (ch === '"' && !inTemplate) {
            inString = true;
            stringEsc = false;
            continue;
        }
        if (ch === "`" && !inString) {
            inTemplate = true;
            templateEsc = false;
            continue;
        }

        if (ch === "/" && next === "/") {
            return { code: line.slice(0, i), comment: line.slice(i) };
        }
        if (ch === "/" && next === "*") {
            return { code: line.slice(0, i), comment: line.slice(i) };
        }
    }

    return { code: line, comment: "" };
}

function analyzeLine(line, startState) {
    const state = { ...startState };
    let openBraces = 0;
    let closeBraces = 0;
    let startsWithClosingBrace = false;
    let firstTokenSeen = false;
    let stringEsc = false;
    let templateEsc = false;

    for (let i = 0; i < line.length; i += 1) {
        const ch = line[i];
        const next = i + 1 < line.length ? line[i + 1] : "";

        if (state.inBlockComment) {
            if (ch === "*" && next === "/") {
                state.inBlockComment = false;
                i += 1;
            }
            continue;
        }

        if (state.inString) {
            if (ch === '"' && !stringEsc) {
                state.inString = false;
            }
            if (ch === "\\" && !stringEsc) {
                stringEsc = true;
            } else {
                stringEsc = false;
            }
            continue;
        }

        if (state.inTemplate) {
            if (ch === "`" && !templateEsc) {
                state.inTemplate = false;
            }
            if (ch === "\\" && !templateEsc) {
                templateEsc = true;
            } else {
                templateEsc = false;
            }
            continue;
        }

        if (ch === "/" && next === "/") {
            break;
        }
        if (ch === "/" && next === "*") {
            state.inBlockComment = true;
            i += 1;
            continue;
        }
        if (ch === '"') {
            state.inString = true;
            stringEsc = false;
            if (!firstTokenSeen) {
                firstTokenSeen = true;
            }
            continue;
        }
        if (ch === "`") {
            state.inTemplate = true;
            templateEsc = false;
            if (!firstTokenSeen) {
                firstTokenSeen = true;
            }
            continue;
        }

        if (!firstTokenSeen) {
            if (ch === " " || ch === "\t") {
                continue;
            }
            firstTokenSeen = true;
            if (ch === "}") {
                startsWithClosingBrace = true;
            }
        }

        if (ch === "{") {
            openBraces += 1;
        } else if (ch === "}") {
            closeBraces += 1;
        }
    }

    return {
        openBraces,
        closeBraces,
        startsWithClosingBrace,
        endState: state,
    };
}

module.exports = {
    activate,
    deactivate,
};
