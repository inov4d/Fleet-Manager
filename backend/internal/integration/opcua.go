package integration

import "github.com/inov4d/fleet-manager/internal/core"

// OPCUAAdapter is the contract for exposing the fleet to PLC / SCADA systems
// over OPC UA. The fleet publishes vehicle and mission states as variables and
// accepts mission requests through a method call. An implementation based on
// an OPC UA server library plugs in here without touching the core (roadmap
// phase 3). Keeping the contract explicit lets integrators start mapping
// address spaces now.
type OPCUAAdapter interface {
	// PublishVehicle updates the variables of one vehicle (position, battery,
	// availability, mission).
	PublishVehicle(v core.VehicleWithState) error
	// PublishMission updates the variables of one mission.
	PublishMission(m core.Mission) error
	// OnMissionRequest registers the handler invoked when a PLC calls the
	// "RequestMission" method node.
	OnMissionRequest(func(core.Mission) (*core.Mission, error))
	Close() error
}
