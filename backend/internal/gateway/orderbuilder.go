package gateway

import (
	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/mapmgr"
	"github.com/inov4d/fleet-manager/internal/vda5050"
)

// RouteAction is an action to execute at a route index.
type RouteAction struct {
	Index  int
	Action vda5050.Action
}

// BuildOrder converts a planned route into a VDA 5050 order. Nodes with index
// <= releasedThrough form the released base, the rest is the horizon. startIdx
// is the route index of the first node to include (the last node of the
// previous base for order updates, 0 for a new order). Sequence ids derive
// from the absolute route index so updates stitch correctly.
func BuildOrder(g *mapmgr.Graph, mapID, orderID string, updateID int, route []string, startIdx, releasedThrough int, actions []RouteAction, maxSpeed float64, startHeading float64) *vda5050.Order {
	o := &vda5050.Order{OrderID: orderID, OrderUpdateID: updateID, Nodes: []vda5050.Node{}, Edges: []vda5050.Edge{}}
	if startIdx < 0 {
		startIdx = 0
	}
	byIdx := map[int][]vda5050.Action{}
	for _, a := range actions {
		byIdx[a.Index] = append(byIdx[a.Index], a.Action)
	}
	heading := startHeading
	for i := startIdx; i < len(route); i++ {
		n := g.Nodes[route[i]]
		if n == nil {
			break
		}
		released := i <= releasedThrough
		np := &vda5050.NodePosition{X: n.X, Y: n.Y, MapID: mapID, AllowedDeviationXY: f(0.2)}
		if i == len(route)-1 && n.Theta != nil {
			np.Theta = f(*n.Theta)
		}
		acts := byIdx[i]
		if acts == nil {
			acts = []vda5050.Action{}
		}
		o.Nodes = append(o.Nodes, vda5050.Node{NodeID: n.ID, SequenceID: 2 * i, NodeDescription: n.Name, Released: released, NodePosition: np, Actions: acts})
		if i+1 < len(route) {
			e := g.EdgeBetween(route[i], route[i+1])
			if e == nil {
				break
			}
			edgeHeading := g.Heading(route[i], route[i+1])
			travel := mapmgr.TravelHeading(heading, edgeHeading, e.Motion, g.Length(e))
			heading = travel
			speed := e.MaxSpeedMs
			if maxSpeed > 0 && maxSpeed < speed {
				speed = maxSpeed
			}
			for _, zid := range g.ZonesForEdge(e.ID) {
				if z := g.Zones[zid]; z != nil && z.MaxSpeedMs != nil && *z.MaxSpeedMs > 0 && *z.MaxSpeedMs < speed {
					speed = *z.MaxSpeedMs
				}
			}
			length := g.Length(e)
			rot := e.Motion != core.MotionLateral
			o.Edges = append(o.Edges, vda5050.Edge{EdgeID: e.ID, SequenceID: 2*i + 1, Released: released && i+1 <= releasedThrough,
				StartNodeID: route[i], EndNodeID: route[i+1], MaxSpeed: f(speed), Orientation: f(travel), OrientationType: "GLOBAL",
				RotationAllowed: &rot, Length: f(length), Actions: []vda5050.Action{}})
		}
	}
	return o
}

func f(v float64) *float64 { return &v }
