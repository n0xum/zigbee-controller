package homekit

import (
	"fmt"
	"log/slog"

	"github.com/ak/zigbee-controller/internal/mqtt"
	"github.com/ak/zigbee-controller/internal/zigbee"
	"github.com/brutella/hap/accessory"
	"github.com/brutella/hap/characteristic"
	"github.com/brutella/hap/service"
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

// bulbLightbulb ist ein erweiterter Lightbulb-Service mit Brightness und ColorTemperature.
type bulbLightbulb struct {
	*service.S

	On               *characteristic.On
	Brightness       *characteristic.Brightness
	ColorTemperature *characteristic.ColorTemperature
}

func newBulbLightbulb() *bulbLightbulb {
	s := &bulbLightbulb{}
	s.S = service.New(service.TypeLightbulb)

	s.On = characteristic.NewOn()
	s.AddC(s.On.C)

	s.Brightness = characteristic.NewBrightness()
	s.AddC(s.Brightness.C)

	s.ColorTemperature = characteristic.NewColorTemperature()
	s.AddC(s.ColorTemperature.C)

	return s
}

// bulbAccessory kapselt das HAP-Accessory mit dem erweiterten Lightbulb-Service.
type bulbAccessory struct {
	*accessory.A
	Lightbulb *bulbLightbulb
}

// NewBulbAccessory erstellt ein HomeKit-Lightbulb-Accessory für eine KAJPLATS-Lampe.
// Änderungen in HomeKit werden via MQTT an Zigbee2MQTT weitergeleitet.
func NewBulbAccessory(bulb *zigbee.BulbDevice, mqttClient mqtt.Client, log *slog.Logger) *accessory.A {
	info := accessory.Info{
		Name:         bulb.DisplayName,
		Manufacturer: "IKEA",
		Model:        "LED2411G3",
	}

	a := &bulbAccessory{}
	a.A = accessory.New(info, accessory.TypeLightbulb)
	a.Lightbulb = newBulbLightbulb()
	a.AddS(a.Lightbulb.S)

	setTopic := fmt.Sprintf("zigbee2mqtt/%s/set", bulb.FriendlyName)

	// HomeKit → Zigbee2MQTT: Schaltzustand
	a.Lightbulb.On.OnValueRemoteUpdate(func(on bool) {
		state := "OFF"
		if on {
			state = "ON"
		}
		bulb.SetState(on, 0, 0) // 0,0: preserve existing brightness/colorTemp (SetState skips zero values)
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
