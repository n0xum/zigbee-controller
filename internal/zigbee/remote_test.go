package zigbee_test

import (
	"testing"

	"github.com/ak/zigbee-controller/internal/zigbee"
)

func TestRemoteDevice_Name(t *testing.T) {
	r := &zigbee.RemoteDevice{FriendlyName: "bilresa_1"}
	if r.Name() != "bilresa_1" {
		t.Errorf("Name falsch: %s", r.Name())
	}
}

func TestRemoteDevice_ActionOn(t *testing.T) {
	called := ""
	r := &zigbee.RemoteDevice{
		FriendlyName: "bilresa_1",
		OnAction: func(a zigbee.RemoteAction) { called = string(a) },
	}
	r.HandleMessage([]byte(`{"action":"on"}`))
	if called != "on" {
		t.Errorf("erwartet 'on', bekommen '%s'", called)
	}
}

func TestRemoteDevice_ActionBrightnessMoveUp(t *testing.T) {
	called := zigbee.RemoteAction("")
	r := &zigbee.RemoteDevice{
		FriendlyName: "bilresa_1",
		OnAction: func(a zigbee.RemoteAction) { called = a },
	}
	r.HandleMessage([]byte(`{"action":"brightness_move_up"}`))
	if called != zigbee.ActionBrightnessMoveUp {
		t.Errorf("erwartet brightness_move_up, bekommen '%s'", called)
	}
}

func TestRemoteDevice_ActionRecall1_Ignored(t *testing.T) {
	called := false
	r := &zigbee.RemoteDevice{
		FriendlyName: "bilresa_1",
		OnAction: func(a zigbee.RemoteAction) { called = true },
	}
	r.HandleMessage([]byte(`{"action":"recall_1"}`))
	if called {
		t.Error("recall_1 sollte ignoriert werden")
	}
}

func TestRemoteDevice_EmptyAction_Ignored(t *testing.T) {
	called := false
	r := &zigbee.RemoteDevice{
		FriendlyName: "bilresa_1",
		OnAction: func(a zigbee.RemoteAction) { called = true },
	}
	r.HandleMessage([]byte(`{"action":""}`))
	if called {
		t.Error("leere Action sollte ignoriert werden")
	}
}
