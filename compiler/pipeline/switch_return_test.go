package pipeline

import (
	"regexp"
	"strings"
	"testing"
)

func TestGenerateCode_PreservesFunctionReturnSemanticsInsideSwitch(t *testing.T) {
	prog := parseProgramForTest(t, `
server { port 8080 }

fn choose(kind) {
  switch kind {
    case "one" {
      return "picked"
    }
    default {
      return "fallback"
    }
  }
}

route GET "/" {
  response.body = choose("one")
}
`)

	out, err := GenerateCode(prog)
	if err != nil {
		t.Fatalf("expected generation success, got error: %v", err)
	}

	re := regexp.MustCompile(`func fn_choose\([^)]*\) Value \{\n(?s:(.*?))\n\}`)
	match := re.FindStringSubmatch(out)
	if len(match) != 2 {
		t.Fatalf("expected generated choose function in output")
	}
	body := match[1]
	if strings.Contains(body, "response =") {
		t.Fatalf("expected function switch return to stay local, got route-style response assignment:\n%s", body)
	}
	if !strings.Contains(body, `return Value("picked")`) {
		t.Fatalf("expected case return in generated function body, got:\n%s", body)
	}
	if !strings.Contains(body, `return Value("fallback")`) {
		t.Fatalf("expected default return in generated function body, got:\n%s", body)
	}
}
