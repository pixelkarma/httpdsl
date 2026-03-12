# HTTPDSL Project Style Guide

- [Why This Guide Exists](#why-this-guide-exists)
- [Core Model: Global by Design](#core-model-global-by-design)
- [Rule A: Exactly One `server {}` Per Project](#rule-a-exactly-one-server--per-project)
- [Rule B: Keep Folders Shallow Until You Need More](#rule-b-keep-folders-shallow-until-you-need-more)
- [Rule C: Use PascalCase for User-Defined Names](#rule-c-use-pascalcase-for-user-defined-names)
- [Cross-File Blocks and Ordering](#cross-file-blocks-and-ordering)
- [Working With Global Scope Safely](#working-with-global-scope-safely)
- [Recommended Layouts](#recommended-layouts)
- [Quick Checklist](#quick-checklist)

## Why This Guide Exists

HTTPDSL is intentionally simple: top-level blocks define app behavior, and `init {}` defines shared runtime state.

That simplicity is a strength, but only if project structure is disciplined. This guide explains conventions that keep multi-file codebases readable, conflict-free, and easy to maintain.

## Core Model: Global by Design

HTTPDSL behaves like a global orchestration language:

- `init {}` creates shared variables available across routes, middleware, schedulers, and functions.
- Top-level blocks are merged across files into one project-level program.
- There is no file-level namespace boundary by default.

This model is good for velocity because data access and control flow are direct. You avoid wiring, imports, and dependency plumbing. The tradeoff is naming and structure discipline: without conventions, collisions and hidden coupling are easy.

## Rule A: Exactly One `server {}` Per Project

Projects should define one and only one `server {}` block.

Why:

- Server config is global application policy (port, TLS, sessions, static mounts, CORS).
- Multiple `server {}` blocks create ambiguity over which policy wins.
- A single block gives one obvious place to audit runtime behavior.

Recommendation:

- Treat duplicate `server {}` blocks as a compile error.
- Keep the `server {}` block in `app.httpdsl` (or another single root file).

## Rule B: Keep Folders Shallow Until You Need More

Default to a flat top level. Split into folders only when a concern becomes large enough to justify it.

Why:

- Flat layouts are faster to scan and grep in small and medium projects.
- Premature nesting spreads related logic across too many paths.
- Controlled splitting preserves clarity as code grows.

Practical convention:

- Start with files like `app.httpdsl`, `routes.httpdsl`, `jobs.httpdsl`.
- When routes become large, introduce `routes/`.
- In split route projects, keep shared route middleware in `routes/routes.httpdsl` (for route-wide `before {}` / `after {}`).

## Rule C: Use PascalCase for User-Defined Names

Use PascalCase for user-defined functions and shared globals.

Why:

- System/builtin identifiers are lowercase, so PascalCase visually separates app code from language/runtime helpers.
- In a global symbol space, PascalCase plus domain prefixes reduces accidental collisions.
- Consistent casing improves scanability in mixed files.

Suggested pattern:

- `<Domain><Action><Target>` for functions, for example: `UsersGetById`, `AuthCreateSession`, `TransitRefreshFeed`.
- `<Domain><Concept>` for shared globals, for example: `UsersDB`, `AppConfig`, `TransitCache`.

## Cross-File Blocks and Ordering

Top-level blocks can exist in any `.httpdsl` file. That includes `init {}`, `before {}`, `after {}`, routes, groups, and schedulers.

Why this is useful:

- You can keep feature concerns together (`routes/users.httpdsl` can include related helper `fn` blocks and a feature-local `init {}`).
- You can build pseudo-namespaced globals by grouping init state by domain (`UsersStore`, `AuthConfig`, `TransitFeedCache`) in domain files.

Ordering rule to remember:

- Entry file (`app.httpdsl`) is loaded first.
- Beyond that, cross-file load order is not guaranteed and should not be treated as stable.

Practical implication:

- Do not rely on one non-entry file's `init {}` running before another non-entry file's `init {}`.
- If order matters, keep ordering-sensitive setup in `app.httpdsl` or call explicit setup functions from a single coordinating `init {}`.

## Working With Global Scope Safely

Global-like access is useful, but mutations should be predictable.

- Put project-wide shared assignments in `init {}` only.
- Avoid implicit cross-file writes to the same global unless intentional and documented.
- Keep `before {}` focused on request setup/validation.
- Keep `after {}` for side effects only (logging, metrics, async enqueue). Do not rely on changing `response` there, because the response is already finalized.
- Keep business logic in `fn` blocks so routes stay declarative and short.

## Recommended Layouts

Small project:

```text
.
├── app.httpdsl        # single server {} + shared init
├── routes.httpdsl     # all routes/groups
└── jobs.httpdsl       # every {} blocks, shutdown {}
```

Growing project:

```text
.
├── app.httpdsl
├── routes/
│   ├── routes.httpdsl # shared route-level before/after
│   ├── users.httpdsl
│   └── auth.httpdsl
├── jobs/
│   └── jobs.httpdsl
└── views/
    └── templates.gohtml
```

Large project (still shallow-first):

```text
.
├── app.httpdsl
├── routes/
│   ├── routes.httpdsl
│   ├── users.httpdsl
│   ├── auth.httpdsl
│   └── feed.httpdsl
├── domain/
│   ├── users.httpdsl
│   ├── auth.httpdsl
│   └── feed.httpdsl
└── jobs/
    ├── cron.httpdsl
    └── maintenance.httpdsl
```

## Quick Checklist

- Exactly one `server {}` in the project.
- Shared state created in `init {}` (no top-level loose assignments).
- Flat layout first; split folders only when files get too large.
- PascalCase for user-defined functions and globals.
- Domain-prefixed names to avoid global symbol collisions.
- `after {}` used for side effects, not response mutation.
