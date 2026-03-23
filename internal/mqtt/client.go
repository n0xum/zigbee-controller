// Package mqtt stellt einen MQTT-Client mit automatischem Reconnect bereit.
package mqtt

import (
	"fmt"
	"log/slog"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
)

// Message kapselt eine eingehende oder ausgehende MQTT-Nachricht.
type Message struct {
	Topic   string
	Payload []byte
}

// MessageHandler wird bei eingehenden Nachrichten aufgerufen.
type MessageHandler func(msg Message)

// Client definiert das Interface für MQTT-Operationen.
type Client interface {
	Subscribe(topic string, handler MessageHandler) error
	Publish(topic string, payload []byte) error
	Disconnect()
}

// pahoClient ist die produktive Implementierung über Eclipse Paho.
type pahoClient struct {
	inner paho.Client
	log   *slog.Logger
}

// Config enthält Verbindungsparameter für NewClient.
type Config struct {
	Broker   string
	ClientID string
	Username string
	Password string
}

// NewClient verbindet sich mit dem MQTT-Broker und gibt einen Client zurück.
// Bei Verbindungsabbruch wird automatisch mit exponentiellem Backoff reconnectet.
func NewClient(cfg Config, log *slog.Logger) (Client, error) {
	opts := paho.NewClientOptions().
		AddBroker(cfg.Broker).
		SetClientID(cfg.ClientID).
		SetAutoReconnect(true).
		SetMaxReconnectInterval(60 * time.Second).
		SetConnectRetryInterval(2 * time.Second).
		SetConnectRetry(true).
		SetOnConnectHandler(func(_ paho.Client) {
			log.Info("MQTT verbunden", "broker", cfg.Broker)
		}).
		SetConnectionLostHandler(func(_ paho.Client, err error) {
			log.Warn("MQTT-Verbindung verloren, reconnect...", "fehler", err)
		})

	if cfg.Username != "" {
		opts.SetUsername(cfg.Username).SetPassword(cfg.Password)
	}

	c := paho.NewClient(opts)

	// Erster Verbindungsversuch mit exponentiellem Backoff (1s, 2s, 4s … max 60s)
	backoff := []time.Duration{1, 2, 4, 8, 16, 30, 60, 60, 60, 60}
	var lastErr error
	for attempt, delay := range backoff {
		if token := c.Connect(); token.Wait() && token.Error() != nil {
			lastErr = token.Error()
			log.Warn("MQTT-Verbindung fehlgeschlagen, warte...",
				"versuch", attempt+1, "delay", delay*time.Second, "fehler", lastErr)
			time.Sleep(delay * time.Second)
			continue
		}
		return &pahoClient{inner: c, log: log}, nil
	}
	return nil, fmt.Errorf("MQTT-Verbindung nach mehreren Versuchen fehlgeschlagen: %w", lastErr)
}

// Subscribe abonniert ein Topic und ruft handler bei eingehenden Nachrichten auf.
func (c *pahoClient) Subscribe(topic string, handler MessageHandler) error {
	token := c.inner.Subscribe(topic, 0, func(_ paho.Client, m paho.Message) {
		handler(Message{Topic: m.Topic(), Payload: m.Payload()})
	})
	token.Wait()
	return token.Error()
}

// Publish sendet eine Nachricht an das angegebene Topic.
func (c *pahoClient) Publish(topic string, payload []byte) error {
	token := c.inner.Publish(topic, 0, false, payload)
	token.Wait()
	return token.Error()
}

// Disconnect trennt die MQTT-Verbindung sauber.
func (c *pahoClient) Disconnect() {
	c.inner.Disconnect(250)
	c.log.Info("MQTT getrennt")
}
