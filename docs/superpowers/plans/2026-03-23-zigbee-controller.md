# Zigbee Controller Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a local Go application that bridges Zigbee2MQTT (via MQTT) to Apple HomeKit using the HAP protocol, controlling IKEA KAJPLATS bulbs and BILRESA scroll wheel remote.

**Architecture:** A HAP bridge exposes all Zigbee devices as HomeKit accessories. An MQTT client subscribes to Zigbee2MQTT topics and synchronises state bidirectionally. Automation logic (dimmer) runs in-process, driven by BILRESA scroll events received from MQTT.

**Tech Stack:** Go 1.26, `github.com/brutella/hap`, `github.com/eclipse/paho.mqtt.golang`, `github.com/spf13/viper`, `log/slog`, Docker (Mosquitto + Zigbee2MQTT)

---

## File Map

| File | Responsibility |
|---|---|
| `go.mod` / `go.sum` | Module definition and locked deps |
| `.gitignore` | Extend with project-specific ignores |
| `config.example.yaml` | Committed example config |
| `config.yaml` | Local config (gitignored) |
| `mosquitto.conf` | Mosquitto broker config |
| `docker-compose.yml` | Mosquitto + Zigbee2MQTT services |
| `zigbee2mqtt/configuration.yaml` | Z2M bootstrap config |
| `Makefile` | Build / run / docker targets |
| `README.md` | Full setup guide |
| `internal/config/config.go` | Load & validate YAML config via Viper |
| `internal/config/config_test.go` | Config validation tests |
| `internal/mqtt/client.go` | MQTT connect/reconnect/pub/sub interface |
| `internal/mqtt/client_test.go` | Interface-based unit tests |
| `internal/zigbee/device.go` | `Device` interface + `Registry` |
| `internal/zigbee/bulb.go` | KAJPLATS JSON payload ↔ HomeKit mapping |
| `internal/zigbee/bulb_test.go` | Payload parsing and command tests |
| `internal/zigbee/remote.go` | BILRESA action parser |
| `internal/zigbee/remote_test.go` | Action mapping tests |
| `internal/homekit/bridge.go` | HAP bridge setup, QR output |
| `internal/homekit/bulb_accessory.go` | Lightbulb accessory (On/Brightness/ColorTemp) |
| `internal/homekit/remote_accessory.go` | StatelessProgrammableSwitch accessory |
| `internal/automation/dimmer.go` | Scroll wheel → brightness ticker |
| `internal/automation/dimmer_test.go` | Dimmer tick / clamp / stop tests |
| `cmd/bridge/main.go` | Wire everything, graceful shutdown |

---

## Task 1: Project Scaffold

**Files:**
- Modify: `.gitignore`
- Create: `go.mod`, `config.example.yaml`, `mosquitto.conf`, `docker-compose.yml`, `zigbee2mqtt/configuration.yaml`, `Makefile`

- [ ] **Step 1: Extend .gitignore**

Append to `.gitignore`:
```
# Project-specific
hap-data/
bin/
config.yaml
zigbee2mqtt/database.db
zigbee2mqtt/coordinator_backup.json
zigbee2mqtt/devices.yaml
```

- [ ] **Step 2: Create go.mod**

```
module github.com/ak/zigbee-controller

go 1.24
```

Run: `go mod tidy` (will fail until deps are added — OK for now, skip)

- [ ] **Step 3: Create config.example.yaml**

```yaml
mqtt:
  broker: "tcp://localhost:1883"
  client_id: "zigbee-controller"
  username: ""
  password: ""

homekit:
  pin: "11122333"
  name: "Zigbee Bridge"
  storage_path: "./hap-data"

devices:
  bulbs:
    - friendly_name: "kajplats_1"
      display_name: "Wohnzimmer Lampe"
    - friendly_name: "kajplats_2"
      display_name: "Schlafzimmer Lampe"
  remotes:
    - friendly_name: "bilresa_1"
      display_name: "BILRESA Scrollrad"
      controls_bulbs: ["kajplats_1", "kajplats_2"]
```

- [ ] **Step 4: Create mosquitto.conf**

```
listener 1883
allow_anonymous true
persistence false
```

- [ ] **Step 5: Create docker-compose.yml**

```yaml
services:
  mosquitto:
    image: eclipse-mosquitto:2
    ports:
      - "1883:1883"
    volumes:
      - ./mosquitto.conf:/mosquitto/config/mosquitto.conf

  zigbee2mqtt:
    image: koenkk/zigbee2mqtt:latest
    depends_on:
      - mosquitto
    volumes:
      - ./zigbee2mqtt:/app/data
      - /run/udev:/run/udev:ro
    devices:
      - /dev/ttyUSB0:/dev/ttyUSB0
    environment:
      - TZ=Europe/Berlin
    restart: unless-stopped
```

- [ ] **Step 6: Create zigbee2mqtt/configuration.yaml**

```yaml
homeassistant: false
permit_join: true
mqtt:
  base_topic: zigbee2mqtt
  server: mqtt://mosquitto
serial:
  port: /dev/ttyUSB0
  adapter: ezsp
advanced:
  log_level: info
  transmit_power: 20
frontend:
  enabled: true
  port: 8080
# Nach dem Pairing IEEE-Adresse des BILRESA hier eintragen:
# devices:
#   '0xXXXXXXXXXXXXXXXX':
#     simulated_brightness:
#       delta: 20
#       interval: 200
```

- [ ] **Step 7: Create Makefile**

```makefile
.PHONY: build run docker-up docker-logs tidy

build:
	go build -o bin/bridge ./cmd/bridge

run:
	go run ./cmd/bridge

docker-up:
	docker compose up -d

docker-logs:
	docker compose logs -f zigbee2mqtt

tidy:
	go mod tidy
```

- [ ] **Step 8: Commit scaffold**

```bash
git add .gitignore go.mod config.example.yaml mosquitto.conf docker-compose.yml zigbee2mqtt/configuration.yaml Makefile
git commit -m "chore: project scaffold, docker config, Makefile"
```

---

## Task 2: Config Package

**Files:**
- Create: `internal/config/config.go`, `internal/config/config_test.go`

Installs dep: `github.com/spf13/viper`

- [ ] **Step 1: Write the failing test**

`internal/config/config_test.go`:
```go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ak/zigbee-controller/internal/config"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad_ValidConfig(t *testing.T) {
	path := writeConfig(t, `
mqtt:
  broker: "tcp://localhost:1883"
  client_id: "test"
homekit:
  pin: "11122333"
  name: "Test Bridge"
  storage_path: "./hap-data"
devices:
  bulbs:
    - friendly_name: "b1"
      display_name: "Bulb 1"
  remotes:
    - friendly_name: "r1"
      display_name: "Remote 1"
      controls_bulbs: ["b1"]
`)
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MQTT.Broker != "tcp://localhost:1883" {
		t.Errorf("broker mismatch: %s", cfg.MQTT.Broker)
	}
	if len(cfg.Devices.Bulbs) != 1 {
		t.Errorf("expected 1 bulb, got %d", len(cfg.Devices.Bulbs))
	}
	if cfg.Devices.Remotes[0].ControlsBulbs[0] != "b1" {
		t.Errorf("controls_bulbs mismatch")
	}
}

func TestLoad_MissingBroker(t *testing.T) {
	path := writeConfig(t, `
mqtt:
  broker: ""
