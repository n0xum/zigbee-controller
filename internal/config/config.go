// Package config lädt und validiert die Anwendungskonfiguration aus einer YAML-Datei.
package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// MQTTConfig enthält MQTT-Verbindungsparameter.
type MQTTConfig struct {
	Broker   string `mapstructure:"broker"`
	ClientID string `mapstructure:"client_id"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

// HomeKitConfig enthält HAP-Serverparameter.
type HomeKitConfig struct {
	PIN         string `mapstructure:"pin"`
	Name        string `mapstructure:"name"`
	StoragePath string `mapstructure:"storage_path"`
}

// BulbConfig beschreibt eine einzelne Lampe.
type BulbConfig struct {
	FriendlyName string `mapstructure:"friendly_name"`
	DisplayName  string `mapstructure:"display_name"`
}

// RemoteConfig beschreibt ein Scrollrad-Gerät.
type RemoteConfig struct {
	FriendlyName  string   `mapstructure:"friendly_name"`
	DisplayName   string   `mapstructure:"display_name"`
	ControlsBulbs []string `mapstructure:"controls_bulbs"`
}

// DevicesConfig enthält alle konfigurierten Geräte.
type DevicesConfig struct {
	Bulbs   []BulbConfig   `mapstructure:"bulbs"`
	Remotes []RemoteConfig `mapstructure:"remotes"`
}

// Config ist die Hauptkonfigurationsstruktur.
type Config struct {
	MQTT    MQTTConfig    `mapstructure:"mqtt"`
	HomeKit HomeKitConfig `mapstructure:"homekit"`
	Devices DevicesConfig `mapstructure:"devices"`
}

// Load liest die YAML-Konfigurationsdatei unter path und validiert sie.
func Load(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("konfigurationsdatei lesen: %w", err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("konfiguration parsen: %w", err)
	}

	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// validate prüft, ob alle Pflichtfelder gesetzt sind.
func validate(cfg *Config) error {
	if cfg.MQTT.Broker == "" {
		return fmt.Errorf("mqtt.broker darf nicht leer sein")
	}
	if cfg.HomeKit.PIN == "" {
		return fmt.Errorf("homekit.pin darf nicht leer sein")
	}
	if cfg.HomeKit.Name == "" {
		return fmt.Errorf("homekit.name darf nicht leer sein")
	}
	if cfg.HomeKit.StoragePath == "" {
		return fmt.Errorf("homekit.storage_path darf nicht leer sein")
	}
	return nil
}
