package runtimeassets

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"
)

//go:embed templates/*.gotmpl
var templateFS embed.FS

var runtimeTemplates = template.Must(template.New("runtime").Funcs(template.FuncMap{
	"quote":      quote,
	"joinQuoted": joinQuoted,
}).ParseFS(templateFS, "templates/*.gotmpl"))

// Render renders an embedded runtime template by filename, e.g. "session_runtime.gotmpl".
func Render(name string, data any) (string, error) {
	var b bytes.Buffer
	if err := runtimeTemplates.ExecuteTemplate(&b, name, data); err != nil {
		return "", fmt.Errorf("render template %q: %w", name, err)
	}
	return b.String(), nil
}

func quote(v string) string {
	return fmt.Sprintf("%q", v)
}

func joinQuoted(items []string) string {
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, len(items))
	for i, s := range items {
		parts[i] = quote(s)
	}
	return strings.Join(parts, ", ")
}
