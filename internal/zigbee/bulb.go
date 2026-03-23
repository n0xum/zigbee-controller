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
