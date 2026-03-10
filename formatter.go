package main

import (
	"fmt"
	"httpdsl/compiler"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type formatState struct {
	inBlockComment bool
	inTemplate     bool
	inString       bool
}

type lineAnalysis struct {
	openBraces             int
	closeBraces            int
	startsWithClosingBrace bool
	endState               formatState
}

func doFmt(target string) error {
	files, err := collectHTTPDSLFiles(target)
	if err != nil {
		return err
	}

	changed := 0
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		original := string(data)
		formatted := formatHTTPDSLSource(original)
		if formatted == original {
			continue
		}

		// Safety check: if input parsed cleanly, output must parse cleanly.
		if errs := parseErrors(original); len(errs) == 0 {
			if outErrs := parseErrors(formatted); len(outErrs) > 0 {
				return fmt.Errorf("%s: formatter produced invalid code: %s", path, outErrs[0])
			}
		}

		if err := os.WriteFile(path, []byte(formatted), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
		fmt.Printf("formatted %s\n", path)
		changed++
	}

	if changed == 0 {
		fmt.Printf("No files changed (%d checked)\n", len(files))
	} else {
		fmt.Printf("Formatted %d file%s\n", changed, plural(changed))
	}
	return nil
}

func collectHTTPDSLFiles(target string) ([]string, error) {
	info, err := os.Stat(target)
	if err != nil {
		return nil, err
	}

	var files []string
	if info.IsDir() {
		err := filepath.Walk(target, func(path string, fi os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if fi.IsDir() {
				return nil
			}
			if strings.HasSuffix(fi.Name(), ".httpdsl") {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		if !strings.HasSuffix(info.Name(), ".httpdsl") {
			return nil, fmt.Errorf("%s is not a .httpdsl file", target)
		}
		files = append(files, target)
	}

	if len(files) == 0 {
		return nil, fmt.Errorf("no .httpdsl files found")
	}
	sort.Strings(files)
	return files, nil
}

func parseErrors(src string) []string {
	l := compiler.NewLexer(src)
	p := compiler.NewParser(l)
	_ = p.ParseProgram()
	return p.Errors()
}

func formatHTTPDSLSource(src string) string {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")

	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines))

	indentLevel := 0
	state := formatState{}

	for _, line := range lines {
		if state.inTemplate || state.inString || state.inBlockComment {
			out = append(out, line)
			analysis := analyzeLine(line, state)
			state = analysis.endState
			continue
		}

		noTrail := strings.TrimRight(line, " \t")
		if strings.TrimSpace(noTrail) == "" {
			out = append(out, "")
			continue
		}

		analysis := analyzeLine(noTrail, state)
		indent := indentLevel
		if analysis.startsWithClosingBrace {
			indent--
		}
		if indent < 0 {
			indent = 0
		}

		content := strings.TrimLeft(noTrail, " \t")
		out = append(out, strings.Repeat("    ", indent)+content)

		indentLevel += analysis.openBraces - analysis.closeBraces
		if indentLevel < 0 {
			indentLevel = 0
		}
		state = analysis.endState
	}

	// Trim extra trailing blank lines but always end with exactly one newline.
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n") + "\n"
}

func analyzeLine(line string, start formatState) lineAnalysis {
	a := lineAnalysis{endState: start}
	stringEsc := false
	templateEsc := false
	firstTokenSeen := false

	for i := 0; i < len(line); i++ {
		ch := line[i]

		if a.endState.inBlockComment {
			if ch == '*' && i+1 < len(line) && line[i+1] == '/' {
				a.endState.inBlockComment = false
				i++
			}
			continue
		}

		if a.endState.inString {
			if ch == '"' && !stringEsc {
				a.endState.inString = false
			}
			if ch == '\\' && !stringEsc {
				stringEsc = true
			} else {
				stringEsc = false
			}
			continue
		}

		if a.endState.inTemplate {
			if ch == '`' && !templateEsc {
				a.endState.inTemplate = false
			}
			if ch == '\\' && !templateEsc {
				templateEsc = true
			} else {
				templateEsc = false
			}
			continue
		}

		// Normal mode
		if ch == '/' && i+1 < len(line) && line[i+1] == '/' {
			break
		}
		if ch == '/' && i+1 < len(line) && line[i+1] == '*' {
			a.endState.inBlockComment = true
			i++
			continue
		}
		if ch == '"' {
			a.endState.inString = true
			stringEsc = false
			if !firstTokenSeen {
				firstTokenSeen = true
			}
			continue
		}
		if ch == '`' {
			a.endState.inTemplate = true
			templateEsc = false
			if !firstTokenSeen {
				firstTokenSeen = true
			}
			continue
		}

		if !firstTokenSeen {
			if ch == ' ' || ch == '\t' {
				continue
			}
			firstTokenSeen = true
			if ch == '}' {
				a.startsWithClosingBrace = true
			}
		}

		if ch == '{' {
			a.openBraces++
		} else if ch == '}' {
			a.closeBraces++
		}
	}

	return a
}
