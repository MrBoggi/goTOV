
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
go run ./cmd/server
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
├── cmd/gotov/          # CLI-verktøy (brewfather, fermentation-db, osv.)
├── cmd/server/         # OPC UA backend (edge controller)
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
Dette inkluderer:

- liste batcher  
- hente ut recipes  
- importere fermenteringsprofiler  
- inspisere lokale fermenteringsplaner  
- slette / nullstille databasen  

Alle kommandoer kjøres slik:

```bash
go run ./cmd/gotov <command> [...]
```

eller bygget:

```bash
./gotov <command> [...]
```

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

1. Henter batch fra Brewfather  
2. Tar fermenteringssteg fra batch → fallback recipe  
3. Konverterer time/days → *timer*  
4. Lagrer planen i **data/fermentation.db**

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

## 🔐 Config – Brewfather

I `config/config.yaml`:

```yaml
brewfather:
  user_id: "YOUR_USER_ID"
  api_key: "YOUR_API_KEY"
```

---

## 🧱 Roadmap

| Status | Beskrivelse |
|--------|-------------|
| ✅ | OPC UA core, Brewfather import, SQLite |
| 🔧 | Fermenteringsmotor med step-tracking |
| 🔜 | GUI‑integrasjon |
| 🔮 | Full bryggeprosess motor |

---

© 2025 Tæsse ØlVerksted – Brew smarter 🍺
