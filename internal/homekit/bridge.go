// Package homekit stellt die HAP-Bridge und Accessories für Apple HomeKit bereit.
package homekit

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/brutella/hap"
	"github.com/brutella/hap/accessory"
)

// Bridge kapselt den HAP-Server und alle registrierten Accessories.
type Bridge struct {
	server *hap.Server
	log    *slog.Logger
}

// Config enthält HAP-Serverparameter.
type Config struct {
	PIN         string
	Name        string
	StoragePath string
}

// NewBridge erstellt eine HAP-Bridge mit allen übergebenen Accessories.
// Der QR-Code und die Pairing-Informationen werden auf stdout ausgegeben.
func NewBridge(cfg Config, accessories []*accessory.A, log *slog.Logger) (*Bridge, error) {
	bridge := accessory.NewBridge(accessory.Info{
		Name:         cfg.Name,
		Manufacturer: "zigbee-controller",
	})

	store := hap.NewFsStore(cfg.StoragePath)
	server, err := hap.NewServer(store, bridge.A, accessories...)
	if err != nil {
		return nil, fmt.Errorf("HAP-Server erstellen: %w", err)
	}

	server.Pin = cfg.PIN
	server.SetupId = "AB-CD" // statische Setup-ID

	log.Info("HomeKit-Bridge bereit",
		"name", cfg.Name,
		"pin", formatPIN(cfg.PIN),
		"storage", cfg.StoragePath,
	)

	// Pairing-Informationen auf stdout ausgeben
	fmt.Printf("\n=== HomeKit Pairing ===\n")
	fmt.Printf("PIN: %s\n", formatPIN(cfg.PIN))
	fmt.Printf("Gerät mit der Home App koppeln und PIN eingeben.\n\n")

	return &Bridge{server: server, log: log}, nil
}

// Start startet den HAP-Server und blockiert bis ctx abgebrochen wird.
func (b *Bridge) Start(ctx context.Context) error {
	b.log.Info("HAP-Server gestartet")
	return b.server.ListenAndServe(ctx)
}

// formatPIN formatiert eine 8-stellige PIN als XXX-XX-XXX.
func formatPIN(pin string) string {
	if len(pin) != 8 {
		return pin
	}
	return fmt.Sprintf("%s-%s-%s", pin[:3], pin[3:5], pin[5:])
}