homekit:
  pin: "11122333"
  name: "Test"
  storage_path: "./hap-data"
`)
	_, err := config.Load(path)
	if err == nil {
		t.Fatal("expected error for missing broker")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := config.Load("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
cd /Users/ak/IdeaProjects/zigbee-controller
go test ./internal/config/... 2>&1
```
Expected: compile error — package doesn't exist yet.

- [ ] **Step 3: Implement config.go**

`internal/config/config.go`:
```go
// Package config lädt und validiert die Anwendungskonfiguration aus einer YAML-Datei.
package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// MQTTConfig enthält MQTT-Verbindungsparameter.
type MQTTConfig struct {
	Broker   string `mapstructure:"broker"`
	ClientID string `mapstructure:"client_id"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

// HomeKitConfig enthält HAP-Serverparameter.
type HomeKitConfig struct {
	PIN         string `mapstructure:"pin"`
	Name        string `mapstructure:"name"`
	StoragePath string `mapstructure:"storage_path"`
}

// BulbConfig beschreibt eine einzelne Lampe.
type BulbConfig struct {
	FriendlyName string `mapstructure:"friendly_name"`
	DisplayName  string `mapstructure:"display_name"`
}

// RemoteConfig beschreibt ein Scrollrad-Gerät.
type RemoteConfig struct {
	FriendlyName  string   `mapstructure:"friendly_name"`
	DisplayName   string   `mapstructure:"display_name"`
	ControlsBulbs []string `mapstructure:"controls_bulbs"`
}

// DevicesConfig enthält alle konfigurierten Geräte.
type DevicesConfig struct {
	Bulbs   []BulbConfig   `mapstructure:"bulbs"`
	Remotes []RemoteConfig `mapstructure:"remotes"`
}

// Config ist die Hauptkonfigurationsstruktur.
type Config struct {
	MQTT    MQTTConfig    `mapstructure:"mqtt"`
	HomeKit HomeKitConfig `mapstructure:"homekit"`
	Devices DevicesConfig `mapstructure:"devices"`
}

// Load liest die YAML-Konfigurationsdatei unter path und validiert sie.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("konfigurationsdatei lesen: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("konfiguration parsen: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// validate prüft, ob alle Pflichtfelder gesetzt sind.
func validate(cfg *Config) error {
	if cfg.MQTT.Broker == "" {
		return fmt.Errorf("mqtt.broker darf nicht leer sein")
	}
	if cfg.HomeKit.PIN == "" {
		return fmt.Errorf("homekit.pin darf nicht leer sein")
	}
	if cfg.HomeKit.Name == "" {
		return fmt.Errorf("homekit.name darf nicht leer sein")
	}
	if cfg.HomeKit.StoragePath == "" {
		return fmt.Errorf("homekit.storage_path darf nicht leer sein")
	}
	return nil
}
```

- [ ] **Step 4: Install dep and run tests**

```bash
cd /Users/ak/IdeaProjects/zigbee-controller
go get github.com/spf13/viper
go test ./internal/config/... -v
```
Expected: all 3 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/ go.mod go.sum
git commit -m "feat: config package with viper loading and validation"
```

---

## Task 3: MQTT Client

**Files:**
- Create: `internal/mqtt/client.go`, `internal/mqtt/client_test.go`

Installs dep: `github.com/eclipse/paho.mqtt.golang`

- [ ] **Step 1: Write the failing test**

`internal/mqtt/client_test.go`:
```go
// Package mqtt_test testet das MQTT-Client-Interface.
package mqtt_test

import (
	"sync"
	"testing"
	"time"

	mqttclient "github.com/ak/zigbee-controller/internal/mqtt"
)

// mockClient implementiert mqttclient.Client für Tests.
type mockClient struct {
	mu        sync.Mutex
	published []mqttclient.Message
	handlers  map[string]mqttclient.MessageHandler
}

func (m *mockClient) Subscribe(topic string, handler mqttclient.MessageHandler) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.handlers == nil {
		m.handlers = make(map[string]mqttclient.MessageHandler)
	}
	m.handlers[topic] = handler
	return nil
}

func (m *mockClient) Publish(topic string, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.published = append(m.published, mqttclient.Message{Topic: topic, Payload: payload})
	return nil
}

func (m *mockClient) Disconnect() {}

// simuliert eingehende Nachricht
func (m *mockClient) receive(topic string, payload []byte) {
	m.mu.Lock()
	h := m.handlers[topic]
	m.mu.Unlock()
	if h != nil {
		h(mqttclient.Message{Topic: topic, Payload: payload})
	}
}

func TestClient_Interface(t *testing.T) {
	// Stellt sicher, dass mockClient das Interface erfüllt
	var _ mqttclient.Client = &mockClient{}
}

func TestClient_PublishCollected(t *testing.T) {
	c := &mockClient{}
	_ = c.Publish("test/topic", []byte(`{"state":"ON"}`))

	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(c.published))
	}
	if c.published[0].Topic != "test/topic" {
		t.Errorf("topic mismatch: %s", c.published[0].Topic)
	}
}

func TestClient_SubscribeAndReceive(t *testing.T) {
	c := &mockClient{}
	received := make(chan mqttclient.Message, 1)

	_ = c.Subscribe("test/topic", func(msg mqttclient.Message) {
		received <- msg
	})

	c.receive("test/topic", []byte(`{"state":"OFF"}`))

	select {
	case msg := <-received:
		if string(msg.Payload) != `{"state":"OFF"}` {
			t.Errorf("payload mismatch: %s", msg.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
go test ./internal/mqtt/... 2>&1
```
Expected: compile error.

- [ ] **Step 3: Implement mqtt/client.go**

`internal/mqtt/client.go`:
```go
// Package mqtt stellt einen MQTT-Client mit automatischem Reconnect bereit.
package mqtt

import (
	"fmt"
	"log/slog"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
)

// Message kapselt eine eingehende oder ausgehende MQTT-Nachricht.
type Message struct {
	Topic   string
	Payload []byte
}

// MessageHandler wird bei eingehenden Nachrichten aufgerufen.
type MessageHandler func(msg Message)

// Client definiert das Interface für MQTT-Operationen.
type Client interface {
	Subscribe(topic string, handler MessageHandler) error
	Publish(topic string, payload []byte) error
	Disconnect()
}

// pahoClient ist die produktive Implementierung über Eclipse Paho.
type pahoClient struct {
	inner paho.Client
	log   *slog.Logger
}

// Config enthält Verbindungsparameter für NewClient.
type Config struct {
	Broker   string
	ClientID string
	Username string
	Password string
}

// NewClient verbindet sich mit dem MQTT-Broker und gibt einen Client zurück.
// Bei Verbindungsabbruch wird automatisch mit exponentiellem Backoff reconnectet.
func NewClient(cfg Config, log *slog.Logger) (Client, error) {
	opts := paho.NewClientOptions().
		AddBroker(cfg.Broker).
		SetClientID(cfg.ClientID).
		SetAutoReconnect(true).
		SetMaxReconnectInterval(60 * time.Second).
		SetConnectRetryInterval(2 * time.Second).
		SetConnectRetry(true).
		SetOnConnectHandler(func(_ paho.Client) {
			log.Info("MQTT verbunden", "broker", cfg.Broker)
		}).
		SetConnectionLostHandler(func(_ paho.Client, err error) {
			log.Warn("MQTT-Verbindung verloren, reconnect...", "fehler", err)
		})

	if cfg.Username != "" {
		opts.SetUsername(cfg.Username).SetPassword(cfg.Password)
	}

	c := paho.NewClient(opts)

	// Erster Verbindungsversuch mit exponentiellem Backoff (1s, 2s, 4s … max 60s)
	backoff := []time.Duration{1, 2, 4, 8, 16, 30, 60, 60, 60, 60}
	var lastErr error
	for attempt, delay := range backoff {
		if token := c.Connect(); token.Wait() && token.Error() != nil {
			lastErr = token.Error()
			log.Warn("MQTT-Verbindung fehlgeschlagen, warte...",
				"versuch", attempt+1, "delay", delay*time.Second, "fehler", lastErr)
			time.Sleep(delay * time.Second)
			continue
		}
		return &pahoClient{inner: c, log: log}, nil
	}
	return nil, fmt.Errorf("MQTT-Verbindung nach mehreren Versuchen fehlgeschlagen: %w", lastErr)
}

// Subscribe abonniert ein Topic und ruft handler bei eingehenden Nachrichten auf.
func (c *pahoClient) Subscribe(topic string, handler MessageHandler) error {
	token := c.inner.Subscribe(topic, 0, func(_ paho.Client, m paho.Message) {
		handler(Message{Topic: m.Topic(), Payload: m.Payload()})
	})
	token.Wait()
	return token.Error()
}

// Publish sendet eine Nachricht an das angegebene Topic.
func (c *pahoClient) Publish(topic string, payload []byte) error {
	token := c.inner.Publish(topic, 0, false, payload)
	token.Wait()
	return token.Error()
}

// Disconnect trennt die MQTT-Verbindung sauber.
func (c *pahoClient) Disconnect() {
	c.inner.Disconnect(250)
	c.log.Info("MQTT getrennt")
}
```

