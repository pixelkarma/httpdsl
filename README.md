# HTTPDSL

HTTPDSL is a domain-specific language for building HTTP backends that compile to native Go binaries.

You write `.httpdsl` files, run the compiler, and ship a single executable.

## What this project is

HTTPDSL is a language + compiler aimed at practical web backends:

- HTTP routes and groups
- request/response handling
- middleware (`before {}` / `after {}`)
- sessions + CSRF
- SSE
- scheduling (`every` interval/cron)
- databases and key/value store helpers

It compiles through an internal pipeline (`.httpdsl -> AST -> IR -> Go source -> binary`) so deployment remains standard: run one binary.

## Why it was created

The project exists to reduce backend boilerplate and speed up development without giving up native binaries.

Typical Go services are powerful but repetitive for common API tasks (routing, body parsing, validation, response shaping, middleware wiring, and startup orchestration). HTTPDSL packages those patterns into a smaller language surface so teams can move faster for the 80% case.

## Advantages

- Less code for common backend patterns
- Faster iteration for CRUD/API-heavy services
- Built-in language features for app concerns (routes, middleware, init/shutdown, jobs)
- Compiles to native Go binaries (operationally familiar)
- Good readability for mixed-skill teams

## Disadvantages

- Less flexible than writing raw Go directly
- Smaller ecosystem than general-purpose languages
- Compile step is required (not interpreted)
- Global-like project scope requires naming/structure discipline
- Some advanced/custom behaviors still need lower-level workarounds

## Best fit

HTTPDSL works best for:

- JSON APIs and internal tools
- admin dashboards and back-office apps
- community/content apps with straightforward request lifecycles
- webhook/event intake services
- real-time update endpoints with SSE

It is less ideal when you need deep custom protocol work, highly specialized runtime behavior, or heavy framework/library integration beyond the language surface.

## Documentation

- Language docs: [docs/readme.md](docs/readme.md)
- Compiler docs: [docs/compiler.md](docs/compiler.md)
- Project style guide: [docs/style-guide.md](docs/style-guide.md)
- Examples: [examples/](examples/)
- VS Code extension: [vscode-httpdsl/](vscode-httpdsl/)

## Basic example (HTTPDSL)

```httpdsl
server {
  port 8080
}

fn NewUserProfile(name, email, bio) {
  return {
    name: trim(name),
    email: lower(trim(email)),
    bio: bio ?? "",

    validate: fn() {
      errors = []
      if len(self.name) < 2 { push(errors, "Name must be at least 2 characters") }
      if !is_email(self.email) { push(errors, "Email is invalid") }
      return {ok: len(errors) == 0, errors: errors}
    },

    public: fn() {
      return {name: self.name, email: self.email, bio: self.bio}
    }
  }
}

init {
  Profiles = {}
}

route POST "/profiles" json {
  profile = NewUserProfile(
    request.data.name ?? "",
    request.data.email ?? "",
    request.data.bio ?? ""
  )

  result = profile.validate()
  if !result.ok {
    response.status = 400
    response.body = {error: "validation failed", fields: result.errors}
    return
  }

  id = cuid2()
  Profiles[id] = profile.public()
  response.status = 201
  response.body = {id: id, profile: Profiles[id]}
}

route GET "/profiles/:id" {
  profile = Profiles[request.params.id] ?? null
  if profile == null {
    response.status = 404
    response.body = {error: "not found"}
    return
  }
  response.body = profile
}
```

## Equivalent Go (one possible implementation)

```go
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Profile struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Bio   string `json:"bio"`
}

type createProfileRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Bio   string `json:"bio"`
}

var (
	profiles = map[string]Profile{}
	mu       sync.RWMutex
)

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func validateProfile(p Profile) []string {
	var errs []string
	if len(strings.TrimSpace(p.Name)) < 2 {
		errs = append(errs, "Name must be at least 2 characters")
	}
	if !strings.Contains(p.Email, "@") {
		errs = append(errs, "Email is invalid")
	}
	return errs
}

func createProfileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	var req createProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}

	p := Profile{
		Name:  strings.TrimSpace(req.Name),
		Email: strings.ToLower(strings.TrimSpace(req.Email)),
		Bio:   req.Bio,
	}

	if errs := validateProfile(p); len(errs) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":  "validation failed",
			"fields": errs,
		})
		return
	}

	id := fmt.Sprintf("%d", time.Now().UnixNano())
	mu.Lock()
	profiles[id] = p
	mu.Unlock()

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":      id,
		"profile": p,
	})
}

func getProfileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/profiles/")
	if id == "" || id == r.URL.Path {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}

	mu.RLock()
	p, ok := profiles[id]
	mu.RUnlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
		return
	}

	writeJSON(w, http.StatusOK, p)
}

func main() {
	http.HandleFunc("/profiles", createProfileHandler)
	http.HandleFunc("/profiles/", getProfileHandler)

	log.Println("listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
```

The Go version is still straightforward, but HTTPDSL expresses the same behavior with fewer moving parts and less setup code.

## Getting started

```bash
go build -o httpdsl .
./httpdsl build server.httpdsl
./server
```

For project-mode builds, create an `app.httpdsl` entry file and run:

```bash
./httpdsl build
```

In project mode, the output binary is named after the project directory.
