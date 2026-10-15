// Package geometry provides the planar geometry used by the traffic manager:
// oriented vehicle footprints, swept envelopes of omnidirectional manoeuvres
// and polygon intersection tests. All units are metres / radians.
package geometry

import (
	"math"
	"sort"

	"github.com/inov4d/fleet-manager/internal/core"
)

type Point = core.Point

// Polygon is a simple polygon given as a vertex ring (no repeated end point).
type Polygon []Point

// Rect builds the oriented rectangle (OBB) of a footprint centred on pose.
// Length is along the vehicle x axis (heading), width along y.
func Rect(pose core.Pose, lengthM, widthM float64) Polygon {
	hl, hw := lengthM/2, widthM/2
	c, s := math.Cos(pose.Theta), math.Sin(pose.Theta)
	corners := [4][2]float64{{hl, hw}, {-hl, hw}, {-hl, -hw}, {hl, -hw}}
	out := make(Polygon, 4)
	for i, k := range corners {
		out[i] = Point{X: pose.X + k[0]*c - k[1]*s, Y: pose.Y + k[0]*s + k[1]*c}
	}
	return out
}

// Circle approximates a disc with n vertices.
func Circle(center Point, r float64, n int) Polygon {
	if n < 6 {
		n = 6
	}
	p := make(Polygon, n)
	for i := 0; i < n; i++ {
		a := 2 * math.Pi * float64(i) / float64(n)
		p[i] = Point{X: center.X + r*math.Cos(a), Y: center.Y + r*math.Sin(a)}
	}
	return p
}

// Offset returns the convex polygon grown outward by d (mitered Minkowski
// offset). Works for any orientation; returns p unchanged when d <= 0.
func Offset(p Polygon, d float64) Polygon {
	n := len(p)
	if d <= 0 || n < 3 {
		return p
	}
	sign := 1.0
	if Area(p) < 0 {
		sign = -1.0
	}
	out := make(Polygon, n)
	for i := 0; i < n; i++ {
		prev, cur, next := p[(i+n-1)%n], p[i], p[(i+1)%n]
		// outward normals of the two adjacent edges (for CCW: rotate edge dir by -90deg)
		n1 := edgeNormal(prev, cur, sign)
		n2 := edgeNormal(cur, next, sign)
		bx, by := n1.X+n2.X, n1.Y+n2.Y
		bl := math.Hypot(bx, by)
		if bl < 1e-12 {
			out[i] = Point{X: cur.X + n1.X*d, Y: cur.Y + n1.Y*d}
			continue
		}
		bx, by = bx/bl, by/bl
		cosHalf := bx*n1.X + by*n1.Y
		if cosHalf < 0.2 { // very sharp corner: cap miter
			cosHalf = 0.2
		}
		k := d / cosHalf
		out[i] = Point{X: cur.X + bx*k, Y: cur.Y + by*k}
	}
	return out
}

func edgeNormal(a, b Point, sign float64) Point {
	dx, dy := b.X-a.X, b.Y-a.Y
	l := math.Hypot(dx, dy)
	if l < 1e-12 {
		return Point{}
	}
	return Point{X: sign * dy / l, Y: -sign * dx / l}
}

// Centroid of a polygon (vertex average, adequate for convex shapes).
func Centroid(p Polygon) Point {
	var c Point
	for _, v := range p {
		c.X += v.X
		c.Y += v.Y
	}
	n := float64(len(p))
	if n > 0 {
		c.X /= n
		c.Y /= n
	}
	return c
}

// Area returns the signed area (positive = counter clockwise).
func Area(p Polygon) float64 {
	a := 0.0
	for i := range p {
		j := (i + 1) % len(p)
		a += p[i].X*p[j].Y - p[j].X*p[i].Y
	}
	return a / 2
}

// ContainsPoint tests point inclusion with the even-odd rule (works for concave polygons).
func ContainsPoint(p Polygon, q Point) bool {
	inside := false
	n := len(p)
	for i, j := 0, n-1; i < n; j, i = i, i+1 {
		if (p[i].Y > q.Y) != (p[j].Y > q.Y) {
			x := (p[j].X-p[i].X)*(q.Y-p[i].Y)/(p[j].Y-p[i].Y) + p[i].X
			if q.X < x {
				inside = !inside
			}
		}
	}
	return inside
}

func cross(a, b, c Point) float64 { return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X) }

func onSegment(a, b, p Point) bool {
	return math.Min(a.X, b.X)-1e-9 <= p.X && p.X <= math.Max(a.X, b.X)+1e-9 &&
		math.Min(a.Y, b.Y)-1e-9 <= p.Y && p.Y <= math.Max(a.Y, b.Y)+1e-9
}

