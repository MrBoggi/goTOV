# AGENTS.md - Context & Rules for AI Agents

Welcome to **goTOV** (Go Tæsse Øl Verksted). You are assisting in the development of a modular brewery automation system.

---

## 🎯 Project Overview
- **Core Goal**: Real-time automation and monitoring of brewery hardware.
- **Hardware Integration**: Beckhoff CX8190 / TwinCAT via **OPC UA**.
- **Data Stack**: SQLite (Fermentation), TimescaleDB (Production Metrics), Grafana (Visualization), MQTT (Communication).
- **Integrations**: Brewfather API (Recipes & Batches).

---

## 🛠 Technology Stack & Best Practices

### 1. Go (Golang) - Strict Rules
- **Version**: Go 1.21+
- **Structure**:
    - `cmd/`: CLI tools and the main server.
    - `internal/`: All core logic (private to the project).
    - `data/`: Database files (SQLite/etc).
- **Coding Standards**:
    - Use `gofmt` / `goimports`.
    - **Error Handling**: Never ignore errors (`_`). Wrap with context: `fmt.Errorf("context: %w", err)`.
    - **Context**: Pass `context.Context` as the first argument for I/O and PLC calls.
    - **Concurrency**: Use standard patterns (Channels, WaitGroups) for real-time tag updates.

### 2. Git Workflow (MANDATORY)
- **Base Branch**: `development`.
- **Model**: Feature Branching.
    1. Checkout `development` and pull latest.
    2. Create branch: `feature/`, `fix/`, or `refactor/`.
    3. Implement, verify (tests), and commit (Conventional Commits).
    4. Merge back to `development` and delete feature branch.
- **NEVER** commit directly to `main` or `development`.

---

## 🤖 Agent Behavioral Rules

### 🛡 Safety & Hardware Caution
- **PLC Interaction**: Be extremely cautious when suggesting writes to OPC UA tags. Verify tag names and data types against `internal/opcua` or existing symbols.
- **Simulation**: If hardware is not available, suggest or use mock/simulation patterns where possible.

### 🧩 Proactivity & Quality
- **Testing**: Always check if tests exist for the package you are modifying. Run `go test ./...` frequently.
- **CLI first**: Prefer using the existing CLI tool (`./cmd/gotov`) for administrative tasks or data imports.
- **Documentation**: Keep `README.md` and `API` documentation updated as you add features.

---

## 📁 Key File Map
- [GEMINI.md](file:///c:/Repos/goTOV/GEMINI.md): Original strict rules for the Gemini Agent.
- [README.md](file:///c:/Repos/goTOV/README.md): System architecture and Quickstart.
- `internal/opcua/`: PLC communication logic.
- `internal/brewfather/`: Recipe/Batch import logic.
- `internal/fermentation/`: Fermentation tracking and SQLite storage.

---

"Brew smarter, code safer. 🍺"
