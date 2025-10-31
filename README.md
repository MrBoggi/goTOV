# 🍺 goTØV – Go Tæsse Øl Verksted (Quick Start)

**goTØV** is a modular and extensible brewery automation system written in **Go**, integrating **Beckhoff CX8190 / TwinCAT** via **OPC UA**, and a **Dockerized core stack** (TimescaleDB, Grafana, MQTT).

---

## ⚙️ Quick Start

```bash
# Clone repo
git clone https://github.com/MrBoggi/goTOV.git
cd goTOV

# Install dependencies
go mod tidy

# Run backend (OPC UA edge controller)
go run ./cmd/server
```

Example output:
```
INF Connected to Beckhoff PLC via OPC UA
INF Temp_HLT = 133 (type int16)
```

---

## 🧩 Structure

```
goTOV/
├── cmd/server/        # OPC UA backend (edge)
├── internal/opcua/    # OPC UA client implementation
├── internal/logger/   # Structured logging (zerolog)
├── internal/config/   # YAML config loader
└── deployments/       # Docker stack (core)
```

---

## 🧠 Highlights

- ⚡ Real‑time OPC UA communication with Beckhoff PLC
- 🔧 Built‑in namespace browser (`BrowseNamespace(4)`)
- ☁️ Docker‑ready core stack (TimescaleDB, Grafana, MQTT)
- 🧱 Clean modular Go design (`internal/` packages)

---

## 🧱 Roadmap

| Phase | Description |
|--------|-------------|
| ✅ v0.1 | Stable OPC UA connection & tag browser |
| 🔄 v0.2 | Process logic (heat, pump, valves) |
| 🔜 v0.3 | MQTT / TimescaleDB integration |
| 🔮 v1.0 | Web UI + recipe management |

---

MIT License © 2025 Morten Bogetvedt
