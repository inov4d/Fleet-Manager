// Package colony implements the ant-colony task allocation: the Fleet Manager
// publishes a shared task board (stigmergy) and every AGV runs an autonomous
// open-source agent that evaluates the open tasks with its own knowledge and
// claims the most opportune one. The centre only arbitrates simultaneous
// claims, checks eligibility as a safety net and keeps protecting the shared
// space through the traffic manager.
//
// Transport: MQTT (JSON). Topics, with <site> the site id and <vid> the vehicle id:
//
//	fleet/<site>/board/tasks            retained  board -> agents  open tasks with urgency
//	fleet/<site>/board/claims                     agent -> board   Claim
//	fleet/<site>/board/grants/<vid>               board -> agent   Grant
//	fleet/<site>/board/requests                   agent -> board   Request (charge / park)
//	fleet/<site>/board/releases                   agent -> board   Release (give a task back)
//	fleet/<site>/board/assignments/<vid> retained board -> agent   Assignment (what the vehicle is doing)
//	fleet/<site>/map                    retained  board -> agents  active map (agents plan locally)
//	fleet/<site>/vehicles/<vid>         retained  board -> agent   VehicleInfo (model, capacity)
package colony

import (
	"time"

	"github.com/inov4d/fleet-manager/internal/core"
)

// Task is an open task on the board.
type Task struct {
	ID              string     `json:"id"`
	Type            string     `json:"type"`
	Origin          string     `json:"origin"`
	Destination     string     `json:"destination"`
	Nodes           []string   `json:"nodes"` // step nodes in order
	Priority        int        `json:"priority"`
	Urgency         float64    `json:"urgency"` // priority + age pressure: the "pheromone" of the task
	MassKg          float64    `json:"massKg"`
	LoadPresent     bool       `json:"loadPresent"`
	LoadLengthM     float64    `json:"loadLengthM"`
	LoadWidthM      float64    `json:"loadWidthM"`
	RequiredModelID string     `json:"requiredModelId"`
	Deadline        *time.Time `json:"deadline"`
	CreatedAt       time.Time  `json:"createdAt"`
}

// Board is the retained task list.
type Board struct {
	TS     time.Time `json:"ts"`
	SiteID string    `json:"siteId"`
	Tasks  []Task    `json:"tasks"`
}

// Claim is sent by an agent that wants a task.
type Claim struct {
	ClaimID   string    `json:"claimId"`
	TaskID    string    `json:"taskId"`
	VehicleID string    `json:"vehicleId"`
	Cost      float64   `json:"cost"` // the agent's own estimate (seconds)
	At        time.Time `json:"at"`
}

// Grant is the board's answer to a claim.
type Grant struct {
	ClaimID   string `json:"claimId"`
	TaskID    string `json:"taskId"`
	VehicleID string `json:"vehicleId"`
	Granted   bool   `json:"granted"`
	Reason    string `json:"reason"`
	Competing int    `json:"competing"`
}

// Request asks the board for a system task (recharge, parking).
type Request struct {
	VehicleID string `json:"vehicleId"`
	Type      string `json:"type"` // charge | park
	Reason    string `json:"reason"`
}

// Release gives a task back to the board.
type Release struct {
	VehicleID string `json:"vehicleId"`
	TaskID    string `json:"taskId"`
	Reason    string `json:"reason"`
}

// Assignment tells an agent what the fleet thinks it is doing.
type Assignment struct {
	VehicleID string    `json:"vehicleId"`
	TaskID    string    `json:"taskId"`
	Status    string    `json:"status"` // idle | assigned | executing
	TS        time.Time `json:"ts"`
}

// VehicleInfo is the registry entry an agent needs to evaluate tasks.
type VehicleInfo struct {
	ID      string            `json:"id"`
	SiteID  string            `json:"siteId"`
	ModelID string            `json:"modelId"`
	Model   core.VehicleModel `json:"model"`
	Enabled bool              `json:"enabled"`
}

func TopicTasks(site string) string            { return "fleet/" + site + "/board/tasks" }
func TopicClaims(site string) string           { return "fleet/" + site + "/board/claims" }
func TopicGrants(site, vid string) string      { return "fleet/" + site + "/board/grants/" + vid }
func TopicRequests(site string) string         { return "fleet/" + site + "/board/requests" }
func TopicReleases(site string) string         { return "fleet/" + site + "/board/releases" }
func TopicAssignments(site, vid string) string { return "fleet/" + site + "/board/assignments/" + vid }
func TopicMap(site string) string              { return "fleet/" + site + "/map" }
func TopicVehicle(site, vid string) string     { return "fleet/" + site + "/vehicles/" + vid }