- [ ] **Step 4: Install dep and run tests**

```bash
go get github.com/eclipse/paho.mqtt.golang
go test ./internal/mqtt/... -v
```
Expected: all 3 tests PASS (interface, publish, subscribe).

- [ ] **Step 5: Commit**

```bash
git add internal/mqtt/ go.mod go.sum
git commit -m "feat: MQTT client with auto-reconnect and Client interface"
```

---

## Task 4: Zigbee Device Registry

**Files:**
- Create: `internal/zigbee/device.go`

No new deps.

- [ ] **Step 1: Write the failing test**

`internal/zigbee/device_test.go`:
```go
package zigbee_test

import (
	"testing"

	"github.com/ak/zigbee-controller/internal/zigbee"
)

// stubDevice ist ein minimales Device für Registry-Tests, ohne Abhängigkeit zu BulbDevice.
type stubDevice struct{ name string }

func (s *stubDevice) Name() string         { return s.name }
func (s *stubDevice) HandleMessage([]byte) {}

func TestRegistry_RegisterAndLookup(t *testing.T) {
	r := zigbee.NewRegistry()
	r.Register(&stubDevice{name: "kajplats_1"})

	got, ok := r.Lookup("kajplats_1")
	if !ok {
		t.Fatal("erwartetes Gerät nicht gefunden")
	}
	if got.Name() != "kajplats_1" {
		t.Errorf("Name falsch: %s", got.Name())
	}
}

func TestRegistry_LookupUnknown(t *testing.T) {
	r := zigbee.NewRegistry()
	_, ok := r.Lookup("unbekannt")
	if ok {
		t.Fatal("unbekanntes Gerät sollte nicht gefunden werden")
	}
}

func TestRegistry_All(t *testing.T) {
	r := zigbee.NewRegistry()
	r.Register(&stubDevice{name: "b1"})
	r.Register(&stubDevice{name: "b2"})
	if len(r.All()) != 2 {
		t.Errorf("erwartet 2 Geräte, bekommen %d", len(r.All()))
	}
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
go test ./internal/zigbee/... 2>&1
```
Expected: compile error.

- [ ] **Step 3: Implement device.go**

`internal/zigbee/device.go`:
```go
// Package zigbee definiert Zigbee-Gerät-Interfaces und die Device-Registry.
package zigbee

import "sync"

// Device ist das Interface, das alle Zigbee-Geräte implementieren müssen.
type Device interface {
	// Name gibt den friendly_name des Geräts zurück.
	Name() string
	// HandleMessage verarbeitet eingehende Zigbee2MQTT-Nachrichten.
	HandleMessage(payload []byte)
}

// Registry hält alle registrierten Zigbee-Geräte.
type Registry struct {
	mu      sync.RWMutex
	devices map[string]Device
}

// NewRegistry erstellt eine leere Device-Registry.
func NewRegistry() *Registry {
	return &Registry{devices: make(map[string]Device)}
}

// Register fügt ein Gerät zur Registry hinzu.
func (r *Registry) Register(d Device) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.devices[d.Name()] = d
}

// Lookup sucht ein Gerät anhand seines friendly_name.
func (r *Registry) Lookup(name string) (Device, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.devices[name]
	return d, ok
}

// All gibt alle registrierten Geräte zurück.
func (r *Registry) All() []Device {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Device, 0, len(r.devices))
	for _, d := range r.devices {
		out = append(out, d)
	}
	return out
}
```

- [ ] **Step 4: Run test**

```bash
go test ./internal/zigbee/... -run TestRegistry -v
```
Expected: all 3 Registry tests PASS (the test compiles independently because it uses `stubDevice`, not `BulbDevice`).

- [ ] **Step 5: Commit**

```bash
git add internal/zigbee/device.go internal/zigbee/device_test.go
git commit -m "feat: zigbee Device interface and Registry"
```

---

## Task 5: Bulb Handler (KAJPLATS)

**Files:**
- Create: `internal/zigbee/bulb.go`, `internal/zigbee/bulb_test.go`
- The device_test.go from Task 4 also becomes runnable here.

- [ ] **Step 1: Write the failing test**

`internal/zigbee/bulb_test.go`:
```go
package zigbee_test

import (
	"encoding/json"
	"testing"

	"github.com/ak/zigbee-controller/internal/zigbee"
)

func TestBulbDevice_Name(t *testing.T) {
	b := &zigbee.BulbDevice{FriendlyName: "kajplats_1"}
	if b.Name() != "kajplats_1" {
		t.Errorf("Name falsch: %s", b.Name())
	}
}

func TestBulbDevice_ParseState_On(t *testing.T) {
	b := &zigbee.BulbDevice{FriendlyName: "kajplats_1"}
	payload := []byte(`{"state":"ON","brightness":254,"color_temp":370,"linkquality":80}`)
	b.HandleMessage(payload)

	on, br, ct := b.GetState()
	if !on {
		t.Error("Lampe sollte AN sein")
	}
	if br != 254 {
		t.Errorf("Helligkeit falsch: %d", br)
	}
	if ct != 370 {
		t.Errorf("Farbtemperatur falsch: %d", ct)
	}
}

func TestBulbDevice_ParseState_Off(t *testing.T) {
	b := &zigbee.BulbDevice{FriendlyName: "kajplats_1"}
	b.HandleMessage([]byte(`{"state":"OFF"}`))
	on, _, _ := b.GetState()
	if on {
		t.Error("Lampe sollte AUS sein")
	}
}

func TestBulbDevice_BrightnessToHomeKit(t *testing.T) {
	// 254 Z2M → 100% HomeKit
	hk := zigbee.Z2MBrightnessToHomeKit(254)
	if hk != 100 {
		t.Errorf("erwartet 100, bekommen %d", hk)
	}
	// 0 Z2M → 0% HomeKit
	hk = zigbee.Z2MBrightnessToHomeKit(0)
	if hk != 0 {
		t.Errorf("erwartet 0, bekommen %d", hk)
	}
}

func TestBulbDevice_BrightnessFromHomeKit(t *testing.T) {
	// 100% HomeKit → 254 Z2M
	z2m := zigbee.HomeKitBrightnessToZ2M(100)
	if z2m != 254 {
		t.Errorf("erwartet 254, bekommen %d", z2m)
	}
}

func TestBulbDevice_SetCommand(t *testing.T) {
	b := &zigbee.BulbDevice{FriendlyName: "kajplats_1"}
	b.SetState(true, 128, 370)
	cmd := b.SetCommand()
	var m map[string]interface{}
	if err := json.Unmarshal(cmd, &m); err != nil {
		t.Fatal(err)
	}
	if m["state"] != "ON" {
		t.Errorf("state falsch: %v", m["state"])
	}
}

func TestBulbDevice_HandleInvalidJSON(t *testing.T) {
	b := &zigbee.BulbDevice{FriendlyName: "kajplats_1"}
	// Kein Panic bei ungültigem JSON
	b.HandleMessage([]byte(`nicht-json`))
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
go test ./internal/zigbee/... 2>&1
```
Expected: compile error — BulbDevice undefined.

