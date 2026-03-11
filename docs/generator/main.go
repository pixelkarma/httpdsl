package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type languageSpec struct {
	Schema      string       `json:"$schema"`
	Meta        languageMeta `json:"meta"`
	TopLevel    topLevelSpec `json:"topLevel"`
	Lexical     lexicalSpec  `json:"lexical"`
	Statements  []statement  `json:"statements"`
	Expressions expressions  `json:"expressions"`
	Semantics   []ruleItem   `json:"semantics"`
	KnownLimits []ruleItem   `json:"knownLimits"`
}

type languageMeta struct {
	Name          string   `json:"name"`
	SpecVersion   string   `json:"specVersion"`
	SourceVersion string   `json:"sourceVersion"`
	UpdatedAt     string   `json:"updatedAt"`
	Description   string   `json:"description"`
	Notes         []string `json:"notes"`
}

type topLevelSpec struct {
	AllowedBlocks []topLevelBlock `json:"allowedBlocks"`
	Forbidden     []ruleItem      `json:"forbidden"`
}

type topLevelBlock struct {
	Name    string `json:"name"`
	Syntax  string `json:"syntax"`
	Summary string `json:"summary"`
}

type lexicalSpec struct {
	Keywords   []string `json:"keywords"`
	Operators  []string `json:"operators"`
	Delimiters []string `json:"delimiters"`
	Literals   []string `json:"literals"`
}

type statement struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Contexts []string `json:"contexts"`
	Syntax   string   `json:"syntax"`
	Summary  string   `json:"summary"`
	Notes    []string `json:"notes"`
	Examples []string `json:"examples"`
}

type expressions struct {
	Forms      []expressionForm `json:"forms"`
	Precedence []precedenceItem `json:"precedence"`
}

type expressionForm struct {
	Name    string `json:"name"`
	Syntax  string `json:"syntax"`
	Summary string `json:"summary"`
}

type precedenceItem struct {
	Level         int      `json:"level"`
	Operators     []string `json:"operators"`
	Associativity string   `json:"associativity"`
}

type ruleItem struct {
	ID     string `json:"id"`
	Rule   string `json:"rule"`
	Reason string `json:"reason"`
	Source string `json:"source"`
}

type builtinsSpec struct {
	Schema string         `json:"$schema"`
	Meta   builtinsMeta   `json:"meta"`
	Groups []builtinGroup `json:"groups"`
}

type builtinsMeta struct {
	Name        string   `json:"name"`
	SpecVersion string   `json:"specVersion"`
	UpdatedAt   string   `json:"updatedAt"`
	Notes       []string `json:"notes"`
}

type builtinGroup struct {
	ID    string        `json:"id"`
	Name  string        `json:"name"`
	Kind  string        `json:"kind"`
	Items []builtinItem `json:"items"`
}

type builtinItem struct {
	Name         string   `json:"name"`
	Syntax       string   `json:"syntax"`
	Summary      string   `json:"summary"`
	Contexts     []string `json:"contexts"`
	Availability string   `json:"availability"`
}

func main() {
	repoRoot, err := findRepoRoot()
	if err != nil {
		fatal(err)
	}

	specDir := filepath.Join(repoRoot, "docs", "spec")
	markdownDir := filepath.Join(repoRoot, "docs", "markdown")

	languagePath := filepath.Join(specDir, "language.json")
	builtinsPath := filepath.Join(specDir, "builtins.json")
	grammarPath := filepath.Join(specDir, "grammar.ebnf")

	var lang languageSpec
	if err := readJSONStrict(languagePath, &lang); err != nil {
		fatal(fmt.Errorf("read %s: %w", languagePath, err))
	}

	var builtins builtinsSpec
	if err := readJSONStrict(builtinsPath, &builtins); err != nil {
		fatal(fmt.Errorf("read %s: %w", builtinsPath, err))
	}

	grammarBytes, err := os.ReadFile(grammarPath)
	if err != nil {
		fatal(fmt.Errorf("read %s: %w", grammarPath, err))
	}
	grammar := strings.TrimSpace(string(grammarBytes))

	if err := validateLanguage(lang); err != nil {
		fatal(fmt.Errorf("language spec validation failed: %w", err))
	}
	if err := validateBuiltins(builtins); err != nil {
		fatal(fmt.Errorf("builtins spec validation failed: %w", err))
	}
	if grammar == "" {
		fatal(fmt.Errorf("grammar.ebnf is empty"))
	}

	if err := os.MkdirAll(markdownDir, 0o755); err != nil {
		fatal(fmt.Errorf("create markdown dir: %w", err))
	}

	generatedAt := time.Now().UTC().Format(time.RFC3339)

	files := map[string]string{
		"README.md":   renderMarkdownIndex(lang, builtins, generatedAt),
		"language.md": renderLanguageMarkdown(lang),
		"builtins.md": renderBuiltinsMarkdown(builtins),
		"grammar.md":  renderGrammarMarkdown(grammar),
	}

	for name, content := range files {
		outPath := filepath.Join(markdownDir, name)
		if err := os.WriteFile(outPath, []byte(content), 0o644); err != nil {
			fatal(fmt.Errorf("write %s: %w", outPath, err))
		}
	}

	fmt.Printf("Generated %d markdown files in %s\n", len(files), markdownDir)
}

func findRepoRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	cur := wd
	for {
		if _, err := os.Stat(filepath.Join(cur, "go.mod")); err == nil {
			return cur, nil
		}
		next := filepath.Dir(cur)
		if next == cur {
			return "", fmt.Errorf("could not find go.mod above %s", wd)
		}
		cur = next
	}
}

func readJSONStrict(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("unexpected trailing JSON tokens")
	}
	return nil
}

func validateLanguage(spec languageSpec) error {
	if strings.TrimSpace(spec.Meta.Name) == "" {
		return fmt.Errorf("meta.name is required")
	}
	if len(spec.TopLevel.AllowedBlocks) == 0 {
		return fmt.Errorf("topLevel.allowedBlocks must not be empty")
	}
	if len(spec.Statements) == 0 {
		return fmt.Errorf("statements must not be empty")
	}
	ids := make(map[string]struct{}, len(spec.Statements))
	for i, s := range spec.Statements {
		if strings.TrimSpace(s.ID) == "" {
			return fmt.Errorf("statements[%d].id is required", i)
		}
		if _, exists := ids[s.ID]; exists {
			return fmt.Errorf("duplicate statement id %q", s.ID)
		}
		ids[s.ID] = struct{}{}
	}
	if !hasRule(spec.TopLevel.Forbidden, "no-top-level-assignment") {
		return fmt.Errorf("topLevel.forbidden must include no-top-level-assignment")
	}
	if !hasRule(spec.Semantics, "after-is-post-response") {
		return fmt.Errorf("semantics must include after-is-post-response")
	}
	if !hasRule(spec.Semantics, "every-cron-validated") {
		return fmt.Errorf("semantics must include every-cron-validated")
	}
	return nil
}

func validateBuiltins(spec builtinsSpec) error {
	if strings.TrimSpace(spec.Meta.Name) == "" {
		return fmt.Errorf("meta.name is required")
	}
	if len(spec.Groups) == 0 {
		return fmt.Errorf("groups must not be empty")
	}
	ids := make(map[string]struct{}, len(spec.Groups))
	for i, g := range spec.Groups {
		if strings.TrimSpace(g.ID) == "" {
			return fmt.Errorf("groups[%d].id is required", i)
		}
		if _, exists := ids[g.ID]; exists {
			return fmt.Errorf("duplicate group id %q", g.ID)
		}
		ids[g.ID] = struct{}{}
		if len(g.Items) == 0 {
			return fmt.Errorf("group %q has no items", g.ID)
		}
	}
	return nil
}

func hasRule(rules []ruleItem, id string) bool {
	for _, r := range rules {
		if r.ID == id {
			return true
		}
	}
	return false
}

func renderMarkdownIndex(lang languageSpec, builtins builtinsSpec, generatedAt string) string {
	var b strings.Builder
	b.WriteString("# HTTPDSL Generated Docs\n\n")
	b.WriteString("Generated from machine-readable spec files in `docs/spec/`.\n\n")
	b.WriteString("- Language: `" + mdEscape(lang.Meta.Name) + "`\n")
	b.WriteString("- Language spec version: `" + mdEscape(lang.Meta.SpecVersion) + "`\n")
	b.WriteString("- Builtins spec version: `" + mdEscape(builtins.Meta.SpecVersion) + "`\n")
	b.WriteString("- Generated at (UTC): `" + mdEscape(generatedAt) + "`\n\n")
	b.WriteString("## Pages\n\n")
	b.WriteString("- [Language Reference](./language.md)\n")
	b.WriteString("- [Builtins Reference](./builtins.md)\n")
	b.WriteString("- [Grammar (EBNF)](./grammar.md)\n")
	return b.String()
}

