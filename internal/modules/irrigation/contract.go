package irrigation

import "github.com/abelpinheiro/smarthome-hub/internal/platform/modular"

// ModuleName is the module's stable identifier.
const ModuleName = "irrigation"

// Capabilities required from the devices this module manages.
//
// The module knows nothing about hardware brands or models — only abilities.
// Any device that declares "valve" can irrigate, whether it is a home-built
// ESP32 or a commercial controller. That decoupling is what makes the system
// usable by other people, not just in one house.
const (
	CapValve        modular.Capability = "valve"         // required: open/close
	CapSoilMoisture modular.Capability = "soil_moisture" // required: moisture sensor
	CapFlowMeter    modular.Capability = "flow_meter"    // optional: leak detection
)

// Events published by this module on the bus.
// Names are a public contract: changing them breaks subscribers.
const (
	EventIrrigationStarted = "irrigation.started"
	EventIrrigationStopped = "irrigation.stopped"
	EventMoistureLow       = "irrigation.moisture_low"
	EventWateringSkipped   = "irrigation.watering_skipped"
)
