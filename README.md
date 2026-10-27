# goTØV

goTØV er backend for bryggeriets prosess- og vedlikeholdsfunksjoner. Tjenesten tilbyr HTTP- og WebSocket-API, lagrer driftsdata i SQLite og kommuniserer med PLC-en via OPC UA.

## Lokal kjøring

Krever Go-versjonen angitt i `go.mod` og en konfigurasjonsfil.

```bash
go test ./...
go run ./cmd/gotov server
```

Eksempelkonfigurasjon ligger i `config/config.example.yaml`. Ikke legg passord, sertifikater eller produksjonsendepunkter i Git.

## Helse og API

`GET /health` svarer bare på om backendprosessen er oppe. PLC-tilkobling og ADS-telemetri eksponeres separat, slik at en levende prosess ikke feilaktig tolkes som tilgjengelig prosesskontroll.

Viktige endepunkter:

- `GET /health` – prosesshelse.
- `GET /api/ads/status` – ADS V1-leserens tilstand, siste gyldige prøve og eventuelle feil.
- `/api/fermentation/*` – gjæringsplaner, aktive gjæringer og kontrollstatus.
- `/ws` – WebSocket-oppdateringer til GUI.

## PLC-integrasjon

### OPC UA – aktiv kontrollvei

OPC UA er den aktive veien for gjæringskontroll. Den historiske gjæringsmotoren kjører i backend når serveren starter og gjenoppretter aktive gjæringer fra SQLite. Den leser temperaturer og skriver ventiler, varme og glykolpumpe via de etablerte UA-nodene.

Temperaturregulering bruker reseptens settpunkt med ±0,3 °C hysterese. For hver tank kan kjøling og varme aldri være aktive samtidig. Glykolkoordinatoren bruker følgende sekvens:

1. Slå av tankens varme før kjølebehovet publiseres.
2. Åpne korrekt kjøleventil og bekreft lokal tilstand.
3. Start først pumpen etter forsinkelse og bare når minst én ventil er bekreftet åpen.
4. Ved stopp: stans pumpen før ventiler stenges.

Dermed skal pumpen ikke startes mot lukkede ventiler. Flere tanker kan kjøle samtidig, og en annen tank kan varme, men én og samme tank kan ikke varme og kjøle samtidig.

### ADS V1 – kun lesing og observasjon

ADS V1 er en valgfri, feature-flaggstyrt leser av de binære PLC-inputene. Den kan slås på med `ads.enabled`, men den skriver aldri PLC-utganger og erstatter ikke OPC UA-kontrollen. Status finnes på `/api/ads/status`.

Det versjonerte grensesnittet, binary-layout, testvektorer og kompatibilitetsregler eies av [`goTOV-Interfaces/ads/v1`](../goTOV-Interfaces/ads/v1/). Endringer på tvers av repoer skal gjøres der først. ADS V2 er foreløpig en spesifikasjon; den autoriserer ikke produksjonsskriving.

## Data og gjenoppretting

SQLite lagrer gjæringsplaner, aktive steg, hendelser, tilstandsdata og historikk. Ved oppstart leser motoren aktive gjæringer fra databasen. Den synkroniserer deretter kjent pumpe-, ventil- og varmetilstand fra PLC-en før den fortsetter reguleringen.

## Utvikling

Kjør før pull request:

```bash
gofmt -w .
go test ./...
go vet ./...
go build ./...
```

Produksjonsutrulling skjer kun via GitHub Actions etter merge til `main`. Ikke bygg eller start produksjonscontainere manuelt på runneren. Etter deploy verifiseres prosessen med `GET http://192.168.10.115:18085/health`; dette er ikke en godkjenning av PLC-skriving.

## Videre ADS-arbeid

ADS-skriving, valg mellom UA og ADS i GUI, og PLC-eierskap kan først innføres når PLC-prosjektet har en eksplisitt ADS-mottaker, watchdog/timeout, arbitrering mot eksisterende UA-skriving og en bekreftet sikker fallback. Disse kontraktene og testene skal versjoneres i `goTOV-Interfaces` før backend eller GUI bruker dem.
