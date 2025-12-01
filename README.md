
# 🍺 goTØV – Go Tæsse Øl Verksted

**goTØV** er et modulært og utvidbart bryggeri-automatiseringssystem skrevet i **Go**, integrert med  
**Beckhoff CX8190 / TwinCAT via OPC UA**, og med en Docker‑klar kjernestack (TimescaleDB, Grafana, MQTT).

---

## ⚙️ Quick Start

```bash
# Clone repo
git clone https://github.com/MrBoggi/goTOV.git
cd goTOV

# Install dependencies
go mod tidy

# Run backend (OPC UA edge controller)
go run ./cmd/gotov server
```

Eksempel output:

```
INF Connected to Beckhoff PLC via OPC UA
INF Temp_HLT = 133 (type int16)
```

---

## 🧩 Struktur

```
goTOV/
├── cmd/gotov/          # CLI-verktøy + server (brewfather, fermentation-db, osv.)
├── internal/api/       # Web API / WS
├── internal/opcua/     # OPC UA klient
├── internal/brewfather # Brewfather klient, parser, batch/recipe-funksjoner
├── internal/fermentation # Fermentering: SQLite store, typer, logikk
├── internal/logger     # Zerolog-baserte logger
├── internal/config     # YAML config loader
└── data/               # Lokale SQLite databaser
```

---

## 🧠 Highlights

- ⚡ Realtime OPC UA kommunikasjon mot Beckhoff CX8190  
- 🏷 Automatisk tag discovery (`ListSymbols`)  
- 🔌 Web API + WebSocket sanntidsstrøm  
- 🧪 Brewfather integrasjon (recipes + batches)  
- 🧬 Importer fermenteringsprofiler direkte til SQLite  
- 🧱 Ryddig Go‑arkitektur i `internal/`  

---

# 📦 Brewfather CLI – Fermentering & Batch-verktøy

goTØV inkluderer et komplett sett CLI‑kommandoer for å jobbe med Brewfather.

Kommandoene kan kjøres på to måter:
1.  **Med `go run`** (enklest for utvikling): `go run ./cmd/gotov <kommando>`
2.  **Som bygget binærfil** (anbefalt for jevnlig bruk):
    ```bash
    # Bygg binærfilen én gang
    go build -o gotov ./cmd/gotov

    # Kjør kommandoer direkte
    ./gotov <kommando>
    ```

Nedenfor brukes `go run`-metoden for enkelhets skyld.

---

## 🧪 1. List Brewfather batches

```bash
go run ./cmd/gotov brewfather-batches
```

Output:

```
BATCH ID                        NAME
KeRcvtkWQCgXyIC50pQkn1O0dDcY2b  Cactus Sombrero
CgQogdpjMQM75PfjDXHZBrd1LU8iWL  Medouche-aaah
...
```

Dette brukes til å finne batch-ID du ønsker å importere.

---

## 🍺 2. Importer fermenteringsprofil

```bash
go run ./cmd/gotov fermentation-import <batch-id>
```

Eksempel:

```bash
go run ./cmd/gotov fermentation-import KeRcvtkWQCgXyIC50pQkn1O0dDcY2b
```

Dette gjør:

1.  Henter batch fra Brewfather
2.  Tar fermenteringssteg fra batch → fallback recipe
3.  Konverterer time/days → *timer*
4.  Lagrer planen i **data/fermentation.db**

Output:

```
INF Fermentation plan imported successfully name=Cactus Sombrero steps=4
```

---

## 🗄️ 3. Sjekk lokal fermenteringsdatabase

List alle tilgjengelige planer:

```bash
go run ./cmd/gotov fermentation-db plans
```

List steg for én plan:

```bash
go run ./cmd/gotov fermentation-db steps <plan-id>
```


---

## 🧹 4. Tøm fermenteringsdatabase

```bash
go run ./cmd/gotov fermentation-db clear
```

