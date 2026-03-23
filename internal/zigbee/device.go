// Package zigbee definiert Zigbee-Gerät-Interfaces und die Device-Registry.
package zigbee

import "sync"

// Device ist das Interface, das alle Zigbee-Geräte implementieren müssen.
type Device interface {
	// Name gibt den friendly_name des Geräts zurück.
	Name() string
	// HandleMessage verarbeitet eingehende Zigbee2MQTT-Nachrichten.
	HandleMessage(payload []byte)
}

// Registry hält alle registrierten Zigbee-Geräte.
type Registry struct {
	mu      sync.RWMutex
	devices map[string]Device
}

// NewRegistry erstellt eine leere Device-Registry.
func NewRegistry() *Registry {
	return &Registry{devices: make(map[string]Device)}
}

// Register fügt ein Gerät zur Registry hinzu.
func (r *Registry) Register(d Device) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.devices[d.Name()] = d
}

// Lookup sucht ein Gerät anhand seines friendly_name.
func (r *Registry) Lookup(name string) (Device, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.devices[name]
	return d, ok
}

// All gibt alle registrierten Geräte zurück.
func (r *Registry) All() []Device {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Device, 0, len(r.devices))
	for _, d := range r.devices {
		out = append(out, d)
	}
	return out
}
