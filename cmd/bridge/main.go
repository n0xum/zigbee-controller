// Paket main ist der Einstiegspunkt für die zigbee-controller Bridge.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/brutella/hap/accessory"

	"github.com/ak/zigbee-controller/internal/automation"
	"github.com/ak/zigbee-controller/internal/config"
	"github.com/ak/zigbee-controller/internal/homekit"
	mqttclient "github.com/ak/zigbee-controller/internal/mqtt"
	"github.com/ak/zigbee-controller/internal/zigbee"
)

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Konfiguration laden
	cfgPath := "config.yaml"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Error("Konfiguration laden fehlgeschlagen", "fehler", err)
		os.Exit(1)
	}

	// MQTT-Client verbinden
	mqttCfg := mqttclient.Config{
		Broker:   cfg.MQTT.Broker,
		ClientID: cfg.MQTT.ClientID,
		Username: cfg.MQTT.Username,
		Password: cfg.MQTT.Password,
	}
	client, err := mqttclient.NewClient(mqttCfg, log)
	if err != nil {
		log.Error("MQTT-Verbindung fehlgeschlagen", "fehler", err)
		os.Exit(1)
	}
	defer client.Disconnect()

	// Geräte-Registry befüllen
	registry := zigbee.NewRegistry()

	// Lampen anlegen
	bulbsByName := make(map[string]*zigbee.BulbDevice)
	for _, bcfg := range cfg.Devices.Bulbs {
		b := &zigbee.BulbDevice{
			FriendlyName: bcfg.FriendlyName,
			DisplayName:  bcfg.DisplayName,
		}
		registry.Register(b)
		bulbsByName[bcfg.FriendlyName] = b
	}

	// HomeKit-Accessories in Konfigurationsreihenfolge erstellen.
	// WICHTIG: Reihenfolge muss deterministisch sein, damit die HAP-Accessory-IDs
	// bei jedem Neustart identisch sind und HomeKit die Pairings behält.
	var accessories []*accessory.A
	for _, bcfg := range cfg.Devices.Bulbs {
		b := bulbsByName[bcfg.FriendlyName]
		acc := homekit.NewBulbAccessory(b, client, log)
		accessories = append(accessories, acc)
	}

	// MQTT-Topics für Lampen abonnieren (ebenfalls in Konfigurationsreihenfolge)
	for _, bcfg := range cfg.Devices.Bulbs {
		b := bulbsByName[bcfg.FriendlyName]
		stateTopic := "zigbee2mqtt/" + bcfg.FriendlyName
		availTopic := "zigbee2mqtt/" + bcfg.FriendlyName + "/availability"

		if err := client.Subscribe(stateTopic, func(msg mqttclient.Message) {
			b.HandleMessage(msg.Payload)
		}); err != nil {
			log.Error("Subscribe fehlgeschlagen", "topic", stateTopic, "fehler", err)
		}

		if err := client.Subscribe(availTopic, func(msg mqttclient.Message) {
			log.Info("Verfügbarkeit geändert",
				"device_name", b.FriendlyName,
				"status", string(msg.Payload),
			)
		}); err != nil {
			log.Error("Subscribe fehlgeschlagen", "topic", availTopic, "fehler", err)
		}
	}

	// Scrollrad-Remotes anlegen
	for _, rcfg := range cfg.Devices.Remotes {
		// Verknüpfte Lampen sammeln
		var linkedBulbs []*zigbee.BulbDevice
		for _, bname := range rcfg.ControlsBulbs {
			if b, ok := bulbsByName[bname]; ok {
				linkedBulbs = append(linkedBulbs, b)
			} else {
				log.Warn("Verknüpfte Lampe nicht gefunden", "lampe", bname, "remote", rcfg.FriendlyName)
			}
		}

		dimmer := automation.NewDimmer(linkedBulbs, client.Publish, 20)

		remote := &zigbee.RemoteDevice{
			FriendlyName: rcfg.FriendlyName,
			DisplayName:  rcfg.DisplayName,
			OnAction: func(action zigbee.RemoteAction) {
				switch action {
				case zigbee.ActionOn:
					for _, b := range linkedBulbs {
						b.SetState(true, 0, 0)
						_ = client.Publish("zigbee2mqtt/"+b.FriendlyName+"/set", []byte(`{"state":"ON"}`))
					}
				case zigbee.ActionOff:
					for _, b := range linkedBulbs {
						b.SetState(false, 0, 0)
						_ = client.Publish("zigbee2mqtt/"+b.FriendlyName+"/set", []byte(`{"state":"OFF"}`))
					}
				case zigbee.ActionBrightnessMoveUp, zigbee.ActionBrightnessMoveDown:
					dimmer.Start(action)
				case zigbee.ActionBrightnessStop:
					dimmer.Stop()
				}
			},
		}
		registry.Register(remote)

		remoteAcc := homekit.NewRemoteAccessory(rcfg.FriendlyName, rcfg.DisplayName)
		accessories = append(accessories, remoteAcc)

		stateTopic := "zigbee2mqtt/" + rcfg.FriendlyName
		if err := client.Subscribe(stateTopic, func(msg mqttclient.Message) {
			remote.HandleMessage(msg.Payload)
		}); err != nil {
			log.Error("Subscribe fehlgeschlagen", "topic", stateTopic, "fehler", err)
		}
	}

	// HomeKit-Bridge starten
	hkCfg := homekit.Config{
		PIN:         cfg.HomeKit.PIN,
		Name:        cfg.HomeKit.Name,
		StoragePath: cfg.HomeKit.StoragePath,
	}
	bridge, err := homekit.NewBridge(hkCfg, accessories, log)
	if err != nil {
		log.Error("HomeKit-Bridge erstellen fehlgeschlagen", "fehler", err)
		os.Exit(1)
	}

	// Graceful shutdown bei SIGINT/SIGTERM
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	log.Info("zigbee-controller gestartet")
	if err := bridge.Start(ctx); err != nil && ctx.Err() == nil {
		// Unexpected error — not caused by context cancellation
		log.Error("Bridge unerwartet beendet", "fehler", err)
		os.Exit(1)
	} else if err != nil {
		log.Info("Bridge beendet", "grund", err)
	}
}
