package automation

import "time"

// SetzeSettleFuerTest verkürzt die Wartezeit vor der Untergrenzen-Korrektur.
// Nur für Tests -- ohne das müsste jeder Testlauf 700 ms warten.
func SetzeSettleFuerTest(d *Dimmer, dauer time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.settle = dauer
}