---

## 🔧 Filplasseringer

| Fil | Beskrivelse |
|-----|-------------|
| `internal/brewfather/` | Brewfather API-klient, batch/recipe/fermentation parsing |
| `internal/fermentation/` | SQLite-lagring for fermenteringsplaner |
| `cmd/gotov/` | CLI‑kommandoer definert via Cobra |
| `config/config.yaml` | OPC UA + Brewfather config |

---

## 🔐 Configuration

I `config/config.yaml`:

```yaml
# OPC UA server
opcua:
  endpoint: "opc.tcp://192.168.1.10:4840"
  username: "your-username"
  password: "your-password"

# Web server
server:
  listen_addr: ":8080"

# Brewfather API
brewfather:
  user_id: "YOUR_USER_ID"
  api_key: "YOUR_API_KEY"
```

---

## 🐳 Docker

Prosjektet er fullt ut "dockerized" og kan kjøres med `docker-compose`.

```bash
# Bygg og start goTOV-containeren
docker-compose up --build
```

### Production Stack (TimescaleDB + Grafana)

For produksjon kan du aktivere `production`-profilen som inkluderer TimescaleDB og Grafana:

```bash
docker-compose --profile production up --build
```

Dette starter:
- `gotov`: Applikasjonen (port 8085)
- `timescaledb`: TimescaleDB database (port 5432)
- `grafana`: Grafana (port 3000)

---

## 🧱 Roadmap

| Status | Beskrivelse |
|--------|-------------|
| ✅ | OPC UA core, Brewfather import, SQLite |
| 🔧 | Fermenteringsmotor med step-tracking |
| 🔜 | GUI‑integrasjon |
| 🔮 | Full bryggeprosess motor |

---

---

## 🌐 API Endpoints

goTØV provides a RESTful API and a WebSocket stream for interacting with the system and OPC UA tags.

### Health Checks

*   **`GET /healthz`**
    *   **Description:** A basic health check endpoint.
    *   **Response:** `ok` (plain text)
    *   **Example (curl):**
        ```bash
        curl http://localhost:8080/healthz
        ```
    *   **Example Response:**
        ```
        ok
        ```

*   **`GET /health`**
    *   **Description:** Provides a health status in JSON format.
    *   **Response:** `{"status": "ok"}` (JSON)
    *   **Example (curl):**
        ```bash
        curl http://localhost:8080/health
        ```
    *   **Example Response:**
        ```json
        {"status": "ok"}
        ```

### System Information

*   **`GET /api/version`**
    *   **Description:** Returns the current version of the goTØV application.
    *   **Response:** `{"version": "..."}` (JSON)
    *   **Example (curl):**
        ```bash
        curl http://localhost:8080/api/version
        ```
    *   **Example Response:**
        ```json
        {"version": "v1.0.0"}
        ```

### OPC UA Tag Interaction

*   **`GET /api/tags`**
    *   **Description:** Returns a snapshot of the latest known OPC UA tag values.
    *   **Response:** A JSON object where keys are tag names and values are `WSMessage` objects.
        ```json
        {
            "Tag_Name_1": {
                "tag": "Tag_Name_1",
                "display_name": "Display Name 1",
                "value": "Current Value 1",
                "value_type": "string",
                "ts_ms": 1678886400000
            },
            "Tag_Name_2": {
                "tag": "Tag_Name_2",
                "display_name": "Display Name 2",
                "value": 123.45,
                "value_type": "float64",
                "ts_ms": 1678886400100
            }
        }
        ```
    *   **Example (curl):**
        ```bash
        curl http://localhost:8080/api/tags
        ```
    *   **Example Response:**
        ```json
        {
            "Mixer.Temp": {
                "tag": "Mixer.Temp",
                "display_name": "Mixer Temperature",
                "value": 25.5,
                "value_type": "float64",
                "ts_ms": 1701388800000
            },
            "Pump.Status": {
                "tag": "Pump.Status",
                "display_name": "Pump Operational Status",
                "value": true,
                "value_type": "bool",
                "ts_ms": 1701388800100
            }
        }
        ```