- [ ] **Step 3: Implement bulb.go**

`internal/zigbee/bulb.go`:
```go
// Package zigbee enthält gerätespezifische Handler für Zigbee2MQTT-Payloads.
package zigbee

import (
	"encoding/json"
	"log/slog"
	"sync"
)

// bulbPayload ist das eingehende JSON-Format von Zigbee2MQTT für KAJPLATS-Lampen.
type bulbPayload struct {
	State      string `json:"state"`
	Brightness int    `json:"brightness"`
	ColorTemp  int    `json:"color_temp"`
}

// BulbSetCommand ist das ausgehende JSON-Format für Zigbee2MQTT-Set-Commands.
type BulbSetCommand struct {
	State      string  `json:"state"`
	Brightness int     `json:"brightness,omitempty"`
	ColorTemp  int     `json:"color_temp,omitempty"`
	Transition float64 `json:"transition,omitempty"`
}

// BulbDevice repräsentiert eine KAJPLATS-Lampe mit aktuellem Zustand.
// Felder für Zustand sind privat; Zugriff nur über GetState/SetState (thread-safe).
type BulbDevice struct {
	FriendlyName string
	DisplayName  string

	mu         sync.RWMutex
	on         bool
	brightness int // 0–254 (Zigbee2MQTT-Skala)
	colorTemp  int // 153–500 mired

	// OnStateChange wird aufgerufen, wenn sich der Zustand ändert.
	OnStateChange func(b *BulbDevice)
	log           *slog.Logger
}

// Name implementiert das Device-Interface.
func (b *BulbDevice) Name() string { return b.FriendlyName }

// HandleMessage verarbeitet eingehende Zigbee2MQTT-Statuspayloads.
func (b *BulbDevice) HandleMessage(payload []byte) {
	var p bulbPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		if b.log != nil {
			b.log.Warn("Ungültiges JSON von Lampe",
				"device_name", b.FriendlyName,
				"fehler", err,
			)
		}
		return
	}

	b.mu.Lock()
	b.on = p.State == "ON"
	if p.Brightness > 0 {
		b.brightness = p.Brightness
	}
	if p.ColorTemp > 0 {
		b.colorTemp = p.ColorTemp
	}
	cb := b.OnStateChange
	b.mu.Unlock()

	if cb != nil {
		cb(b)
	}
}

// SetCommand erzeugt einen Zigbee2MQTT-Set-Command mit Zustand, Helligkeit und Farbtemperatur.
// Nicht für den Dimmer verwenden — dort BrightnessOnlyCommand() nutzen.
func (b *BulbDevice) SetCommand() []byte {
	b.mu.RLock()
	defer b.mu.RUnlock()

	state := "OFF"
	if b.on {
		state = "ON"
	}
	cmd := BulbSetCommand{
		State:      state,
		Brightness: b.brightness,
		ColorTemp:  b.colorTemp,
	}
	data, _ := json.Marshal(cmd)
	return data
}

// BrightnessOnlyCommand erzeugt einen Zigbee2MQTT-Set-Command nur mit Helligkeit.
// Wird vom Dimmer verwendet, um die KAJPLATS-Einschränkung einzuhalten:
// Brightness und ColorTemp dürfen niemals gleichzeitig mit transition > 0 gesendet werden.
func (b *BulbDevice) BrightnessOnlyCommand() []byte {
	b.mu.RLock()
	defer b.mu.RUnlock()

	state := "OFF"
	if b.on {
		state = "ON"
	}
	cmd := BulbSetCommand{
		State:      state,
		Brightness: b.brightness,
	}
	data, _ := json.Marshal(cmd)
	return data
}

// GetState gibt den aktuellen Zustand thread-safe zurück.
func (b *BulbDevice) GetState() (on bool, brightness, colorTemp int) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.on, b.brightness, b.colorTemp
}

// SetState setzt den Zustand direkt (für HomeKit → MQTT Richtung).
func (b *BulbDevice) SetState(on bool, brightness, colorTemp int) {
	b.mu.Lock()
	b.on = on
	if brightness > 0 {
		b.brightness = brightness
	}
	if colorTemp > 0 {
		b.colorTemp = colorTemp
	}
	b.mu.Unlock()
}

// Z2MBrightnessToHomeKit wandelt 0–254 in 0–100% um.
func Z2MBrightnessToHomeKit(z2m int) int {
	if z2m <= 0 {
		return 0
	}
	if z2m >= 254 {
		return 100
	}
	return int(float64(z2m) / 254.0 * 100.0)
}

// HomeKitBrightnessToZ2M wandelt 0–100% in 0–254 um.
func HomeKitBrightnessToZ2M(hk int) int {
	if hk <= 0 {
		return 0
	}
	if hk >= 100 {
		return 254
	}
	return int(float64(hk) / 100.0 * 254.0)
}
```

- [ ] **Step 4: Run all zigbee tests**

```bash
go test ./internal/zigbee/... -v
```
Expected: all tests PASS (device registry + bulb tests).

- [ ] **Step 5: Commit**

```bash
git add internal/zigbee/
git commit -m "feat: BulbDevice with Z2M payload parsing and HomeKit brightness mapping"
```

---

## Task 6: Remote Handler (BILRESA)

**Files:**
- Create: `internal/zigbee/remote.go`, `internal/zigbee/remote_test.go`

- [ ] **Step 1: Write the failing test**

