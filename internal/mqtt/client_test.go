// Package mqtt_test testet das MQTT-Client-Interface.
package mqtt_test

import (
	"sync"
	"testing"
	"time"

	mqttclient "github.com/ak/zigbee-controller/internal/mqtt"
)

// mockClient implementiert mqttclient.Client für Tests.
type mockClient struct {
	mu        sync.Mutex
	published []mqttclient.Message
	handlers  map[string]mqttclient.MessageHandler
}

func (m *mockClient) Subscribe(topic string, handler mqttclient.MessageHandler) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.handlers == nil {
		m.handlers = make(map[string]mqttclient.MessageHandler)
	}
	m.handlers[topic] = handler
	return nil
}

func (m *mockClient) Publish(topic string, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.published = append(m.published, mqttclient.Message{Topic: topic, Payload: payload})
	return nil
}

func (m *mockClient) Disconnect() {}

// simuliert eingehende Nachricht
func (m *mockClient) receive(topic string, payload []byte) {
	m.mu.Lock()
	h := m.handlers[topic]
	m.mu.Unlock()
	if h != nil {
		h(mqttclient.Message{Topic: topic, Payload: payload})
	}
}

func TestClient_Interface(t *testing.T) {
	// Stellt sicher, dass mockClient das Interface erfüllt
	var _ mqttclient.Client = &mockClient{}
}

func TestClient_PublishCollected(t *testing.T) {
	c := &mockClient{}
	_ = c.Publish("test/topic", []byte(`{"state":"ON"}`))

	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.published) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(c.published))
	}
	if c.published[0].Topic != "test/topic" {
		t.Errorf("topic mismatch: %s", c.published[0].Topic)
	}
}

func TestClient_SubscribeAndReceive(t *testing.T) {
	c := &mockClient{}
	received := make(chan mqttclient.Message, 1)

	_ = c.Subscribe("test/topic", func(msg mqttclient.Message) {
		received <- msg
	})

	c.receive("test/topic", []byte(`{"state":"OFF"}`))

	select {
	case msg := <-received:
		if string(msg.Payload) != `{"state":"OFF"}` {
			t.Errorf("payload mismatch: %s", msg.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}
