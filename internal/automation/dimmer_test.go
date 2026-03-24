package automation_test

import (
	"sync"
	"testing"
	"time"

	"github.com/ak/zigbee-controller/internal/automation"
	"github.com/ak/zigbee-controller/internal/zigbee"
)

// publishCapture zeichnet Publish-Aufrufe auf.
type publishCapture struct {
	mu   sync.Mutex
	msgs []string
}

func (p *publishCapture) publish(topic string, _ []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.msgs = append(p.msgs, topic)
	return nil
}

func (p *publishCapture) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.msgs)
}

func newBulb(name string, brightness int) *zigbee.BulbDevice {
	b := &zigbee.BulbDevice{FriendlyName: name}
	b.SetState(true, brightness, 370)
	return b
}

func TestDimmer_BrightnessUp(t *testing.T) {
	cap := &publishCapture{}
	bulbs := []*zigbee.BulbDevice{newBulb("b1", 50)}
	d := automation.NewDimmer(bulbs, cap.publish, 20)

	d.Start(zigbee.ActionBrightnessMoveUp)
	time.Sleep(250 * time.Millisecond)
	d.Stop()

	_, br, _ := bulbs[0].GetState()
	if br <= 50 {
		t.Errorf("Helligkeit sollte gestiegen sein: %d", br)
	}
}

func TestDimmer_BrightnessDown(t *testing.T) {
	cap := &publishCapture{}
	bulbs := []*zigbee.BulbDevice{newBulb("b1", 80)}
	d := automation.NewDimmer(bulbs, cap.publish, 20)

	d.Start(zigbee.ActionBrightnessMoveDown)
	time.Sleep(250 * time.Millisecond)
	d.Stop()

	_, br, _ := bulbs[0].GetState()
	if br >= 80 {
		t.Errorf("Helligkeit sollte gesunken sein: %d", br)
	}
}

func TestDimmer_ClampMin(t *testing.T) {
	cap := &publishCapture{}
	bulbs := []*zigbee.BulbDevice{newBulb("b1", 5)}
	d := automation.NewDimmer(bulbs, cap.publish, 20)

	d.Start(zigbee.ActionBrightnessMoveDown)
	time.Sleep(500 * time.Millisecond)
	d.Stop()

	_, br, _ := bulbs[0].GetState()
	// Minimum ist minBrightness = 3 Z2M-Einheiten (ca. 1%)
	if br < 3 {
		t.Errorf("Helligkeit darf nicht unter minBrightness (3) fallen: %d", br)
	}
}

func TestDimmer_ClampMax(t *testing.T) {
	cap := &publishCapture{}
	bulbs := []*zigbee.BulbDevice{newBulb("b1", 250)}
	d := automation.NewDimmer(bulbs, cap.publish, 20)

	d.Start(zigbee.ActionBrightnessMoveUp)
	time.Sleep(500 * time.Millisecond)
	d.Stop()

	_, br, _ := bulbs[0].GetState()
	if br > 254 {
		t.Errorf("Helligkeit darf nicht über 254 steigen: %d", br)
	}
}

func TestDimmer_StopPublishes(t *testing.T) {
	cap := &publishCapture{}
	bulbs := []*zigbee.BulbDevice{newBulb("b1", 100)}
	d := automation.NewDimmer(bulbs, cap.publish, 20)

	d.Start(zigbee.ActionBrightnessMoveUp)
	time.Sleep(250 * time.Millisecond)
	d.Stop()

	if cap.count() == 0 {
		t.Error("Stop sollte finalen Zustand publishen")
	}
}

func TestDimmer_DoubleStop(t *testing.T) {
	cap := &publishCapture{}
	bulbs := []*zigbee.BulbDevice{newBulb("b1", 100)}
	d := automation.NewDimmer(bulbs, cap.publish, 20)

	d.Stop()
	d.Stop()
	_ = cap
}