// SegmentsIntersect tests proper and touching intersection of segments ab and cd.
func SegmentsIntersect(a, b, c, d Point) bool {
	d1, d2 := cross(c, d, a), cross(c, d, b)
	d3, d4 := cross(a, b, c), cross(a, b, d)
	if ((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) && ((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0)) {
		return true
	}
	if math.Abs(d1) < 1e-12 && onSegment(c, d, a) {
		return true
	}
	if math.Abs(d2) < 1e-12 && onSegment(c, d, b) {
		return true
	}
	if math.Abs(d3) < 1e-12 && onSegment(a, b, c) {
		return true
	}
	if math.Abs(d4) < 1e-12 && onSegment(a, b, d) {
		return true
	}
	return false
}

// SegmentIntersectsPolygon reports whether segment ab touches or crosses polygon p
// (including being fully inside).
func SegmentIntersectsPolygon(a, b Point, p Polygon) bool {
	if len(p) < 3 {
		return false
	}
	if ContainsPoint(p, a) || ContainsPoint(p, b) {
		return true
	}
	for i := range p {
		j := (i + 1) % len(p)
		if SegmentsIntersect(a, b, p[i], p[j]) {
			return true
		}
	}
	return false
}

// Intersects reports whether two simple polygons overlap (works for concave
// polygons; O(n*m)). Touching counts as overlap.
func Intersects(a, b Polygon) bool {
	if len(a) < 3 || len(b) < 3 {
		return false
	}
	// quick AABB rejection
	amin, amax := bounds(a)
	bmin, bmax := bounds(b)
	if amax.X < bmin.X || bmax.X < amin.X || amax.Y < bmin.Y || bmax.Y < amin.Y {
		return false
	}
	for i := range a {
		if ContainsPoint(b, a[i]) {
			return true
		}
	}
	for i := range b {
		if ContainsPoint(a, b[i]) {
			return true
		}
	}
	for i := range a {
		ia := (i + 1) % len(a)
		for j := range b {
			jb := (j + 1) % len(b)
			if SegmentsIntersect(a[i], a[ia], b[j], b[jb]) {
				return true
			}
		}
	}
	return false
}

func bounds(p Polygon) (Point, Point) {
	mn := Point{X: math.Inf(1), Y: math.Inf(1)}
	mx := Point{X: math.Inf(-1), Y: math.Inf(-1)}
	for _, v := range p {
		mn.X = math.Min(mn.X, v.X)
		mn.Y = math.Min(mn.Y, v.Y)
		mx.X = math.Max(mx.X, v.X)
		mx.Y = math.Max(mx.Y, v.Y)
	}
	return mn, mx
}

// Bounds returns the axis-aligned bounding box of a polygon.
func Bounds(p Polygon) (min, max Point) { return bounds(p) }

// ConvexHull computes the convex hull (Andrew's monotone chain).
func ConvexHull(pts []Point) Polygon {
	n := len(pts)
	if n < 3 {
		return Polygon(pts)
	}
	p := make([]Point, n)
	copy(p, pts)
	sort.Slice(p, func(i, j int) bool {
		if p[i].X == p[j].X {
			return p[i].Y < p[j].Y
		}
		return p[i].X < p[j].X
	})
	hull := make([]Point, 0, 2*n)
	for _, pt := range p {
		for len(hull) >= 2 && cross(hull[len(hull)-2], hull[len(hull)-1], pt) <= 0 {
			hull = hull[:len(hull)-1]
		}
		hull = append(hull, pt)
	}
	lower := len(hull) + 1
	for i := n - 2; i >= 0; i-- {
		pt := p[i]
		for len(hull) >= lower && cross(hull[len(hull)-2], hull[len(hull)-1], pt) <= 0 {
			hull = hull[:len(hull)-1]
		}
		hull = append(hull, pt)
	}
	return Polygon(hull[:len(hull)-1])
}

// NormalizeAngle wraps an angle to (-pi, pi].
func NormalizeAngle(a float64) float64 {
	for a > math.Pi {
		a -= 2 * math.Pi
	}
	for a <= -math.Pi {
		a += 2 * math.Pi
	}
	return a
}

// Lerp interpolates a pose; heading takes the shortest rotation.
func Lerp(a, b core.Pose, t float64) core.Pose {
	d := NormalizeAngle(b.Theta - a.Theta)
	return core.Pose{X: a.X + (b.X-a.X)*t, Y: a.Y + (b.Y-a.Y)*t, Theta: a.Theta + d*t}
}

// SweptEnvelope returns the convex hull of the footprint sampled along the
// manoeuvre from pose a to pose b (translation and rotation combined, which is
// what an omnidirectional AGV does), padded by the chord sagitta so the result
// always contains the exact swept region. samples >= 2.
func SweptEnvelope(a, b core.Pose, lengthM, widthM float64, samples int) Polygon {
	if samples < 2 {
		samples = 2
	}
	rot := math.Abs(NormalizeAngle(b.Theta - a.Theta))
	pts := make([]Point, 0, 4*samples)
	for i := 0; i < samples; i++ {
		t := float64(i) / float64(samples-1)
		pts = append(pts, Rect(Lerp(a, b, t), lengthM, widthM)...)
	}
	hull := ConvexHull(pts)
	if rot > 1e-9 {
		step := rot / float64(samples-1)
		sagitta := HalfDiagonal(lengthM, widthM) * (1 - math.Cos(step/2))
		hull = Offset(hull, sagitta)
	}
	return hull
}

// RotationEnvelope is the disc swept by a footprint rotating in place.
func RotationEnvelope(center Point, lengthM, widthM float64) Polygon {
	r := math.Hypot(lengthM/2, widthM/2)
	return Circle(center, r, 16)
}

// HalfDiagonal is the radius of the circumscribed circle of a footprint.
func HalfDiagonal(lengthM, widthM float64) float64 { return math.Hypot(lengthM/2, widthM/2) }

// EffectiveFootprint combines vehicle body, load overhang, safety margin and
// localization uncertainty into one rectangle (length, width).
func EffectiveFootprint(bodyL, bodyW, loadL, loadW, margin, uncertainty float64) core.Footprint {
	l := math.Max(bodyL, loadL)
	w := math.Max(bodyW, loadW)
	pad := 2 * (margin + uncertainty)
	return core.Footprint{LengthM: l + pad, WidthM: w + pad}
}
