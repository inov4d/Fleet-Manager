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
//
// Local negotiation of right of way between neighbouring agents:
//
//	fleet/<site>/board/conflicts/<vid>  retained  board -> agent   Conflict (who blocks me, since when)
//	fleet/<site>/agents/<vid>/inbox               agent -> agent   Proposal / Resolution (peer to peer, along the wait chain)
//	fleet/<site>/board/yields                     agent -> board   Yield (the agreed concession)
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

// Conflict tells an agent which neighbour holds the resource it waits for.
type Conflict struct {
	VehicleID string           `json:"vehicleId"`
	Resource  core.ResourceRef `json:"resource"`
	HeldBy    []string         `json:"heldBy"`
	Since     time.Time        `json:"since"`
	Active    bool             `json:"active"`
}

// ChainMember is one agent of a wait chain with its cost of yielding.
type ChainMember struct {
	VehicleID string           `json:"vehicleId"`
	YieldCost float64          `json:"yieldCost"`
	WaitsFor  core.ResourceRef `json:"waitsFor"` // resource this member waits for (empty for a free terminal)
}

// Proposal travels along the wait chain: the originator sends it to the
// neighbour blocking it; a blocked neighbour appends itself and forwards it to
// its own blocker, until it reaches a free agent, closes a cycle, or runs out
// of hops. Chain[0] is the originator, Chain[len-1] the last forwarder.
type Proposal struct {
	Nonce string        `json:"nonce"`
	From  string        `json:"from"`
	To    string        `json:"to"`
	Chain []ChainMember `json:"chain"`
	Hops  int           `json:"hops"`
	At    time.Time     `json:"at"`
}

// Resolution is sent by the terminal agent to every member of the chain: the
// cheapest member yields to the member waiting on it. Every agent applies the
// same rule (DecideChain), so the decision needs no referee.
type Resolution struct {
	Nonce       string        `json:"nonce"`
	From        string        `json:"from"`
	Chain       []ChainMember `json:"chain"`
	Yielder     string        `json:"yielder"`
	Beneficiary string        `json:"beneficiary"`
	Cycle       bool          `json:"cycle"`
}

// Yield is the agreed concession, sent to the board which executes it safely.
type Yield struct {
	Nonce       string           `json:"nonce"`
	VehicleID   string           `json:"vehicleId"`   // the one giving way
	Beneficiary string           `json:"beneficiary"` // the one allowed through
	Resource    core.ResourceRef `json:"resource"`
	MyCost      float64          `json:"myCost"`
	PeerCost    float64          `json:"peerCost"`
	Chain       []string         `json:"chain"` // the wait chain that was negotiated
	Reason      string           `json:"reason"`
	At          time.Time        `json:"at"`
}

func TopicConflicts(site, vid string) string { return "fleet/" + site + "/board/conflicts/" + vid }
func TopicInbox(site, vid string) string     { return "fleet/" + site + "/agents/" + vid + "/inbox" }
func TopicYields(site string) string         { return "fleet/" + site + "/board/yields" }

// DecideYield applies the shared rule between two agents: the cheaper side
// yields; ties go to the lexicographically smaller vehicle id. It returns true
// when "me" yields.
func DecideYield(me string, myCost float64, peer string, peerCost float64) bool {
	if myCost != peerCost {
		return myCost < peerCost
	}
	return me < peer
}

// DecideChain picks the yielder of a wait chain (the cheapest member, ties by
// id) and the beneficiary: the member waiting on the yielder, i.e. its
// predecessor in the chain, or the last member when the chain is a cycle
// closed on the originator.
func DecideChain(chain []ChainMember, cycle bool) (yielder, beneficiary string) {
	if len(chain) == 0 {
		return "", ""
	}
	best := 0
	for i := 1; i < len(chain); i++ {
		if chain[i].YieldCost < chain[best].YieldCost || (chain[i].YieldCost == chain[best].YieldCost && chain[i].VehicleID < chain[best].VehicleID) {
			best = i
		}
	}
	yielder = chain[best].VehicleID
	switch {
	case best > 0:
		beneficiary = chain[best-1].VehicleID
	case cycle && len(chain) > 1:
		beneficiary = chain[len(chain)-1].VehicleID
	default:
		beneficiary = ""
	}
	return yielder, beneficiary
}
