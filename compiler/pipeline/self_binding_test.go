package pipeline

import (
	"regexp"
	"strings"
	"testing"

	front "httpdsl/compiler"
)

func parseProgramForTest(t *testing.T, src string) *front.Program {
	t.Helper()
	l := front.NewLexer(src)
	p := front.NewParser(l)
	prog := p.ParseProgram()
	if errs := p.Errors(); len(errs) > 0 {
		t.Fatalf("parse errors: %s", strings.Join(errs, "; "))
	}
	return prog
}

func TestGenerateCode_BindsSelfForObjectAnonymousFunctions(t *testing.T) {
	prog := parseProgramForTest(t, `
server {
  port 8080
}
init {
  profile = {
    name: "Bob",
    talk: fn() { return self.name },
    nested: {
      shout: fn() { return upper(self.name) }
    }
  }
}
route GET "/" {
  response.body = profile.talk()
}
`)

	out, err := GenerateCode(prog)
	if err != nil {
		t.Fatalf("expected generation success, got error: %v", err)
	}

	re := regexp.MustCompile(`var self Value = Value\((_t\d+)\)`)
	matches := re.FindAllStringSubmatch(out, -1)
	if len(matches) < 2 {
		t.Fatalf("expected at least two bound self declarations, got %d", len(matches))
	}
	root := matches[0][1]
	for _, m := range matches[1:] {
		if m[1] != root {
			t.Fatalf("expected nested object methods to bind to same root (%s), got %s", root, m[1])
		}
	}
}

func TestGenerateCode_RejectsSelfParameterInObjectAnonymousFunction(t *testing.T) {
	prog := parseProgramForTest(t, `
server { port 8080 }
init {
  obj = {
    bad: fn(self) { return self }
  }
}
route GET "/" { response.body = "ok" }
`)

	_, err := GenerateCode(prog)
	if err == nil {
		t.Fatalf("expected generation error for self parameter")
	}
	if !strings.Contains(err.Error(), "parameter name 'self' is reserved") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGenerateCode_RejectsSelfAssignmentInObjectAnonymousFunction(t *testing.T) {
	prog := parseProgramForTest(t, `
server { port 8080 }
init {
  obj = {
    bad: fn() {
      self = "x"
    }
  }
}
route GET "/" { response.body = "ok" }
`)

	_, err := GenerateCode(prog)
	if err == nil {
		t.Fatalf("expected generation error for self assignment")
	}
	if !strings.Contains(err.Error(), "cannot assign to reserved identifier 'self'") {
		t.Fatalf("unexpected error: %v", err)
	}
}
