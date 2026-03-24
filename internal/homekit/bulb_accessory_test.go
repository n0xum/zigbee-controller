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
