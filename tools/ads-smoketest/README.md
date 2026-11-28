# ADS-smoketest

Dette er en isolert, read-only kontroll av ADS-kommunikasjon mot bryggeri-PLC-en.
Den er ikke en del av goTØVs produksjonsintegrasjon og utfører ingen ADS-skriving,
ruteregistrering eller PLC-endringer.

Testen verifiserer:

1. ADS/TCP-tilkobling til `192.168.10.150:48898`.
2. `ReadState()` mot ADS-runtime `851`.
3. Symboloppslag for `MAIN.fbUA`.
4. Én samlet rålesing av `MAIN.fbUA` (120 byte) og dekoding av et lite utvalg
   `INT`, `REAL` og `BOOL` basert på dokumenterte offsets.

`MAIN.fbUA` er kun et midlertidig testgrensesnitt. Produksjonsgrensesnittet skal
erstattes av en dedikert ADS-struktur på PLC-en.

## ADS-rute

PLC-en må ha en rute som tillater den lokale AMS Net ID-en og IP-adressen fra
kjøremiljøet. Standard lokal AMS-adresse er `192.168.10.115.1.1:10500`; `10500`
er en logisk AMS-kildeport, ikke en eksponert TCP-port. Overstyr Net ID ved behov:

```sh
ADS_LOCAL_NETID=192.168.10.115.1.1 go run .
```

## Kjøring

Smoketesten er en selvstendig Go-modul og bruker `github.com/siyka-au/go-ads/v3`.
Modulen krever Go 1.26.2. Kjør fra denne katalogen:

```sh
cd tools/ads-smoketest
go test ./...
go run .
```

Kjøring oppretter nettverkstrafikk mot PLC-en; bruk den bare når ADS-ruten er
klar og kontrollen er ønsket. Byggartefakten `ads-smoketest` skal ikke versjoneres.