*   **`POST /api/write`**
    *   **Description:** Writes a new value to a specified OPC UA tag. If the tag does not start with `ns=`, `ns=4;s=` will be automatically prefixed.
    *   **Method:** `POST`
    *   **Request Body (JSON):**
        ```json
        {
            "tag": "Your.OPC.UA.Tag",
            "value": "New Value"
        }
        ```
    *   **Response:** `{"status": "ok"}` (JSON) on success.
    *   **Example (curl):**
        ```bash
        curl -X POST -H "Content-Type: application/json" \
             -d '{"tag": "Mixer.SetPoint", "value": 60.0}' \
             http://localhost:8080/api/write
        ```
    *   **Example Response:**
        ```json
        {"status": "ok"}
        ```

### Fermentation Plan Management

*   **`POST /api/fermentation/plan`**
    *   **Description:** Creates and saves a new fermentation plan.
    *   **Method:** `POST`
    *   **Request Body (JSON):**
        ```json
        {
            "name": "My Custom Fermentation Plan",
            "recipe_id": "CUSTOM_RECIPE_001",
            "steps": [
                {
                    "step_number": 1,
                    "temperature": 18.5,
                    "duration_hours": 72.0,
                    "description": "Primary Fermentation",
                    "type": "Ferment"
                },
                {
                    "step_number": 2,
                    "temperature": 2.0,
                    "duration_hours": 48.0,
                    "description": "Cold Crash",
                    "type": "Condition"
                }
            ]
        }
        ```
    *   **Response:** `{"status": "ok", "planID": <id>}` (JSON) on success, where `<id>` is the ID of the newly created plan.
    *   **Example (curl):**
        ```bash
        curl -X POST -H "Content-Type: application/json" \
             -d '{
                 "name": "IPA Fermentation Profile",
                 "recipe_id": "IPA_001",
                 "steps": [
                     {"step_number": 1, "temperature": 19.0, "duration_hours": 120.0, "description": "Active Fermentation", "type": "Ferment"},
                     {"step_number": 2, "temperature": 3.0, "duration_hours": 48.0, "description": "Dry Hop + Cold Crash", "type": "Condition"}
                 ]
             }' \
             http://localhost:8080/api/fermentation/plan
        ```
    *   **Example Response:**
        ```json
        {"status": "ok", "planID": 123}
        ```

### Real-time Tag Stream (WebSocket)

*   **`GET /api/stream/tags`**
    *   **Description:** Establishes a WebSocket connection to receive real-time updates for all subscribed OPC UA tags. Each message received will be a `WSMessage` object.
    *   **Protocol:** WebSocket (`ws://` or `wss://`)
    *   **Messages:** Each message is a JSON object representing a `WSMessage` struct:
        ```json
        {
            "tag": "Tag_Name",
            "display_name": "Display Name",
            "value": "Current Value",
            "value_type": "string",
            "ts_ms": 1678886400000
        }
        ```
    *   **Example (JavaScript - simplified):**
        ```javascript
        const ws = new WebSocket("ws://localhost:8080/api/stream/tags");

        ws.onopen = () => {
            console.log("WebSocket connected!");
        };

        ws.onmessage = (event) => {
            const data = JSON.parse(event.data);
            console.log("Received tag update:", data);
            // Example: { "tag": "Tank1.Level", "display_name": "Tank 1 Level", "value": 75.2, "value_type": "float64", "ts_ms": 1701388800500 }
        };

        ws.onclose = () => {
            console.log("WebSocket disconnected.");
        };

        ws.onerror = (error) => {
            console.error("WebSocket error:", error);
        };
        ```

---


---

© 2025 Tæsse ØlVerksted – Brew smarter 🍺
