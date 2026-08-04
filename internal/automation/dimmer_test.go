package automation_test

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/ak/zigbee-controller/internal/automation"
	"github.com/ak/zigbee-controller/internal/zigbee"
)

type nachricht struct {
	topic   string
	payload []byte
}

// publishCapture zeichnet Publish-Aufrufe auf.
type publishCapture struct {
	mu   sync.Mutex
	msgs []nachricht
}

func (p *publishCapture) publish(topic string, payload []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.msgs = append(p.msgs, nachricht{topic, append([]byte(nil), payload...)})
	return nil
}

func (p *publishCapture) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.msgs)
}

func (p *publishCapture) alle() []nachricht {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]nachricht(nil), p.msgs...)
}

// feld liest einen Zahlenwert aus dem JSON einer aufgezeichneten Nachricht.
// ok ist false, wenn der Schlüssel fehlt.
func feld(t *testing.T, n nachricht, schluessel string) (int, bool) {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(n.payload, &m); err != nil {
		t.Fatalf("Payload ist kein JSON: %s", n.payload)
	}
	v, ok := m[schluessel]
	if !ok {
		return 0, false
	}
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("%s ist keine Zahl: %v", schluessel, v)
	}
	return int(f), true
}

func newBulb(name string, brightness int) *zigbee.BulbDevice {
	b := &zigbee.BulbDevice{FriendlyName: name}
	b.SetState(true, brightness, 370)
	return b
}

// Der Kern der Änderung: Hochdimmen schickt genau EINEN Befehl, nicht mehr eine
// Salve von Einzelschritten. Früher wären in 300 ms ein bis zwei Ticks gelaufen.
func TestDimmer_StartSendetGenauEinenBefehl(t *testing.T) {
	cap := &publishCapture{}
	d := automation.NewDimmer([]*zigbee.BulbDevice{newBulb("b1", 50)}, cap.publish, 100)

	d.Start(zigbee.ActionBrightnessMoveUp)
	time.Sleep(300 * time.Millisecond)

	if got := cap.count(); got != 1 {
		t.Fatalf("erwartet 1 Befehl, bekommen %d", got)
	}
	n := cap.alle()[0]
	if n.topic != "zigbee2mqtt/b1/set" {
		t.Errorf("Topic: %s", n.topic)
	}
	if rate, ok := feld(t, n, "brightness_move"); !ok || rate != 100 {
		t.Errorf("erwartet brightness_move=100, bekommen %d (vorhanden=%v)", rate, ok)
	}
}

func TestDimmer_RunterHatNegativeRate(t *testing.T) {
	cap := &publishCapture{}
	d := automation.NewDimmer([]*zigbee.BulbDevice{newBulb("b1", 50)}, cap.publish, 100)

	d.Start(zigbee.ActionBrightnessMoveDown)

	if rate, ok := feld(t, cap.alle()[0], "brightness_move"); !ok || rate != -100 {
		t.Errorf("erwartet brightness_move=-100, bekommen %d", rate)
	}
}

// Ohne Haltebefehl würde die Lampe bis zum Anschlag weiterdimmen.
func TestDimmer_StopSendetHaltebefehl(t *testing.T) {
	cap := &publishCapture{}
	d := automation.NewDimmer([]*zigbee.BulbDevice{newBulb("b1", 50)}, cap.publish, 100)

	d.Start(zigbee.ActionBrightnessMoveUp)
	d.Stop()

	msgs := cap.alle()
	if len(msgs) != 2 {
		t.Fatalf("erwartet 2 Befehle (Start, Stopp), bekommen %d", len(msgs))
	}
	if rate, ok := feld(t, msgs[1], "brightness_move"); !ok || rate != 0 {
		t.Errorf("Haltebefehl erwartet brightness_move=0, bekommen %d", rate)
	}
}

func TestDimmer_StopOhneStartIstStill(t *testing.T) {
	cap := &publishCapture{}
	d := automation.NewDimmer([]*zigbee.BulbDevice{newBulb("b1", 50)}, cap.publish, 100)

	d.Stop()
	d.Stop()

	if got := cap.count(); got != 0 {
		t.Errorf("erwartet 0 Befehle, bekommen %d", got)
	}
}

