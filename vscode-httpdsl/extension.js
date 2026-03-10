const vscode = require("vscode");

function activate(context) {
    const provider = vscode.languages.registerDocumentFormattingEditProvider("httpdsl", {
        provideDocumentFormattingEdits(document, options) {
            const source = document.getText();
            const formatted = formatHTTPDSL(source, options);
            if (formatted === source) {
                return [];
            }
            const fullRange = new vscode.Range(
                document.positionAt(0),
                document.positionAt(source.length),
            );
            return [vscode.TextEdit.replace(fullRange, formatted)];
        },
    });

    context.subscriptions.push(provider);
}

function deactivate() {}

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

        const content = noTrail.replace(/^[ \t]+/g, "");
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

