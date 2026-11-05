package overlay

import "testing"

func TestEdgeRectsStayInsideAnOffsetDisplay(t *testing.T) {
	// A monitor to the left of the primary one has negative coordinates.
	bounds := rect{-1920, 0, 0, 1080}
	const thickness = 96
	rects := map[Edge]rect{}
	for _, edge := range []Edge{EdgeTop, EdgeBottom, EdgeLeft, EdgeRight} {
		r := edgeRect(bounds, edge, thickness)
		if r.Left < bounds.Left || r.Right > bounds.Right || r.Top < bounds.Top || r.Bottom > bounds.Bottom {
			t.Errorf("%v escapes the display: %v", edge, r)
		}
		rects[edge] = r
	}
	if rects[EdgeTop].Bottom != rects[EdgeLeft].Top || rects[EdgeBottom].Top != rects[EdgeLeft].Bottom {
		t.Fatalf("corners overlap or leave a gap: %v", rects)
	}
	if rects[EdgeLeft].Right > rects[EdgeRight].Left {
		t.Fatalf("vertical bands overlap: %v", rects)
	}
}

func TestMonitorsAreDistinctAndPositive(t *testing.T) {
	found := monitors()
	if len(found) == 0 {
		t.Skip("no display is attached to this session")
	}
	seen := map[rect]bool{}
	for _, m := range found {
		if m.bounds.width() <= 0 || m.bounds.height() <= 0 {
			t.Errorf("monitor with impossible bounds: %+v", m)
		}
		if seen[m.bounds] {
			t.Errorf("duplicate monitor: %+v", m)
		}
		seen[m.bounds] = true
		if m.dpi < 72 || m.dpi > 480 {
			t.Errorf("monitor DPI %d looks wrong: %+v", m.dpi, m)
		}
	}
}

func TestSignatureTracksGeometryAndScale(t *testing.T) {
	a := monitor{bounds: rect{0, 0, 1920, 1080}, dpi: 96}
	b := monitor{bounds: rect{0, 0, 1920, 1080}, dpi: 144}
	c := monitor{bounds: rect{0, 0, 2560, 1600}, dpi: 96}
	if signaturePart(a) == signaturePart(b) {
		t.Fatal("a scale change does not change the signature")
	}
	if signaturePart(a) == signaturePart(c) {
		t.Fatal("a resolution change does not change the signature")
	}
	if signaturePart(a) != signaturePart(a) {
		t.Fatal("the same monitor signs differently")
	}
}
