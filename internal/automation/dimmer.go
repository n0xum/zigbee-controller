// Package automation enthält Automatisierungslogik, z.B. das Dimmen über das Scrollrad.
package automation

import (
	"fmt"
	"sync"
	"time"

	"github.com/ak/zigbee-controller/internal/zigbee"
)

const (
	// minBrightness ist die minimale Helligkeit in Z2M-Einheiten (ca. 1%).
	//
	// Unterhalb davon werden manche Lampen zickig: sie melden zwar state=ON,
	// leuchten aber nicht mehr und reagieren erst wieder, wenn man ihnen
	// ausdrücklich eine Helligkeit > 1 schickt. IKEA-Lampen sind dafür bekannt.
	// "brightness_move" hält zwar an der Untergrenze der Lampe an, die kann aber
	// bei 1 liegen -- deshalb die Nachkorrektur in Stop().
	minBrightness = 3

	// standardSettle ist die Wartezeit nach dem Stoppbefehl, bevor die
	// Endhelligkeit geprüft wird. Die Lampe muss ihren Zustand erst über MQTT
	// zurückmelden.
	standardSettle = 700 * time.Millisecond
)

// PublishFn ist eine Funktion zum Senden von MQTT-Nachrichten.
type PublishFn func(topic string, payload []byte) error

// Dimmer steuert die Helligkeit mehrerer Lampen über das Scrollrad.
//
// Statt im Takt Einzelschritte zu senden, wird der Lampe einmal gesagt, dass
// sie dimmen soll, und einmal, dass sie aufhören soll. Dazwischen interpoliert
// sie selbst. Das ist der Weg, für den Zigbee gebaut ist: zwei Befehle pro
// Geste statt fünf pro Sekunde und Lampe.
type Dimmer struct {
	bulbs   []*zigbee.BulbDevice
	publish PublishFn
	rate    int // Helligkeitsänderung in Einheiten pro Sekunde

	// settle ist die Wartezeit vor der Untergrenzen-Korrektur. Feld statt
	// Konstante, damit Tests nicht warten müssen.
	settle time.Duration

	mu    sync.Mutex
	aktiv bool
	// generation signalisiert einer laufenden Korrektur, dass sie verworfen
	// werden soll, weil inzwischen wieder gedimmt wird.
	generation int
}

// NewDimmer erstellt einen neuen Dimmer für die gegebenen Lampen.
// rate ist die Helligkeitsänderung in Einheiten pro Sekunde (Skala 0–254).
func NewDimmer(bulbs []*zigbee.BulbDevice, publish PublishFn, rate int) *Dimmer {
	return &Dimmer{
		bulbs:   bulbs,
		publish: publish,
		rate:    rate,
		settle:  standardSettle,
	}
}

// Start beginnt das Dimmen in die angegebene Richtung.
// Ein bereits laufender Vorgang wird zuvor beendet.
func (d *Dimmer) Start(action zigbee.RemoteAction) {
	dir := 0
	switch action {
	case zigbee.ActionBrightnessMoveUp:
		dir = 1
	case zigbee.ActionBrightnessMoveDown:
		dir = -1
	default:
		return
	}

	d.Stop()

	d.mu.Lock()
	d.aktiv = true
	d.generation++
	d.mu.Unlock()

	cmd := zigbee.BrightnessMoveCommand(d.rate * dir)
	for _, b := range d.bulbs {
		// Ausgeschaltete Lampen bleiben aus -- "brightness_move" würde sie
		// ohnehin nicht wecken, aber so wird gar nicht erst gefunkt.
		if on, _, _ := b.GetState(); !on {
			continue
		}
		d.publishTo(b, cmd)
	}
}

// Stop beendet das Dimmen und korrigiert anschließend Lampen, die zu weit
// heruntergefahren sind.
func (d *Dimmer) Stop() {
	d.mu.Lock()
	war := d.aktiv
	d.aktiv = false
	gen := d.generation
	d.mu.Unlock()

	if !war {
		return
	}

	halt := zigbee.BrightnessMoveCommand(0)
	for _, b := range d.bulbs {
		d.publishTo(b, halt)
	}

	go d.korrigiereUntergrenze(gen)
}

// korrigiereUntergrenze hebt Lampen an, die unter der Mindesthelligkeit
// gelandet sind. Läuft verzögert, weil die Lampe ihren Endstand erst über MQTT
// zurückmelden muss.
//
// Wird zwischenzeitlich wieder gedimmt (neue Generation), tut sie nichts --
// sonst fiele man sich selbst in den laufenden Vorgang.
func (d *Dimmer) korrigiereUntergrenze(gen int) {
	time.Sleep(d.settle)

	d.mu.Lock()
	veraltet := d.generation != gen || d.aktiv
	d.mu.Unlock()
	if veraltet {
		return
	}

	for _, b := range d.bulbs {
		on, br, _ := b.GetState()
		if !on || br >= minBrightness {
			continue
		}
		b.SetState(on, minBrightness, 0)
		d.publishTo(b, zigbee.BrightnessValueCommand(minBrightness))
	}
}

func (d *Dimmer) publishTo(b *zigbee.BulbDevice, payload []byte) {
	_ = d.publish(fmt.Sprintf("zigbee2mqtt/%s/set", b.FriendlyName), payload)
}
