// Package homekit stellt die HAP-Bridge und Accessories für Apple HomeKit bereit.
package homekit

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

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

	// Interfaces beschränkt die mDNS-Ankündigung auf die genannten
	// Netzwerkschnittstellen. Leer bedeutet: alle (Standardverhalten von dnssd).
	Interfaces []string
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

	// Ohne Einschränkung kündigt dnssd den Dienst auf jeder sichtbaren
	// Schnittstelle an. Im host-Netzmodus sind das auch alle Docker-Bridges
	// und das Loopback — iOS erhält dann mehrere Adressen für denselben
	// Dienst, von denen die meisten nicht erreichbar sind.
	if len(cfg.Interfaces) > 0 {
		server.Ifaces = cfg.Interfaces
	}

	log.Info("HomeKit-Bridge bereit",
		"name", cfg.Name,
		"pin", formatPIN(cfg.PIN),
		"storage", cfg.StoragePath,
		"interfaces", announcedOn(cfg.Interfaces),
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

// announcedOn beschreibt für die Protokollausgabe, auf welchen Schnittstellen
// angekündigt wird. Leer bedeutet, dass dnssd alle verwendet.
func announcedOn(ifaces []string) string {
	if len(ifaces) == 0 {
		return "alle (nicht eingeschränkt)"
	}
	return strings.Join(ifaces, ",")
}

// formatPIN formatiert eine 8-stellige PIN als XXX-XX-XXX.
func formatPIN(pin string) string {
	if len(pin) != 8 {
		return pin
	}
	return fmt.Sprintf("%s-%s-%s", pin[:3], pin[3:5], pin[5:])
}
