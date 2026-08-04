package homekit

import (
	"io"
	"log/slog"
	"testing"

	"github.com/brutella/hap/accessory"
)

func testConfig(t *testing.T, ifaces []string) Config {
	t.Helper()
	return Config{
		PIN:         "11122333",
		Name:        "Test Bridge",
		StoragePath: t.TempDir(),
		Interfaces:  ifaces,
	}
}

func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// Ist interfaces gesetzt, muss die Einschränkung am HAP-Server ankommen —
// sonst kündigt dnssd den Dienst auf jeder sichtbaren Schnittstelle an,
// im Docker-host-Netzmodus also auch auf allen Docker-Bridges.
func TestNewBridge_SetztIfaces(t *testing.T) {
	b, err := NewBridge(testConfig(t, []string{"eth0"}), []*accessory.A{}, quietLogger())
	if err != nil {
		t.Fatalf("NewBridge: %v", err)
	}

	if len(b.server.Ifaces) != 1 || b.server.Ifaces[0] != "eth0" {
		t.Errorf("erwartet [eth0], bekommen %v", b.server.Ifaces)
	}
}

// Ohne Angabe bleibt das bisherige Verhalten erhalten: dnssd entscheidet selbst.
func TestNewBridge_OhneIfacesUneingeschraenkt(t *testing.T) {
	b, err := NewBridge(testConfig(t, nil), []*accessory.A{}, quietLogger())
	if err != nil {
		t.Fatalf("NewBridge: %v", err)
	}

	if len(b.server.Ifaces) != 0 {
		t.Errorf("erwartet keine Einschränkung, bekommen %v", b.server.Ifaces)
	}
}

func TestAnnouncedOn(t *testing.T) {
	if got := announcedOn(nil); got != "alle (nicht eingeschränkt)" {
		t.Errorf("leer: bekommen %q", got)
	}
	if got := announcedOn([]string{"eth0"}); got != "eth0" {
		t.Errorf("eine: bekommen %q", got)
	}
	if got := announcedOn([]string{"eth0", "wlan0"}); got != "eth0,wlan0" {
		t.Errorf("mehrere: bekommen %q", got)
	}
}
