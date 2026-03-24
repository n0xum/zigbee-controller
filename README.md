# zigbee-controller

Lokale Zigbee-zu-HomeKit-Bridge: verbindet IKEA KAJPLATS-Lampen und BILRESA-Scrollrad via Zigbee2MQTT mit Apple HomeKit. Kein Cloud-Zugriff.

## Voraussetzungen

- Docker + Docker Compose
- Go 1.24+
- Sonoff ZBDongle-E eingesteckt

## 1. USB-Pfad ermitteln

```bash
ls /dev/ttyUSB* /dev/ttyACM*   # vor dem Einstecken
# Dongle einstecken, dann nochmal:
ls /dev/ttyUSB* /dev/ttyACM*   # neuer Eintrag = Dongle-Pfad
```

## 2. Geräte in Zigbee-Modus versetzen (einmalig!)

**KAJPLATS-Lampen (2x):** Strom **12x** ein/aus schalten (~1s Pausen). Lampe blinkt kurz kaltweiß = Zigbee-Pairing aktiv.

**BILRESA-Scrollrad:** Reset-Knopf (Rückseite) **8x** drücken. LED blinkt = Pairing aktiv.

## 3. Konfiguration

```bash
cp config.example.yaml config.yaml
```

USB-Pfad in `docker-compose.yml` und `zigbee2mqtt/configuration.yaml` anpassen (Standard: `/dev/ttyUSB0`).

## 4. Docker starten

```bash
make docker-up
```

## 5. Geräte pairen

Zigbee2MQTT Web-UI öffnen: http://localhost:8080

Geräte erscheinen nach dem Reset automatisch. `friendly_name` notieren.

`config.yaml` unter `devices.bulbs` und `devices.remotes` ausfüllen.

## 6. Pairing abschließen

In `zigbee2mqtt/configuration.yaml` setzen:
```yaml
permit_join: false
```

```bash
make docker-up   # Konfiguration neu einlesen
```

## 7. BILRESA simulated_brightness einrichten

IEEE-Adresse des BILRESA aus Z2M-UI kopieren.
In `zigbee2mqtt/configuration.yaml` eintragen:

```yaml
devices:
  '0xXXXXXXXXXXXXXXXX':   # IEEE-Adresse hier eintragen
    simulated_brightness:
      delta: 20
      interval: 200
```

```bash
make docker-up
```

## 8. Bridge starten

```bash
make run
```

Im Terminal erscheint PIN und Pairing-URI.

## 9. HomeKit koppeln

Apple Home App → **+** → **Gerät hinzufügen** → PIN eingeben oder URI scannen.

## Makefile-Befehle

| Befehl | Beschreibung |
|---|---|
| `make build` | Binary nach `bin/bridge` kompilieren |
| `make run` | Bridge direkt starten |
| `make docker-up` | Mosquitto + Zigbee2MQTT starten |
| `make docker-logs` | Zigbee2MQTT-Logs verfolgen |
| `make tidy` | Go-Dependencies bereinigen |
