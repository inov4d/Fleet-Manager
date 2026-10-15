// Package maintenance tracks usage counters against maintenance plans and
// reports due / overdue states. A vehicle in maintenance is withdrawn from
// dispatching by the fleet core.
package maintenance

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/inov4d/fleet-manager/internal/core"
	"github.com/inov4d/fleet-manager/internal/store"
)

// Counters are the usage figures of a vehicle.
type Counters struct {
	OdometerM      float64
	OperatingHours float64
	Missions       int
}

// Manager evaluates plans.
type Manager struct {
	st    store.Store
	clock core.Clock
}

func New(st store.Store, clock core.Clock) *Manager { return &Manager{st: st, clock: clock} }

// Plans lists all plans.
func (m *Manager) Plans() []core.MaintenancePlan {
	l, _ := m.st.ListMaintenancePlans(context.Background())
	return l
}

// SavePlan validates and stores a plan.
func (m *Manager) SavePlan(p *core.MaintenancePlan) error {
	if p.Name == "" {
		return core.Validation("plan name is required")
	}
	switch p.Metric {
	case core.MetricOdometer, core.MetricHours, core.MetricMissions, core.MetricCalendar:
	default:
		return core.Validation("invalid metric %q", p.Metric)
	}
	if p.IntervalValue <= 0 {
		return core.Validation("intervalValue must be positive")
	}
	if p.DueSoonFraction <= 0 || p.DueSoonFraction > 1 {
		p.DueSoonFraction = 0.9
	}
	if p.ID == "" {
		p.ID = uuid.NewString()
		p.CreatedAt = m.clock.Now()
	}
	return m.st.SaveMaintenancePlan(context.Background(), p)
}

func (m *Manager) DeletePlan(id string) error {
	return m.st.DeleteMaintenancePlan(context.Background(), id)
}

// Applies reports whether a plan targets a vehicle.
func Applies(p core.MaintenancePlan, v core.Vehicle) bool {
	if p.VehicleID != nil && *p.VehicleID != "" {
		return *p.VehicleID == v.ID
	}
	if p.ModelID != nil && *p.ModelID != "" {
		return *p.ModelID == v.ModelID
	}
	return true
}

// Evaluate computes the status of every applicable plan for the vehicles.
func (m *Manager) Evaluate(vehicles []core.Vehicle, counters func(vehicleID string) Counters) []core.MaintenanceStatus {
	plans := m.Plans()
	records, _ := m.st.ListMaintenanceRecords(context.Background(), "")
	lastDone := map[string]*core.MaintenanceRecord{} // plan|vehicle -> last record
	for i := range records {
		r := records[i]
		k := r.PlanID + "|" + r.VehicleID
		if ex := lastDone[k]; ex == nil || r.DoneAt.After(ex.DoneAt) {
			lastDone[k] = &r
		}
	}
	now := m.clock.Now()
	var out []core.MaintenanceStatus
	for _, p := range plans {
		for _, v := range vehicles {
			if !Applies(p, v) {
				continue
			}
			c := counters(v.ID)
			var current float64
			switch p.Metric {
			case core.MetricOdometer:
				current = c.OdometerM
			case core.MetricHours:
				current = c.OperatingHours
			case core.MetricMissions:
				current = float64(c.Missions)
			case core.MetricCalendar:
				base := p.CreatedAt
				if r := lastDone[p.ID+"|"+v.ID]; r != nil {
					base = r.DoneAt
				}
				current = now.Sub(base).Hours() / 24
			}
			base := p.LastDoneValue
			if r := lastDone[p.ID+"|"+v.ID]; r != nil && p.Metric != core.MetricCalendar {
				base = baseValueFromRecord(r)
			}
			if p.Metric == core.MetricCalendar {
				base = 0
			}
			next := base + p.IntervalValue
			remaining := next - current
			state := "ok"
			if remaining <= 0 {
				state = "overdue"
			} else if current-base >= p.DueSoonFraction*p.IntervalValue {
				state = "dueSoon"
			}
			out = append(out, core.MaintenanceStatus{PlanID: p.ID, PlanName: p.Name, VehicleID: v.ID, Metric: p.Metric, CurrentValue: current, NextDueValue: next, Remaining: remaining, State: state})
		}
	}
	return out
}

// baseValueFromRecord reads the counter value stored in the record notes
// prefix "@value=" written by Record.
func baseValueFromRecord(r *core.MaintenanceRecord) float64 {
	var v float64
	for i := 0; i+7 <= len(r.Notes); i++ {
		if r.Notes[i:i+7] == "@value=" {
			j := i + 7
			for j < len(r.Notes) && (r.Notes[j] == '.' || r.Notes[j] == '-' || (r.Notes[j] >= '0' && r.Notes[j] <= '9')) {
				j++
			}
			var f float64
			if _, err := parseFloat(r.Notes[i+7:j], &f); err == nil {
				v = f
			}
		}
	}
	return v
}

func parseFloat(s string, out *float64) (int, error) {
	var sign, intPart, frac, scale float64 = 1, 0, 0, 1
	i := 0
	if i < len(s) && s[i] == '-' {
		sign = -1
		i++
	}
	for ; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
		intPart = intPart*10 + float64(s[i]-'0')
	}
	if i < len(s) && s[i] == '.' {
		i++
		for ; i < len(s) && s[i] >= '0' && s[i] <= '9'; i++ {
			frac = frac*10 + float64(s[i]-'0')
			scale *= 10
		}
	}
	*out = sign * (intPart + frac/scale)
	return i, nil
}

// Record stores a completed maintenance and resets the plan counter for the
// vehicle by remembering the counter value at completion.
func (m *Manager) Record(planID, vehicleID, doneBy, notes string, counterValue float64) (*core.MaintenanceRecord, error) {
	r := &core.MaintenanceRecord{ID: uuid.NewString(), VehicleID: vehicleID, PlanID: planID, DoneAt: m.clock.Now(), DoneBy: doneBy,
		Notes: notes + " @value=" + formatFloat(counterValue)}
	return r, m.st.SaveMaintenanceRecord(context.Background(), r)
}

func formatFloat(f float64) string {
	neg := f < 0
	if neg {
		f = -f
	}
	ip := int64(f)
	fr := int64((f - float64(ip)) * 1000)
	s := itoa(ip) + "." + pad3(fr)
	if neg {
		return "-" + s
	}
	return s
}

func itoa(i int64) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

func pad3(i int64) string {
	s := itoa(i)
	for len(s) < 3 {
		s = "0" + s
	}
	return s
}

// Records lists records.
func (m *Manager) Records(vehicleID string) []core.MaintenanceRecord {
	l, _ := m.st.ListMaintenanceRecords(context.Background(), vehicleID)
	return l
}

// CounterValue picks the metric value from counters.
func CounterValue(metric core.MaintenanceMetric, c Counters) float64 {
	switch metric {
	case core.MetricOdometer:
		return c.OdometerM
	case core.MetricHours:
		return c.OperatingHours
	case core.MetricMissions:
		return float64(c.Missions)
	}
	return 0
}

var _ = time.Now