`internal/zigbee/remote_test.go`:
```go
package zigbee_test

import (
	"testing"

	"github.com/ak/zigbee-controller/internal/zigbee"
)

func TestRemoteDevice_Name(t *testing.T) {
	r := &zigbee.RemoteDevice{FriendlyName: "bilresa_1"}
	if r.Name() != "bilresa_1" {
		t.Errorf("Name falsch: %s", r.Name())
	}
}

func TestRemoteDevice_ActionOn(t *testing.T) {
	called := ""
	r := &zigbee.RemoteDevice{
		FriendlyName: "bilresa_1",
		OnAction: func(a zigbee.RemoteAction) { called = string(a) },
	}
	r.HandleMessage([]byte(`{"action":"on"}`))
	if called != "on" {
		t.Errorf("erwartet 'on', bekommen '%s'", called)
	}
}

func TestRemoteDevice_ActionBrightnessMoveUp(t *testing.T) {
	called := zigbee.RemoteAction("")
	r := &zigbee.RemoteDevice{
		FriendlyName: "bilresa_1",
		OnAction: func(a zigbee.RemoteAction) { called = a },
	}
	r.HandleMessage([]byte(`{"action":"brightness_move_up"}`))
	if called != zigbee.ActionBrightnessMoveUp {
		t.Errorf("erwartet brightness_move_up, bekommen '%s'", called)
	}
}

func TestRemoteDevice_ActionRecall1_Ignored(t *testing.T) {
	// recall_1 wird ignoriert, OnAction darf nicht aufgerufen werden
	called := false
	r := &zigbee.RemoteDevice{
		FriendlyName: "bilresa_1",
		OnAction: func(a zigbee.RemoteAction) { called = true },
	}
	r.HandleMessage([]byte(`{"action":"recall_1"}`))
	if called {
		t.Error("recall_1 sollte ignoriert werden")
	}
}

func TestRemoteDevice_EmptyAction_Ignored(t *testing.T) {
	called := false
	r := &zigbee.RemoteDevice{
		FriendlyName: "bilresa_1",
		OnAction: func(a zigbee.RemoteAction) { called = true },
	}
	r.HandleMessage([]byte(`{"action":""}`))
	if called {
		t.Error("leere Action sollte ignoriert werden")
	}
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
go test ./internal/zigbee/... -run TestRemote 2>&1
```
Expected: compile error.

- [ ] **Step 3: Implement remote.go**

`internal/zigbee/remote.go`:
```go
package zigbee

import (
	"encoding/json"
	"log/slog"
)

// RemoteAction ist der Typ für BILRESA-Aktionen.
type RemoteAction string

const (
	ActionOn                RemoteAction = "on"
	ActionOff               RemoteAction = "off"
	ActionBrightnessMoveUp  RemoteAction = "brightness_move_up"
	ActionBrightnessMoveDown RemoteAction = "brightness_move_down"
	ActionBrightnessStop    RemoteAction = "brightness_stop"
)

// ignorierteActions werden nicht weitergeleitet.
var ignorierteActions = map[RemoteAction]bool{
	"recall_1": true,
}

// remotePayload ist das eingehende JSON-Format vom BILRESA-Scrollrad.
type remotePayload struct {
	Action string `json:"action"`
}

// RemoteDevice repräsentiert ein BILRESA-Scrollrad.
type RemoteDevice struct {
	FriendlyName string
	DisplayName  string

	// OnAction wird bei jeder relevanten Aktion aufgerufen.
	OnAction func(action RemoteAction)
	log      *slog.Logger
}

// Name implementiert das Device-Interface.
func (r *RemoteDevice) Name() string { return r.FriendlyName }

// HandleMessage verarbeitet eingehende Zigbee2MQTT-Aktionspayloads.
func (r *RemoteDevice) HandleMessage(payload []byte) {
	var p remotePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		if r.log != nil {
			r.log.Warn("Ungültiges JSON von Remote",
				"device_name", r.FriendlyName,
				"fehler", err,
			)
		}
		return
	}

	action := RemoteAction(p.Action)
	if action == "" || ignorierteActions[action] {
		return
	}

	if r.OnAction != nil {
		r.OnAction(action)
	}
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/zigbee/... -v
```
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/zigbee/remote.go internal/zigbee/remote_test.go
git commit -m "feat: RemoteDevice for BILRESA scroll wheel with action filtering"
```

---

## Task 7: Dimmer Automation

**Files:**
- Create: `internal/automation/dimmer.go`, `internal/automation/dimmer_test.go`

- [ ] **Step 1: Write the failing test**

`internal/automation/dimmer_test.go`:
```go
package automation_test

import (
	"sync"
	"testing"
	"time"

	"github.com/ak/zigbee-controller/internal/automation"
	"github.com/ak/zigbee-controller/internal/zigbee"
)

// publishCapture zeichnet Publish-Aufrufe auf.
type publishCapture struct {
	mu   sync.Mutex
	msgs []string
}

func (p *publishCapture) publish(topic string, _ []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.msgs = append(p.msgs, topic)
	return nil
}

func (p *publishCapture) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.msgs)
}

func newBulb(name string, brightness int) *zigbee.BulbDevice {
	b := &zigbee.BulbDevice{FriendlyName: name}
	b.SetState(true, brightness, 370)
	return b
}

func TestDimmer_BrightnessUp(t *testing.T) {
	cap := &publishCapture{}
	bulbs := []*zigbee.BulbDevice{newBulb("b1", 50)}
	d := automation.NewDimmer(bulbs, cap.publish, 20)

	d.Start(zigbee.ActionBrightnessMoveUp)
	time.Sleep(250 * time.Millisecond) // ≥1 Tick
	d.Stop()

	_, br, _ := bulbs[0].GetState()
	if br <= 50 {
		t.Errorf("Helligkeit sollte gestiegen sein: %d", br)
	}
}

func TestDimmer_BrightnessDown(t *testing.T) {
	cap := &publishCapture{}
	bulbs := []*zigbee.BulbDevice{newBulb("b1", 80)}
	d := automation.NewDimmer(bulbs, cap.publish, 20)

	d.Start(zigbee.ActionBrightnessMoveDown)
	time.Sleep(250 * time.Millisecond)
	d.Stop()

	_, br, _ := bulbs[0].GetState()
	if br >= 80 {
		t.Errorf("Helligkeit sollte gesunken sein: %d", br)
	}
}

func TestDimmer_ClampMin(t *testing.T) {
	cap := &publishCapture{}
	bulbs := []*zigbee.BulbDevice{newBulb("b1", 5)} // fast am Minimum
	d := automation.NewDimmer(bulbs, cap.publish, 20)

	d.Start(zigbee.ActionBrightnessMoveDown)
	time.Sleep(500 * time.Millisecond) // mehrere Ticks
	d.Stop()

	_, br, _ := bulbs[0].GetState()
	// Minimum ist minBrightness = 3 Z2M-Einheiten (ca. 1%)
	if br < 3 {
		t.Errorf("Helligkeit darf nicht unter minBrightness (3) fallen: %d", br)
	}
}

func TestDimmer_ClampMax(t *testing.T) {
	cap := &publishCapture{}
	bulbs := []*zigbee.BulbDevice{newBulb("b1", 250)} // fast am Maximum
	d := automation.NewDimmer(bulbs, cap.publish, 20)

	d.Start(zigbee.ActionBrightnessMoveUp)
	time.Sleep(500 * time.Millisecond)
	d.Stop()

	_, br, _ := bulbs[0].GetState()
	if br > 254 {
		t.Errorf("Helligkeit darf nicht über 254 steigen: %d", br)
	}
}

func TestDimmer_StopPublishes(t *testing.T) {
	cap := &publishCapture{}
	bulbs := []*zigbee.BulbDevice{newBulb("b1", 100)}
	d := automation.NewDimmer(bulbs, cap.publish, 20)

	d.Start(zigbee.ActionBrightnessMoveUp)
	time.Sleep(250 * time.Millisecond)
	d.Stop()

	// Nach Stop muss mindestens 1 Publish stattgefunden haben
	if cap.count() == 0 {
		t.Error("Stop sollte finalen Zustand publishen")
	}
}