func TestDimmer_AusgeschalteteLampeBleibtUnberuehrt(t *testing.T) {
	cap := &publishCapture{}
	aus := &zigbee.BulbDevice{FriendlyName: "aus"}
	aus.SetState(false, 100, 370)
	d := automation.NewDimmer([]*zigbee.BulbDevice{aus, newBulb("an", 50)}, cap.publish, 100)

	d.Start(zigbee.ActionBrightnessMoveUp)

	msgs := cap.alle()
	if len(msgs) != 1 {
		t.Fatalf("erwartet 1 Befehl (nur die eingeschaltete Lampe), bekommen %d", len(msgs))
	}
	if msgs[0].topic != "zigbee2mqtt/an/set" {
		t.Errorf("falsche Lampe angesprochen: %s", msgs[0].topic)
	}
}

// Landet eine Lampe unter der Mindesthelligkeit, wird sie angehoben. Sonst
// meldet sie state=ON, leuchtet aber nicht und reagiert erst wieder auf einen
// ausdrücklichen Helligkeitswert.
func TestDimmer_HebtZuDunkleLampeAn(t *testing.T) {
	cap := &publishCapture{}
	b := newBulb("b1", 50)
	d := automation.NewDimmer([]*zigbee.BulbDevice{b}, cap.publish, 100)
	automation.SetzeSettleFuerTest(d, 10*time.Millisecond)

	d.Start(zigbee.ActionBrightnessMoveDown)
	b.SetState(true, 1, 0) // die Lampe meldet, sie sei ganz unten angekommen
	d.Stop()
	time.Sleep(200 * time.Millisecond)

	msgs := cap.alle()
	if len(msgs) != 3 {
		t.Fatalf("erwartet 3 Befehle (Start, Stopp, Korrektur), bekommen %d", len(msgs))
	}
	if wert, ok := feld(t, msgs[2], "brightness"); !ok || wert != 3 {
		t.Errorf("Korrektur erwartet brightness=3, bekommen %d (vorhanden=%v)", wert, ok)
	}
	if _, br, _ := b.GetState(); br != 3 {
		t.Errorf("interner Zustand sollte 3 sein, ist %d", br)
	}
}

func TestDimmer_KeineKorrekturWennHellGenug(t *testing.T) {
	cap := &publishCapture{}
	b := newBulb("b1", 50)
	d := automation.NewDimmer([]*zigbee.BulbDevice{b}, cap.publish, 100)
	automation.SetzeSettleFuerTest(d, 10*time.Millisecond)

	d.Start(zigbee.ActionBrightnessMoveDown)
	b.SetState(true, 40, 0)
	d.Stop()
	time.Sleep(200 * time.Millisecond)

	if got := cap.count(); got != 2 {
		t.Errorf("erwartet 2 Befehle ohne Korrektur, bekommen %d", got)
	}
}

// Wird zwischenzeitlich wieder gedimmt, darf die Korrektur des vorherigen
// Vorgangs nicht mehr dazwischenfunken.
func TestDimmer_KorrekturVerfaelltBeiNeuemVorgang(t *testing.T) {
	cap := &publishCapture{}
	b := newBulb("b1", 50)
	d := automation.NewDimmer([]*zigbee.BulbDevice{b}, cap.publish, 100)
	automation.SetzeSettleFuerTest(d, 80*time.Millisecond)

	d.Start(zigbee.ActionBrightnessMoveDown)
	b.SetState(true, 1, 0)
	d.Stop()                               // Korrektur ist eingeplant
	d.Start(zigbee.ActionBrightnessMoveUp) // ... wird aber überholt
	time.Sleep(250 * time.Millisecond)

	for _, n := range cap.alle() {
		if _, ok := feld(t, n, "brightness"); ok {
			t.Fatalf("es hätte keine Korrektur kommen dürfen: %s", n.payload)
		}
	}
}

func TestDimmer_UnbekannteAktionTutNichts(t *testing.T) {
	cap := &publishCapture{}
	d := automation.NewDimmer([]*zigbee.BulbDevice{newBulb("b1", 50)}, cap.publish, 100)

	d.Start(zigbee.ActionBrightnessStop)

	if got := cap.count(); got != 0 {
		t.Errorf("erwartet 0 Befehle, bekommen %d", got)
	}
}
