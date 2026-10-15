package fleet

import (
	"context"
	"sort"

	"github.com/google/uuid"
	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/mapmgr"
	"github.com/inov4d/fleet-manager/internal/planner"
)

// Maps lists map summaries.
func (c *Core) Maps(siteID string) []core.Map {
	list, _ := c.st.ListMaps(context.Background(), siteID)
	out := make([]core.Map, 0, len(list))
	for _, m := range list {
		out = append(out, m.Summary())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// GetMap returns a full map.
func (c *Core) GetMap(id string) (*core.Map, error) { return c.st.GetMap(context.Background(), id) }

// ActiveMap returns the active map of a site.
func (c *Core) ActiveMap(siteID string) (*core.Map, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if ge := c.graphs[siteID]; ge != nil {
		cp := *ge.m
		return &cp, nil
	}
	return nil, core.NotFound("active map for site", siteID)
}

// SaveMap creates or replaces a map; the active map is re-installed.
func (c *Core) SaveMap(m core.Map) (*core.Map, error) {
	if err := mapmgr.Validate(&m); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sites[m.SiteID] == nil {
		return nil, core.Validation("unknown site %q", m.SiteID)
	}
	now := c.clock.Now()
	if m.ID == "" {
		m.ID = "map-" + uuid.NewString()[:8]
		m.Version = 1
		m.CreatedAt = now
		m.Active = false
	} else if ex, err := c.st.GetMap(context.Background(), m.ID); err == nil {
		m.Version = ex.Version + 1
		m.CreatedAt = ex.CreatedAt
		m.Active = ex.Active
	} else {
		m.CreatedAt = now
	}
	m.UpdatedAt = now
	if err := c.st.SaveMap(context.Background(), &m); err != nil {
		return nil, err
	}
	if m.Active {
		c.installGraph(&m)
		c.emit(core.Event{Kind: "map.activated", SiteID: m.SiteID, Message: "carte " + m.Name + " v" + itoa(m.Version)})
	}
	return &m, nil
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}

// DeleteMap removes a non-active map.
func (c *Core) DeleteMap(id string) error {
	m, err := c.st.GetMap(context.Background(), id)
	if err != nil {
		return err
	}
	if m.Active {
		return core.Conflict("the active map cannot be deleted")
	}
	return c.st.DeleteMap(context.Background(), id)
}

// ActivateMap makes a map active for its site.
func (c *Core) ActivateMap(id string) (*core.Map, error) {
	m, err := c.st.GetMap(context.Background(), id)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.activateMap(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *Core) activateMap(m *core.Map) error {
	if err := mapmgr.Validate(m); err != nil {
		return err
	}
	ctx := context.Background()
	others, _ := c.st.ListMaps(ctx, m.SiteID)
	for i := range others {
		if others[i].ID != m.ID && others[i].Active {
			others[i].Active = false
			_ = c.st.SaveMap(ctx, &others[i])
		}
	}
	m.Active = true
	if err := c.st.SaveMap(ctx, m); err != nil {
		return err
	}
	c.installGraph(m)
	// executions on this site must replan on the new graph
	for vid, ex := range c.execs {
		if ex.siteID == m.SiteID {
			ex.replanRequested = true
			_ = vid
		}
	}
	c.emit(core.Event{Kind: "map.activated", SiteID: m.SiteID, Message: "carte " + m.Name + " activée"})
	return nil
}

// SetZoneEnabled toggles a zone (temporarily unavailable area).
func (c *Core) SetZoneEnabled(mapID, zoneID string, enabled bool) (*core.Map, error) {
	m, err := c.st.GetMap(context.Background(), mapID)
	if err != nil {
		return nil, err
	}
	found := false
	for i := range m.Zones {
		if m.Zones[i].ID == zoneID {
			m.Zones[i].Enabled = enabled
			found = true
		}
	}
	if !found {
		return nil, core.NotFound("zone", zoneID)
	}
	return c.saveAndReinstall(m)
}

// SetEdgeEnabled toggles an edge.
func (c *Core) SetEdgeEnabled(mapID, edgeID string, enabled bool) (*core.Map, error) {
	m, err := c.st.GetMap(context.Background(), mapID)
	if err != nil {
		return nil, err
	}
	found := false
	for i := range m.Edges {
		if m.Edges[i].ID == edgeID {
			m.Edges[i].Enabled = enabled
			found = true
		}
	}
	if !found {
		return nil, core.NotFound("edge", edgeID)
	}
	return c.saveAndReinstall(m)
}

func (c *Core) saveAndReinstall(m *core.Map) (*core.Map, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	m.UpdatedAt = c.clock.Now()
	if err := c.st.SaveMap(context.Background(), m); err != nil {
		return nil, err
	}
	if m.Active {
		c.installGraph(m)
		for _, ex := range c.execs {
			if ex.siteID == m.SiteID {
				ex.replanRequested = true
			}
		}
		c.emit(core.Event{Kind: "map.activated", SiteID: m.SiteID, Message: "carte " + m.Name + " mise à jour"})
	}
	return m, nil
}

// TestRoute plans a route on a map (editor "test route").
func (c *Core) TestRoute(mapID, from, to, vehicleID string) (*planner.Route, error) {
	m, err := c.st.GetMap(context.Background(), mapID)
	if err != nil {
		return nil, err
	}
	if err := mapmgr.Validate(m); err != nil {
		return nil, err
	}
	g := mapmgr.NewGraph(m)
	req := planner.Request{Graph: g, From: from, To: to}
	if vehicleID != "" {
		c.mu.RLock()
		if r := c.rt[vehicleID]; r != nil {
			req.Envelope = r.state.LoadEnvelope
			req.Heading, req.HasHeading = r.state.Pose.Theta, true
		}
		c.mu.RUnlock()
		req.Congestion = c.traffic.Congestion(m.SiteID)
	}
	return c.planner.Plan(req)
}
