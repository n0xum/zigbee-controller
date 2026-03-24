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
// Thread-safe: d.ticker und d.done werden unter dem Mutex auf nil gesetzt, bevor der Lock
// freigegeben wird. Gleichzeitige Aufrufe von Stop() können den nil-Check nie beide bestehen,
// da nur einer der beiden einen nicht-nil ticker herausbekommt.
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
