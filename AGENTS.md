# AGENTS.md - Context & Rules for AI Agents

Welcome to **goTOV** (Go Tæsse Øl Verksted). You are assisting in the development of a modular brewery automation system.

---

## 🎯 Project Overview
- **Core Goal**: Real-time automation and monitoring of brewery hardware.
- **Hardware Integration**: Beckhoff CX8190 / TwinCAT via **OPC UA**.
- **Data Stack**: SQLite (Fermentation), TimescaleDB (Production Metrics), Grafana (Visualization), MQTT (Communication).
- **Integrations**: Brewfather API (Recipes & Batches).

---

## 🤖 Agent Configuration

For this project, we utilize specialized agent roles to ensure high quality and consistency.

### Skill: Architect (Planning)
- **Role**: Analyze Git Issues and create detailed implementation plans.
- **Model**: `gemini-3.1-pro-high`
- **Responsibility**: Generate step-by-step plans in `ISSUE_TEMPLATE` or `implementation_plan.md`.

### Skill: Coder (Implementation)
- **Role**: Write code based on an approved plan.
- **Model**: `gemini-3-flash`
- **Responsibility**: Execute file changes, handle logic, and create Pull Requests.

### Skill: Reviewer (Validation)
- **Role**: Validate that the code matches the plan and follows all project rules.
- **Model**: `claude-4.6-sonnet-thinking`
- **Responsibility**: Perform code reviews and verify tests.

---

## 🛠 Technology Stack & Absolute Rules

### 1. Go (Golang) - Strict Rules
- **Version**: Go 1.21+
- **Structure**:
    - `cmd/`: CLI tools and the main server.
    - `internal/`: All core logic (private to the project).
    - `data/`: Database files (SQLite/etc).
- **Coding Standards**:
    - **Formatting**: Always run `gofmt` or `goimports` on changed files before commit.
    - **Error Handling**: Never ignore errors (`_`). Wrap with context: `fmt.Errorf("context: %w", err)`.
    - **Context**: Pass `context.Context` as the first argument for I/O, database, and PLC calls.
    - **Naming**: Follow standard Go conventions (CamelCase, descriptive names).
    - **JSON Naming**: Always use **camelCase** for JSON tags. Suffixes like "ID" should be lowercase "id" (e.g., `batchId`).
    - **API Documentation**: Update `handleGetApiDocs` when changing endpoints. Ensure JSON keys match struct tags.

### 2. Git Workflow (MANDATORY)
- **Base Branch**: `development`.
- **Model**: Feature Branching.
    1. **Start**: Ensure you are on `development` and pull latest.
    2. **Branch**: Create `feature/`, `fix/`, or `refactor/` branch.
    3. **Work**: Implement changes and add unit tests for new functionality.
    4. **Verify**: Run `go test ./...` and ensure all tests pass.
    5. **Commit**: Use Conventional Commits (e.g., `feat: ...`, `fix: ...`).
    6. **Push & PR**: Push to origin and create a PR against `development`. **NEVER** merge directly to `development` or `main` locally.

### 3. Terminal & Commands
- **Execution**: Wait for commands to finish before proceeding.
- **Errors**: If a command fails (e.g., merge conflict, compilation error), **STOP** and resolve it or ask for guidance. Do not guess.
- **Caution**: Be extremely careful with `run_shell_command` on destructive actions.

---

## 🛡 Performance & Safety
- **PLC Interaction**: Be extremely cautious when writing to OPC UA tags. Verify names and types against `internal/opcua`.
- **Placeholder Rule**: Never use placeholders like `// ... rest of code`. Write complete, working code.
- **Security**: Never hardcode credentials. Use environment variables.
- **Proactivity**: Always check for existing tests and run them frequently.

---

## 📁 Key Resources & Skills
- **Skills Directory**: [Skills/](file:///c:/Repos/goTOV/Skills/) - Contains specialized workflows for Git, Go, Testing, etc.
- [README.md](file:///c:/Repos/goTOV/README.md): System architecture and Quickstart.
- `internal/opcua/`: PLC communication logic.
- `internal/fermentation/`: Fermentation tracking and SQLite storage.

---

"Brew smarter, code safer. 🍺"
