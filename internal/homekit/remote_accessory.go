package homekit

import (
	"github.com/brutella/hap/accessory"
	"github.com/brutella/hap/service"
)

// NewRemoteAccessory erstellt ein HomeKit-StatelessProgrammableSwitch-Accessory für das BILRESA-Scrollrad.
// StatelessProgrammableSwitch erscheint in der Home App als Fernbedienung/Button, nicht als Schalter.
// Das Accessory ist rein dekorativ — die Steuerlogik läuft im Dimmer.
func NewRemoteAccessory(friendlyName, displayName string) *accessory.A {
	info := accessory.Info{
		Name:         displayName,
		Manufacturer: "IKEA",
		Model:        "E2489",
	}
	a := accessory.New(info, accessory.TypeProgrammableSwitch)
	svc := service.NewStatelessProgrammableSwitch()
	a.AddS(svc.S)
	_ = friendlyName // retained in signature for future MQTT topic routing
	return a
}
