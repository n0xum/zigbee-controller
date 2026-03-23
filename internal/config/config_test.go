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
