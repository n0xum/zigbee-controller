package zigbee

import (
	"encoding/json"
	"log/slog"
)

// RemoteAction ist der Typ für BILRESA-Aktionen.
type RemoteAction string

const (
	ActionOn                   RemoteAction = "on"
	ActionOff                  RemoteAction = "off"
	ActionBrightnessMoveUp     RemoteAction = "brightness_move_up"
	ActionBrightnessMoveDown   RemoteAction = "brightness_move_down"
	ActionBrightnessStop       RemoteAction = "brightness_stop"
	ActionBrightnessMoveToLevel RemoteAction = "brightness_move_to_level"
)

// ignorierteActions werden nicht weitergeleitet.
var ignorierteActions = map[RemoteAction]bool{
	"recall_1": true,
}

// remotePayload ist das eingehende JSON-Format vom BILRESA-Scrollrad.
type remotePayload struct {
	Action      string `json:"action"`
	ActionLevel *int   `json:"action_level"`
}

// RemoteDevice repräsentiert ein BILRESA-Scrollrad.
type RemoteDevice struct {
	FriendlyName string
	DisplayName  string

	// OnAction wird bei jeder relevanten Aktion aufgerufen.
	OnAction func(action RemoteAction)
	// OnBrightnessLevel wird bei brightness_move_to_level mit dem absoluten Helligkeitswert aufgerufen.
	OnBrightnessLevel func(level int)
	log               *slog.Logger
}

// Name implementiert das Device-Interface.
func (r *RemoteDevice) Name() string { return r.FriendlyName }

// HandleMessage verarbeitet eingehende Zigbee2MQTT-Aktionspayloads.
func (r *RemoteDevice) HandleMessage(payload []byte) {
	var p remotePayload
	if err := json.Unmarshal(payload, &p); err != nil {
		if r.log != nil {
			r.log.Warn("Ungültiges JSON von Remote",
				"device_name", r.FriendlyName,
				"fehler", err,
			)
		}
		return
	}

	action := RemoteAction(p.Action)
	if action == "" || ignorierteActions[action] {
		return
	}

	if action == ActionBrightnessMoveToLevel {
		if p.ActionLevel != nil && r.OnBrightnessLevel != nil {
			r.OnBrightnessLevel(*p.ActionLevel)
		}
		return
	}

	if r.OnAction != nil {
		r.OnAction(action)
	}
}