func TestDimmer_DoubleStop(t *testing.T) {
	cap := &publishCapture{}
	bulbs := []*zigbee.BulbDevice{newBulb("b1", 100)}
	d := automation.NewDimmer(bulbs, cap.publish, 20)

	d.Stop() // kein Panic bei Stop ohne Start
	d.Stop()
	_ = cap
}
```

- [ ] **Step 2: Run test to confirm it fails**

```bash
go test ./internal/automation/... 2>&1
```
Expected: compile error.

- [ ] **Step 3: Implement dimmer.go**

`internal/automation/dimmer.go`:
```go
// Package automation enthält Automatisierungslogik, z.B. das Dimmen über das Scrollrad.
package automation

import (
	"fmt"
	"sync"
	"time"

	"github.com/ak/zigbee-controller/internal/zigbee"
)

const (
	// tickInterval ist das Intervall zwischen zwei Helligkeitsschritten.
	tickInterval = 200 * time.Millisecond
	// minBrightness ist die minimale Helligkeit in Z2M-Einheiten (ca. 1%).
	minBrightness = 3
	// maxBrightness ist die maximale Helligkeit in Z2M-Einheiten.
	maxBrightness = 254
)

// PublishFn ist eine Funktion zum Senden von MQTT-Nachrichten.
type PublishFn func(topic string, payload []byte) error

// Dimmer steuert die Helligkeit mehrerer Lampen über das Scrollrad.
type Dimmer struct {
	bulbs   []*zigbee.BulbDevice
	publish PublishFn
	delta   int // Helligkeitsdelta pro Tick in Z2M-Einheiten

	mu     sync.Mutex
	ticker *time.Ticker
	done   chan struct{}
}

// NewDimmer erstellt einen neuen Dimmer für die gegebenen Lampen.
// delta ist die Helligkeitsänderung pro Tick in Z2M-Einheiten (0–254).
func NewDimmer(bulbs []*zigbee.BulbDevice, publish PublishFn, delta int) *Dimmer {
	return &Dimmer{
		bulbs:   bulbs,
		publish: publish,
		delta:   delta,
	}
}

// Start beginnt das Dimmen in die angegebene Richtung.
// Wird Start erneut aufgerufen, wird ein laufender Ticker gestoppt.
func (d *Dimmer) Start(action zigbee.RemoteAction) {
	d.Stop() // laufenden Ticker stoppen

	dir := 0
	switch action {
	case zigbee.ActionBrightnessMoveUp:
		dir = 1
	case zigbee.ActionBrightnessMoveDown:
		dir = -1
	default:
		return
	}

	d.mu.Lock()
	d.ticker = time.NewTicker(tickInterval)
	d.done = make(chan struct{})
	ticker := d.ticker
	done := d.done
	delta := d.delta * dir
	d.mu.Unlock()

	go func() {
		for {
			select {
			case <-ticker.C:
				d.tick(delta)
			case <-done:
				return
			}
		}
	}()
}

// Stop beendet das Dimmen und publiziert den finalen Zustand.
func (d *Dimmer) Stop() {
	d.mu.Lock()
	ticker := d.ticker
	done := d.done
	d.ticker = nil
	d.done = nil
	d.mu.Unlock()

	if ticker == nil {
		return
	}
	ticker.Stop()
	close(done)

	// Finalen Zustand publishen
	for _, b := range d.bulbs {
		d.publishBulb(b)
	}
}

// tick wendet einen Helligkeitsschritt auf alle Lampen an.
func (d *Dimmer) tick(delta int) {
	for _, b := range d.bulbs {
		on, br, ct := b.GetState()
		if !on {
			continue
		}
		br += delta
		if br < minBrightness {
			br = minBrightness
		}
		if br > maxBrightness {
			br = maxBrightness
		}
		b.SetState(on, br, ct)
		d.publishBulb(b)
	}
}

