package geometry

import (
	"math"
	"testing"

	"github.com/inov4d/fleet-manager/internal/core"
)

func TestRectCorners(t *testing.T) {
	r := Rect(core.Pose{X: 1, Y: 1, Theta: 0}, 4, 2)
	if len(r) != 4 {
		t.Fatal("expected 4 corners")
	}
	if math.Abs(Area(r)) < 7.99 || math.Abs(Area(r)) > 8.01 {
		t.Fatalf("area %v", Area(r))
	}
	r90 := Rect(core.Pose{Theta: math.Pi / 2}, 4, 2)
	mn, mx := Bounds(r90)
	if math.Abs(mx.Y-mn.Y-4) > 1e-9 || math.Abs(mx.X-mn.X-2) > 1e-9 {
		t.Fatalf("rotated rect bounds wrong: %v %v", mn, mx)
	}
}

func TestIntersects(t *testing.T) {
	a := Rect(core.Pose{X: 0, Y: 0}, 6, 2.4)
	b := Rect(core.Pose{X: 5, Y: 0}, 6, 2.4) // overlap of 1 m
	c := Rect(core.Pose{X: 7, Y: 0}, 6, 2.4) // gap of 1 m
	if !Intersects(a, b) {
		t.Fatal("a and b should overlap")
	}
	if Intersects(a, c) {
		t.Fatal("a and c should not overlap")
	}
	// containment: small polygon fully inside large one
	small := Rect(core.Pose{}, 1, 1)
	if !Intersects(a, small) || !Intersects(small, a) {
		t.Fatal("containment should count as intersection")
	}
	// concave zone (U shape) with a rectangle inside the notch
	u := Polygon{{X: 0, Y: 0}, {X: 10, Y: 0}, {X: 10, Y: 10}, {X: 7, Y: 10}, {X: 7, Y: 3}, {X: 3, Y: 3}, {X: 3, Y: 10}, {X: 0, Y: 10}}
	inNotch := Rect(core.Pose{X: 5, Y: 7}, 2, 2)
	if Intersects(u, inNotch) {
		t.Fatal("rectangle in the notch must not intersect the U")
	}
	onArm := Rect(core.Pose{X: 1.5, Y: 7}, 2, 2)
	if !Intersects(u, onArm) {
		t.Fatal("rectangle on the arm must intersect the U")
	}
}

func TestSweptEnvelopeCoversRotation(t *testing.T) {
	a := core.Pose{X: 0, Y: 0, Theta: 0}
	b := core.Pose{X: 0, Y: 0, Theta: math.Pi / 2}
	env := SweptEnvelope(a, b, 6, 2.4, 8)
	// A corner at 45 degrees must be inside the envelope.
	mid := Rect(core.Pose{Theta: math.Pi / 4}, 6, 2.4)
	for _, c := range mid {
		if !ContainsPoint(env, Point{X: c.X * 0.999, Y: c.Y * 0.999}) {
			t.Fatalf("corner %v not covered by swept envelope", c)
		}
	}
	// Envelope is larger than a single footprint.
	if math.Abs(Area(env)) <= 6*2.4 {
		t.Fatal("swept envelope should be larger than the footprint")
	}
}

func TestSweptEnvelopeTranslation(t *testing.T) {
	env := SweptEnvelope(core.Pose{X: 0, Y: 0}, core.Pose{X: 10, Y: 0}, 6, 2.4, 4)
	mn, mx := Bounds(env)
	if math.Abs((mx.X-mn.X)-16) > 1e-6 || math.Abs((mx.Y-mn.Y)-2.4) > 1e-6 {
		t.Fatalf("unexpected bounds %v %v", mn, mx)
	}
}

func TestEffectiveFootprint(t *testing.T) {
	fp := EffectiveFootprint(6, 2.4, 7, 2.0, 0.3, 0.1)
	if fp.LengthM != 7.8 || math.Abs(fp.WidthM-3.2) > 1e-9 {
		t.Fatalf("got %+v", fp)
	}
}

func TestSegmentPolygon(t *testing.T) {
	z := Polygon{{X: 0, Y: 0}, {X: 4, Y: 0}, {X: 4, Y: 4}, {X: 0, Y: 4}}
	if !SegmentIntersectsPolygon(Point{X: -1, Y: 2}, Point{X: 5, Y: 2}, z) {
		t.Fatal("crossing segment")
	}
	if SegmentIntersectsPolygon(Point{X: -1, Y: 5}, Point{X: 5, Y: 5}, z) {
		t.Fatal("segment above")
	}
	if !SegmentIntersectsPolygon(Point{X: 1, Y: 1}, Point{X: 2, Y: 2}, z) {
		t.Fatal("segment inside")
	}
}

func TestConvexHull(t *testing.T) {
	h := ConvexHull([]Point{{X: 0, Y: 0}, {X: 1, Y: 1}, {X: 2, Y: 0}, {X: 1, Y: 0.5}, {X: 0, Y: 2}, {X: 2, Y: 2}})
	if len(h) != 4 {
		t.Fatalf("hull has %d points: %v", len(h), h)
	}
}

func TestOffset(t *testing.T) {
	sq := Polygon{{X: 0, Y: 0}, {X: 2, Y: 0}, {X: 2, Y: 2}, {X: 0, Y: 2}}
	o := Offset(sq, 0.5)
	mn, mx := Bounds(o)
	if math.Abs(mn.X+0.5) > 1e-9 || math.Abs(mx.X-2.5) > 1e-9 || math.Abs(mn.Y+0.5) > 1e-9 || math.Abs(mx.Y-2.5) > 1e-9 {
		t.Fatalf("offset bounds %v %v", mn, mx)
	}
	// clockwise input must also grow outward
	cw := Polygon{{X: 0, Y: 0}, {X: 0, Y: 2}, {X: 2, Y: 2}, {X: 2, Y: 0}}
	o2 := Offset(cw, 0.5)
	mn2, mx2 := Bounds(o2)
	if math.Abs(mn2.X+0.5) > 1e-9 || math.Abs(mx2.X-2.5) > 1e-9 {
		t.Fatalf("cw offset bounds %v %v", mn2, mx2)
	}
}
