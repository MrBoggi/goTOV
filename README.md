# 🍺 goTØV – GoTæsseØlVerksted

**goTØV** (Go Tæsse Øl Verksted) er et moderne, modulært og skalerbart bryggeri-automatiseringssystem skrevet i **Go**.  
Prosjektet kombinerer **Beckhoff CX8190 / TwinCAT** for felt-I/O, **Go (goADS)** for edge-kontroll og sekvenslogikk, og en **Docker-basert core-stack** (TimescaleDB, Grafana, MQTT) for logging, oppskrifter og visualisering.

Prosjektet er bygget for å være:
- 💡 **Fleksibelt** – edge + core-arkitektur  
- ⚡ **Sanntidsnært** – direkte ADS-tilkobling til Beckhoff I/O  
- ☁️ **Skyvennlig** – kjører i Docker med moderne stack  
- 🍻 **Utvidbart** – nye noder for gjæring, kjøling og tapping kan enkelt legges til  

---

## 🧰 Kom i gang – utviklingsmiljø

Disse instruksjonene setter opp et komplett Go-utviklingsmiljø for **goTØV**, slik at du kan kompilere, kjøre og bidra til prosjektet.

### 🔧 1. Krav

| Komponent | Minimumversjon | Beskrivelse |
|------------|----------------|-------------|
| **Go** | 1.23+ | Kompileringsverktøy og runtime |
| **Git** | Nyeste | Versjonskontroll |
| **VS Code** | Nyeste | IDE for utvikling |
| **Docker Desktop** | Nyeste | Brukes for core-stack |
| **TwinCAT 3** | 3.1+ | Kjøres på Beckhoff CX8190 PLC |

---

### ⚙️ 2. Klon prosjektet

```bash
cd C:\Repos
git clone https://github.com/MrBoggi/goTOV.git
cd goTOV
```

---

### 🧱 3. Sett opp Go-moduler

```bash
go mod tidy
```

Dette laster alle nødvendige biblioteker, blant annet:
- `goADS` (Beckhoff ADS-protokoll)
- `paho.mqtt.golang` (MQTT)
- `pgx/v5` (TimescaleDB-driver)

---

### 🧪 4. Kjør edge-delen (lokalt)

Test at edge-delen fungerer ved å kjøre:

```bash
go run ./cmd/edge
```

Du skal se:
```
goTØV Edge running…
```

Edge-applikasjonen kommuniserer mot Beckhoff PLC-en via **ADS** og publiserer verdier til **MQTT**.

---

### 🐳 5. Kjør core-stack (Docker)

Core-delen består av backend + database + dashboard.  
Start hele pakken fra `deployments/docker-compose.yml`:

```bash
cd deployments
docker compose up -d
```

Dette starter:
- `timescaledb` – tidsseriedatabase for logging  
- `grafana` – visualisering og dashboards  
- `brewcore` – Go-backend for oppskrifter og data  

---

### 🖥️ 6. Åpne i VS Code

```bash
code C:\Repos\goTOV
```

Anbefalte VS Code-utvidelser:
- Go (Google)
- Docker
- GitLens
- YAML
- Markdown Preview Enhanced

---

## 🧩 Oppsett av Beckhoff ADS

For at **goADS** skal kunne kommunisere med PLC-en, må **AMS-nettverket** være riktig satt opp.  
Dette gjelder spesielt hvis du kjører goTØV Edge fra IPC eller PC i LAN-et.

### ⚙️ 1. Sjekk AMS-adresse på PLC
I TwinCAT XAE:  
**System → AMS Router → AMS Net ID**  
Eksempel: `5.44.1.1.1.1`

### ⚙️ 2. Tillat ekstern klient
På PLC-en (CX8190):
1. Åpne *TwinCAT System Manager* eller *TC/BSD Web Interface*
2. Gå til **Access Control / AMS Router Table**
3. Legg til IPC-ens IP og AMS ID  
   (Eks: IP: `192.168.1.100`, AMS: `192.168.1.100.1.1`)

### ⚙️ 3. Test forbindelsen
Fra PC med goTØV:
```bash
ads-ping 5.44.1.1.1.1
```
Eller kjør Go-testen:
```go
conn, err := goADS.NewConnection("5.44.1.1.1.1", 851)
```
Hvis den kobler uten feil → ADS-kommunikasjonen fungerer.

---

## 🧩 Arkitektur

```
                      🏠 Docker Core-server (AMP)
 ┌─────────────────────────────────────────────────────────┐
 │  brewcore (Go)   – MQTT → TimescaleDB                   │
 │  TimescaleDB     – historikk og batchlogging            │
 │  Grafana         – dashboards / overvåkning             │
 │  Mosquitto       – meldingshub                          │
 └───────────────▲──────────────────────────────────────────┘
                 │ MQTT / HTTPS
 ┌───────────────┴──────────────────────────────────────────┐
 │ IPC – goTØV Edge Client (Go)                             │
 │  • Leser og skriver ADS-variabler mot CX8190             │
 │  • Publiserer verdier til MQTT                           │
 │  • Tar imot kommandoer fra Core                          │
 └───────────────▲──────────────────────────────────────────┘
                 │ ADS/TCP
 ┌───────────────┴──────────────────────────────────────────┐
 │ Beckhoff CX8190 (TwinCAT runtime)                        │
 │  • EtherCAT til EL2008, EL3218, EL4028, EL3058           │
 │  • Eksponerer GVL-variabler over ADS                     │
 └──────────────────────────────────────────────────────────┘
```

---

## 📦 Prosjektstruktur

```
goTOV/
├── cmd/
│   ├── edge/              # Go ADS + MQTT klient (IPC)
│   └── core/              # Core backend (Docker)
├── internal/              # Biblioteker for ads/mqtt/db/logic
├── deployments/           # Docker-compose og service-konfig
├── docs/                  # Dokumentasjon, arkitektur, topics
└── twinCAT/               # GVL-filer og symbolmapping fra PLC
```

---

## 📜 Lisens

MIT License © 2025 Morten Bogetvedt  

---

## ☕️ Bidra

Pull requests, idéer og forslag er velkomne!  
Prosjektet er i aktiv utvikling – målsettingen er å gjøre **bryggeri-automatisering i Go** like elegant som i TwinCAT, bare friere 🍺  