// publishBulb sendet die aktuelle Helligkeit einer Lampe via MQTT.
// Verwendet BrightnessOnlyCommand, um die KAJPLATS-Einschränkung einzuhalten
// (keine gleichzeitigen Transitions für Helligkeit und Farbtemperatur).
func (d *Dimmer) publishBulb(b *zigbee.BulbDevice) {
	topic := fmt.Sprintf("zigbee2mqtt/%s/set", b.FriendlyName)
	_ = d.publish(topic, b.BrightnessOnlyCommand())
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/automation/... -v
```
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/automation/
git commit -m "feat: Dimmer automation for BILRESA scroll wheel brightness control"
```

---

## Task 8: HomeKit Bridge and Accessories

**Files:**
- Create: `internal/homekit/bridge.go`, `internal/homekit/bulb_accessory.go`, `internal/homekit/remote_accessory.go`

Installs dep: `github.com/brutella/hap`

Note: HomeKit accessory code is integration-heavy (HAP server, OS pairing storage). Unit tests are limited to mapping functions; HAP setup is tested via integration (make run).

- [ ] **Step 1: Install hap dep**

```bash
go get github.com/brutella/hap
go get github.com/brutella/hap/accessory
go mod tidy
```

- [ ] **Step 2: Write mapping test**

`internal/homekit/bulb_accessory_test.go`:
```go
package homekit_test

import (
	"testing"

	"github.com/ak/zigbee-controller/internal/homekit"
)

func TestColorTempMapping_Z2MToHomeKit(t *testing.T) {
	// Werte im gültigen Bereich werden direkt durchgereicht
	hk := homekit.Z2MColorTempToHomeKit(153)
	if hk != 153 {
		t.Errorf("erwartet 153, bekommen %d", hk)
	}
	hk = homekit.Z2MColorTempToHomeKit(370)
	if hk != 370 {
		t.Errorf("erwartet 370, bekommen %d", hk)
	}
	// Unterkante: Werte unter 153 werden auf 140 geclampt
	hk = homekit.Z2MColorTempToHomeKit(100)
	if hk != 140 {
		t.Errorf("erwartet 140 (Untergrenze), bekommen %d", hk)
	}
	// Oberkante: 500 mired → 500 HomeKit
	hk = homekit.Z2MColorTempToHomeKit(500)
	if hk != 500 {
		t.Errorf("erwartet 500, bekommen %d", hk)
	}
}

func TestColorTempMapping_HomeKitToZ2M(t *testing.T) {
	z2m := homekit.HomeKitColorTempToZ2M(500)
	if z2m != 500 {
		t.Errorf("erwartet 500, bekommen %d", z2m)
	}
	// Werte unter 153 werden auf 153 geclampt
	z2m = homekit.HomeKitColorTempToZ2M(140)
	if z2m != 153 {
		t.Errorf("erwartet 153 (Untergrenze), bekommen %d", z2m)
	}
	// Werte im gültigen Bereich werden direkt durchgereicht
	z2m = homekit.HomeKitColorTempToZ2M(370)
	if z2m != 370 {
		t.Errorf("erwartet 370, bekommen %d", z2m)
	}
}
```

- [ ] **Step 3: Run test to confirm it fails**

```bash
go test ./internal/homekit/... 2>&1
```
Expected: compile error.

- [ ] **Step 4: Implement bridge.go**

`internal/homekit/bridge.go`:
```go
// Package homekit stellt die HAP-Bridge und Accessories für Apple HomeKit bereit.
package homekit

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/brutella/hap"
	"github.com/brutella/hap/accessory"
)

// Bridge kapselt den HAP-Server und alle registrierten Accessories.
type Bridge struct {
	server *hap.Server
	log    *slog.Logger
}

// Config enthält HAP-Serverparameter.
type Config struct {
	PIN         string
	Name        string
	StoragePath string
}

// NewBridge erstellt eine HAP-Bridge mit allen übergebenen Accessories.
// Der QR-Code und die Pairing-Informationen werden auf stdout ausgegeben.
func NewBridge(cfg Config, accessories []*accessory.A, log *slog.Logger) (*Bridge, error) {
	bridge := accessory.NewBridge(accessory.Info{
		Name:         cfg.Name,
		Manufacturer: "zigbee-controller",
	})

	store := hap.NewFsStore(cfg.StoragePath)
	server, err := hap.NewServer(store, bridge.A, accessories...)
	if err != nil {
		return nil, fmt.Errorf("HAP-Server erstellen: %w", err)
	}

	server.Pin = cfg.PIN
	server.SetupId = "AB-CD" // statische Setup-ID

	log.Info("HomeKit-Bridge bereit",
		"name", cfg.Name,
		"pin", formatPIN(cfg.PIN),
		"storage", cfg.StoragePath,
	)

	// Pairing-Informationen auf stdout ausgeben
	fmt.Printf("\n=== HomeKit Pairing ===\n")
	fmt.Printf("PIN: %s\n", formatPIN(cfg.PIN))
	fmt.Printf("Pairing-URI: X-HM://%s\n\n", server.Pairing().URI)

	return &Bridge{server: server, log: log}, nil
}

// Start startet den HAP-Server und blockiert bis ctx abgebrochen wird.
func (b *Bridge) Start(ctx context.Context) error {
	b.log.Info("HAP-Server gestartet")
	return b.server.ListenAndServe(ctx)
}

// formatPIN formatiert eine 8-stellige PIN als XXX-XX-XXX.
func formatPIN(pin string) string {
	if len(pin) != 8 {
		return pin
	}
	return fmt.Sprintf("%s-%s-%s", pin[:3], pin[3:5], pin[5:])
}
```

- [ ] **Step 5: Implement bulb_accessory.go**

`internal/homekit/bulb_accessory.go`:
```go
package homekit

import (
	"fmt"
	"log/slog"

	"github.com/brutella/hap/accessory"
	"github.com/ak/zigbee-controller/internal/mqtt"
	"github.com/ak/zigbee-controller/internal/zigbee"
)

// Z2MColorTempToHomeKit wandelt mired (153–500) in HomeKit-Werte (140–500) um.
func Z2MColorTempToHomeKit(mired int) int {
	if mired < 153 {
		return 140
	}
	if mired > 500 {
		return 500
	}
	return mired
}

// HomeKitColorTempToZ2M wandelt HomeKit-Werte (140–500) in mired (153–500) um.
func HomeKitColorTempToZ2M(hk int) int {
	if hk < 153 {
		return 153
	}
	if hk > 500 {
		return 500
	}
	return hk
}

// NewBulbAccessory erstellt ein HomeKit-Lightbulb-Accessory für eine KAJPLATS-Lampe.
// Änderungen in HomeKit werden via MQTT an Zigbee2MQTT weitergeleitet.
func NewBulbAccessory(bulb *zigbee.BulbDevice, mqttClient mqtt.Client, log *slog.Logger) *accessory.A {
	info := accessory.Info{
		Name:         bulb.DisplayName,
		Manufacturer: "IKEA",
		Model:        "LED2411G3",
	}
	a := accessory.NewLightbulb(info)

	setTopic := fmt.Sprintf("zigbee2mqtt/%s/set", bulb.FriendlyName)

	// HomeKit → Zigbee2MQTT: Schaltzustand
	a.Lightbulb.On.OnValueRemoteUpdate(func(on bool) {
		state := "OFF"
		if on {
			state = "ON"
		}
		bulb.SetState(on, 0, 0)
		cmd := []byte(fmt.Sprintf(`{"state":"%s"}`, state))
		if err := mqttClient.Publish(setTopic, cmd); err != nil {
			log.Error("MQTT Publish fehlgeschlagen",
				"device_name", bulb.FriendlyName,
				"topic", setTopic,
				"fehler", err,
			)
		}
	})

	// HomeKit → Zigbee2MQTT: Helligkeit
	a.Lightbulb.Brightness.OnValueRemoteUpdate(func(hkBr int) {
		z2mBr := zigbee.HomeKitBrightnessToZ2M(hkBr)
		_, _, ct := bulb.GetState()
		bulb.SetState(true, z2mBr, ct)
		// WICHTIG: Brightness und ColorTemp niemals gleichzeitig mit transition senden
		cmd := []byte(fmt.Sprintf(`{"brightness":%d,"transition":0.5}`, z2mBr))
		if err := mqttClient.Publish(setTopic, cmd); err != nil {
			log.Error("MQTT Publish fehlgeschlagen",
				"device_name", bulb.FriendlyName,
				"fehler", err,
			)
		}
	})

	// HomeKit → Zigbee2MQTT: Farbtemperatur
	a.Lightbulb.ColorTemperature.OnValueRemoteUpdate(func(hkCT int) {
		z2mCT := HomeKitColorTempToZ2M(hkCT)
		_, br, _ := bulb.GetState()
		bulb.SetState(true, br, z2mCT)
		cmd := []byte(fmt.Sprintf(`{"color_temp":%d,"transition":0.5}`, z2mCT))
		if err := mqttClient.Publish(setTopic, cmd); err != nil {
			log.Error("MQTT Publish fehlgeschlagen",
				"device_name", bulb.FriendlyName,
				"fehler", err,
			)
		}
	})

	// Zigbee2MQTT → HomeKit: Zustand synchronisieren
	bulb.OnStateChange = func(b *zigbee.BulbDevice) {
		on, br, ct := b.GetState()
		a.Lightbulb.On.SetValue(on)
		a.Lightbulb.Brightness.SetValue(zigbee.Z2MBrightnessToHomeKit(br))
		a.Lightbulb.ColorTemperature.SetValue(Z2MColorTempToHomeKit(ct))
	}

	return a.A
}
```

- [ ] **Step 6: Implement remote_accessory.go**

`internal/homekit/remote_accessory.go`:
```go
package homekit

import (
	"github.com/brutella/hap/accessory"
)

// NewRemoteAccessory erstellt ein HomeKit-StatelessProgrammableSwitch-Accessory für das BILRESA-Scrollrad.
// StatelessProgrammableSwitch erscheint in der Home App als Fernbedienung/Button, nicht als Schalter.
// Das Accessory ist rein dekorativ — die Steuerlogik läuft im Dimmer.
func NewRemoteAccessory(friendlyName, displayName string) *accessory.A {
	info := accessory.Info{
		Name:         displayName,
		Manufacturer: "IKEA",
		Model:        "E2489",
	}
	a := accessory.NewStatelessProgrammableSwitch(info)
	_ = friendlyName
	return a.A
}
```

- [ ] **Step 7: Run tests**

```bash
go test ./internal/homekit/... -v
```
Expected: ColorTemp mapping tests PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/homekit/ go.mod go.sum
git commit -m "feat: HomeKit bridge and accessories for bulbs and remote"
```

---

## Task 9: Main Entry Point

**Files:**
- Create: `cmd/bridge/main.go`

- [ ] **Step 1: Create main.go**

`cmd/bridge/main.go`:
```go
// Paket main ist der Einstiegspunkt für die zigbee-controller Bridge.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/brutella/hap/accessory"

	"github.com/ak/zigbee-controller/internal/automation"
	"github.com/ak/zigbee-controller/internal/config"
	"github.com/ak/zigbee-controller/internal/homekit"
	mqttclient "github.com/ak/zigbee-controller/internal/mqtt"
	"github.com/ak/zigbee-controller/internal/zigbee"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Konfiguration laden
	cfgPath := "config.yaml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Error("Konfiguration laden fehlgeschlagen", "fehler", err)
		os.Exit(1)
	}

	// MQTT-Client verbinden
	mqttCfg := mqttclient.Config{
		Broker:   cfg.MQTT.Broker,
		ClientID: cfg.MQTT.ClientID,
		Username: cfg.MQTT.Username,
		Password: cfg.MQTT.Password,
	}
	client, err := mqttclient.NewClient(mqttCfg, log)
	if err != nil {
		log.Error("MQTT-Verbindung fehlgeschlagen", "fehler", err)
		os.Exit(1)
	}
	defer client.Disconnect()

	// Geräte-Registry befüllen
	registry := zigbee.NewRegistry()

	// Lampen anlegen
	bulbsByName := make(map[string]*zigbee.BulbDevice)
	for _, bcfg := range cfg.Devices.Bulbs {
		b := &zigbee.BulbDevice{
			FriendlyName: bcfg.FriendlyName,
			DisplayName:  bcfg.DisplayName,
		}
		registry.Register(b)
		bulbsByName[bcfg.FriendlyName] = b
	}

	// HomeKit-Accessories in Konfigurationsreihenfolge erstellen.
	// WICHTIG: Reihenfolge muss deterministisch sein, damit die HAP-Accessory-IDs
	// bei jedem Neustart identisch sind und HomeKit die Pairings behält.
	var accessories []*accessory.A
	for _, bcfg := range cfg.Devices.Bulbs {
		b := bulbsByName[bcfg.FriendlyName]
		acc := homekit.NewBulbAccessory(b, client, log)
		accessories = append(accessories, acc)
	}

	// MQTT-Topics für Lampen abonnieren (ebenfalls in Konfigurationsreihenfolge)
	for _, bcfg := range cfg.Devices.Bulbs {
		b := bulbsByName[bcfg.FriendlyName]
		stateTopic := "zigbee2mqtt/" + bcfg.FriendlyName
		availTopic := "zigbee2mqtt/" + bcfg.FriendlyName + "/availability"

		if err := client.Subscribe(stateTopic, func(msg mqttclient.Message) {
			b.HandleMessage(msg.Payload)
		}); err != nil {
			log.Error("Subscribe fehlgeschlagen", "topic", stateTopic, "fehler", err)
		}

		if err := client.Subscribe(availTopic, func(msg mqttclient.Message) {
			log.Info("Verfügbarkeit geändert",
				"device_name", b.FriendlyName,
				"status", string(msg.Payload),
			)
		}); err != nil {
			log.Error("Subscribe fehlgeschlagen", "topic", availTopic, "fehler", err)
		}
	}

	// Scrollrad-Remotes anlegen
	for _, rcfg := range cfg.Devices.Remotes {
		// Verknüpfte Lampen sammeln
		var linkedBulbs []*zigbee.BulbDevice
		for _, bname := range rcfg.ControlsBulbs {
			if b, ok := bulbsByName[bname]; ok {
				linkedBulbs = append(linkedBulbs, b)
			} else {
				log.Warn("Verknüpfte Lampe nicht gefunden", "lampe", bname, "remote", rcfg.FriendlyName)
			}
		}

		dimmer := automation.NewDimmer(linkedBulbs, client.Publish, 20)

		remote := &zigbee.RemoteDevice{
			FriendlyName: rcfg.FriendlyName,
			DisplayName:  rcfg.DisplayName,
			OnAction: func(action zigbee.RemoteAction) {
				switch action {
				case zigbee.ActionOn:
					for _, b := range linkedBulbs {
						b.SetState(true, 0, 0)
						_ = client.Publish("zigbee2mqtt/"+b.FriendlyName+"/set", []byte(`{"state":"ON"}`))
					}
				case zigbee.ActionOff:
					for _, b := range linkedBulbs {
						b.SetState(false, 0, 0)
						_ = client.Publish("zigbee2mqtt/"+b.FriendlyName+"/set", []byte(`{"state":"OFF"}`))
					}
				case zigbee.ActionBrightnessMoveUp, zigbee.ActionBrightnessMoveDown:
					dimmer.Start(action)
				case zigbee.ActionBrightnessStop:
					dimmer.Stop()
				}
			},
		}
		registry.Register(remote)

		remoteAcc := homekit.NewRemoteAccessory(rcfg.FriendlyName, rcfg.DisplayName)
		accessories = append(accessories, remoteAcc)

		stateTopic := "zigbee2mqtt/" + rcfg.FriendlyName
		if err := client.Subscribe(stateTopic, func(msg mqttclient.Message) {
			remote.HandleMessage(msg.Payload)
		}); err != nil {
			log.Error("Subscribe fehlgeschlagen", "topic", stateTopic, "fehler", err)
		}
	}

	// HomeKit-Bridge starten
	hkCfg := homekit.Config{
		PIN:         cfg.HomeKit.PIN,
		Name:        cfg.HomeKit.Name,
		StoragePath: cfg.HomeKit.StoragePath,
	}
	bridge, err := homekit.NewBridge(hkCfg, accessories, log)
	if err != nil {
		log.Error("HomeKit-Bridge erstellen fehlgeschlagen", "fehler", err)
		os.Exit(1)
	}

	// Graceful shutdown bei SIGINT/SIGTERM
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	log.Info("zigbee-controller gestartet")
	if err := bridge.Start(ctx); err != nil {
		log.Info("Bridge beendet", "grund", err)
	}
}
```

- [ ] **Step 2: Run go mod tidy**

```bash
go mod tidy
```

- [ ] **Step 3: Build to check for compile errors**

```bash
go build ./cmd/bridge/
```
Expected: binary created with no errors (or only missing `go.sum` entries fixed by tidy).

- [ ] **Step 4: Run all tests**

```bash
go test ./...
```
Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/ go.mod go.sum
git commit -m "feat: main entry point wiring MQTT, Zigbee, HomeKit, and dimmer automation"
```

---

## Task 10: README

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Write README.md**

Replace `README.md` with full setup guide:

```markdown
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
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: vollständige Setup-Anleitung in README"
```

---

## Task 11: Final Verification

- [ ] **Step 1: Run all tests**

```bash
go test ./... -v
```
Expected: all PASS, no failures.

- [ ] **Step 2: Build release binary**

```bash
make build
```
Expected: `bin/bridge` erstellt.

- [ ] **Step 3: Verify gitignore entries**

```bash
git status --short
```
Expected: `config.yaml`, `hap-data/`, `bin/` erscheinen nicht in der Liste (falls vorhanden).

- [ ] **Step 4: Final commit if needed**

```bash
git add -p   # nur nicht-gitignorierte Dateien
git commit -m "chore: final cleanup and verification"
```
