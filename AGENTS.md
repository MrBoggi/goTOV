# AGENTS.md — AI Development Contract for goTOV

This repository uses AI agents with modular **Skills**.

Skills provide domain expertise and must be loaded only when required.

Goals:

* minimal token usage
* deterministic code generation
* consistent architecture
* safe brewery hardware interaction

---

# 1. AI Execution Workflow (MANDATORY)

All tasks must follow this workflow:

1. Read AGENTS.md
2. Read AI_RULES.md if architecture context is required
3. Identify required Skills
4. Load only necessary Skills
5. Analyze existing code
6. Follow implementation plan
7. Implement minimal changes
8. Write tests
9. Run lint and tests
10. Create Pull Request

If requirements are unclear:

STOP and ask.

Never guess.

---

# 2. Skills System

All expert knowledge is stored in:

```
Skills/
```

Agents must **load Skills dynamically** depending on the task.

Do not load Skills that are not required.

---

## Available Skills

Examples:

```
Skills/api-design-principles
Skills/api-patterns
Skills/api-security
Skills/backend-development
Skills/clean-code
Skills/docker-expert
Skills/git-workflow
Skills/golang-pro
Skills/planner
Skills/postgres-best-practices
Skills/release-manager
Skills/testing-patterns
```

Each Skill contains:

* domain rules
* coding patterns
* best practices
* anti-patterns

---

# 3. Skill Selection Rules

Agents must select Skills based on task type.

Examples:

API endpoint work

```
api-design-principles
api-patterns
golang-pro
testing-patterns
```

Database work

```
postgres-best-practices
backend-development
golang-pro
```

Refactoring

```
clean-code
golang-pro
testing-patterns
```

Infrastructure

```
docker-expert
backend-development
```

Planning tasks

```
planner
```

Release tasks

```
release-manager
git-workflow
```

---

# 4. Token Efficiency Rules

To reduce token usage:

Agents must:

* load only required Skills
* avoid repeating repository context
* avoid rewriting full files
* prefer minimal diffs
* avoid long explanations

---

# 5. Repository Architecture

Repository structure:

```
cmd/
internal/api/
internal/service/
internal/repository/
internal/opcua/
internal/models/
data/
Skills/
```

Architecture flow:

Handler → Service → Repository

Rules:

Handlers must not access database directly.

Services contain business logic.

Repositories contain SQL.

PLC interaction only allowed in:

```
internal/opcua
```

---

# 6. Code Generation Rules

Generated code must:

* compile
* follow repository architecture
* follow Go conventions
* include proper error handling
* include imports
* include tests where required

Forbidden:

```
TODO
placeholder code
rest of code
omitted code
```

---

# 7. Diff Strategy

Agents must prefer **minimal diffs**.

Avoid:

* rewriting entire files
* unnecessary renaming
* structural rewrites

Large changes require justification.

---

# 8. Testing

All new logic requires tests.

Load Skill:

```
Skills/testing-patterns
```

Test requirements:

* table-driven tests
* success case
* error case
* edge case

Run:

```
go test ./...
```

---

# 9. Git Workflow

Use Skill:

```
Skills/git-workflow
```

Base branch:

```
development
```

Branch naming:

```
feature/<name>
fix/<name>
refactor/<name>
```

Commits must follow Conventional Commits.

---

# 10. Hardware Safety

This system interacts with brewery hardware.

PLC communication uses OPC UA.

Before writing PLC tags:

* verify tag name
* verify type
* verify namespace
* confirm implementation in internal/opcua

Unsafe hardware interaction must be rejected.

---

# 11. Security

Never hardcode:

* credentials
* API keys
* tokens
* passwords

Use environment variables.

---

# 12. Anti-Hallucination Rules

Agents must never:

* invent APIs
* invent database schema
* invent PLC tags
* invent repository patterns

If missing information:

consult AI_RULES.md
search repository
or ask the user.

---

# Core Principle

Prefer small, safe, deterministic changes over large refactors.

Load only the Skills needed for the task.

Brew smarter. Code safer. 🍺

## graphify

This project has a knowledge graph at graphify-out/ with god nodes, community structure, and cross-file relationships.

When the user types `/graphify`, use the installed graphify skill or instructions before doing anything else.

Rules:
- For codebase questions, first run `graphify query "<question>"` when graphify-out/graph.json exists. Use `graphify path "<A>" "<B>"` for relationships and `graphify explain "<concept>"` for focused concepts. These return a scoped subgraph, usually much smaller than GRAPH_REPORT.md or raw grep output.
- Dirty graphify-out/ files are expected after hooks or incremental updates; dirty graph files are not a reason to skip graphify. Only skip graphify if the task is about stale or incorrect graph output, or the user explicitly says not to use it.
- If graphify-out/wiki/index.md exists, use it for broad navigation instead of raw source browsing.
- Read graphify-out/GRAPH_REPORT.md only for broad architecture review or when query/path/explain do not surface enough context.
- After modifying code, run `graphify update .` to keep the graph current (AST-only, no API cost).