func renderLanguageMarkdown(spec languageSpec) string {
	var b strings.Builder
	b.WriteString("# Language Reference\n\n")
	b.WriteString(mdEscape(spec.Meta.Description) + "\n\n")

	if len(spec.Meta.Notes) > 0 {
		b.WriteString("## Notes\n\n")
		for _, n := range spec.Meta.Notes {
			b.WriteString("- " + mdEscape(n) + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("## Top Level\n\n")
	b.WriteString("Allowed top-level blocks:\n\n")
	for _, blk := range spec.TopLevel.AllowedBlocks {
		b.WriteString("- `" + mdEscape(blk.Syntax) + "`: " + mdEscape(blk.Summary) + "\n")
	}
	b.WriteString("\nForbidden rules:\n\n")
	for _, f := range spec.TopLevel.Forbidden {
		b.WriteString("- `" + mdEscape(f.ID) + "`: " + mdEscape(f.Rule) + "\n")
	}
	b.WriteString("\n")

	b.WriteString("## Lexical\n\n")
	b.WriteString("### Keywords\n\n")
	b.WriteString(joinCode(spec.Lexical.Keywords) + "\n\n")
	b.WriteString("### Operators\n\n")
	b.WriteString(joinCode(spec.Lexical.Operators) + "\n\n")
	b.WriteString("### Delimiters\n\n")
	b.WriteString(joinCode(spec.Lexical.Delimiters) + "\n\n")
	b.WriteString("### Literals\n\n")
	for _, lit := range spec.Lexical.Literals {
		b.WriteString("- " + mdEscape(lit) + "\n")
	}
	b.WriteString("\n")

	b.WriteString("## Statements\n\n")
	for _, s := range spec.Statements {
		b.WriteString("### " + mdEscape(s.Name) + "\n\n")
		b.WriteString("- ID: `" + mdEscape(s.ID) + "`\n")
		b.WriteString("- Contexts: " + joinCode(s.Contexts) + "\n")
		b.WriteString("- Syntax: `" + mdEscape(s.Syntax) + "`\n")
		b.WriteString("- Summary: " + mdEscape(s.Summary) + "\n")
		if len(s.Notes) > 0 {
			b.WriteString("- Notes:\n")
			for _, n := range s.Notes {
				b.WriteString("  - " + mdEscape(n) + "\n")
			}
		}
		if len(s.Examples) > 0 {
			b.WriteString("\nExamples:\n\n")
			b.WriteString("```httpdsl\n")
			for _, ex := range s.Examples {
				b.WriteString(ex + "\n")
			}
			b.WriteString("```\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("## Expressions\n\n")
	for _, f := range spec.Expressions.Forms {
		b.WriteString("- `" + mdEscape(f.Syntax) + "`: " + mdEscape(f.Summary) + "\n")
	}
	b.WriteString("\n")

	b.WriteString("## Precedence\n\n")
	levels := append([]precedenceItem(nil), spec.Expressions.Precedence...)
	sort.Slice(levels, func(i, j int) bool { return levels[i].Level < levels[j].Level })
	for _, p := range levels {
		assoc := p.Associativity
		if strings.TrimSpace(assoc) == "" {
			assoc = "n/a"
		}
		b.WriteString(fmt.Sprintf("- L%d (%s): %s\n", p.Level, mdEscape(assoc), joinCode(p.Operators)))
	}
	b.WriteString("\n")

	b.WriteString("## Semantics\n\n")
	for _, r := range spec.Semantics {
		b.WriteString("- `" + mdEscape(r.ID) + "`: " + mdEscape(r.Rule))
		if strings.TrimSpace(r.Source) != "" {
			b.WriteString(" (`" + mdEscape(r.Source) + "`)")
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")

	if len(spec.KnownLimits) > 0 {
		b.WriteString("## Known Limits\n\n")
		for _, r := range spec.KnownLimits {
			b.WriteString("- `" + mdEscape(r.ID) + "`: " + mdEscape(r.Rule))
			if strings.TrimSpace(r.Source) != "" {
				b.WriteString(" (`" + mdEscape(r.Source) + "`)")
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func renderBuiltinsMarkdown(spec builtinsSpec) string {
	var b strings.Builder
	b.WriteString("# Builtins Reference\n\n")
	if len(spec.Meta.Notes) > 0 {
		for _, n := range spec.Meta.Notes {
			b.WriteString("- " + mdEscape(n) + "\n")
		}
		b.WriteString("\n")
	}

	for _, g := range spec.Groups {
		b.WriteString("## " + mdEscape(g.Name) + "\n\n")
		b.WriteString("Kind: `" + mdEscape(g.Kind) + "`\n\n")
		for _, item := range g.Items {
			b.WriteString("### `" + mdEscape(item.Name) + "`\n\n")
			b.WriteString("- Syntax: `" + mdEscape(item.Syntax) + "`\n")
			b.WriteString("- Summary: " + mdEscape(item.Summary) + "\n")
			if len(item.Contexts) > 0 {
				b.WriteString("- Contexts: " + joinCode(item.Contexts) + "\n")
			}
			if strings.TrimSpace(item.Availability) != "" {
				b.WriteString("- Availability: " + mdEscape(item.Availability) + "\n")
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func renderGrammarMarkdown(grammar string) string {
	var b strings.Builder
	b.WriteString("# Grammar (EBNF)\n\n")
	b.WriteString("```ebnf\n")
	b.WriteString(grammar)
	b.WriteString("\n```\n")
	return b.String()
}

func mdEscape(s string) string {
	s = strings.ReplaceAll(s, "`", "\\`")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

func joinCode(items []string) string {
	if len(items) == 0 {
		return ""
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, "`"+mdEscape(item)+"`")
	}
	return strings.Join(out, ", ")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
