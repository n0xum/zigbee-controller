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
	hk := zigbee.Z2MBrightnessToHomeKit(254)
	if hk != 100 {
		t.Errorf("erwartet 100, bekommen %d", hk)
	}
	hk = zigbee.Z2MBrightnessToHomeKit(0)
	if hk != 0 {
		t.Errorf("erwartet 0, bekommen %d", hk)
	}
}

func TestBulbDevice_BrightnessFromHomeKit(t *testing.T) {
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
	b.HandleMessage([]byte(`nicht-json`))
}
