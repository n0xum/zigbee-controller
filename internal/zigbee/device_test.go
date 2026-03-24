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
