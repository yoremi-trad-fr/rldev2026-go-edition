package gan

import (
	"bytes"
	"encoding/xml"
	"testing"
)

func i32(v int32) *int32 { return &v }

func TestReadWriteRoundtrip(t *testing.T) {
	orig := &GAN{
		Bitmap: "test.g00",
		Sets: []Set{
			{Frames: []Frame{
				{Attrs: FrameAttrs{Pattern: i32(0), X: i32(10), Y: i32(20), Time: i32(100)}},
				{Attrs: FrameAttrs{Pattern: i32(1), X: i32(30), Y: i32(40), Time: i32(200)}},
			}},
			{Frames: []Frame{
				{Attrs: FrameAttrs{Pattern: i32(0), Alpha: i32(255)}},
			}},
		},
	}
	// Compute defaults
	for i := range orig.Sets {
		orig.Sets[i].Defaults = computeDefaults(orig.Sets[i].Frames)
	}

	// Write
	var buf bytes.Buffer
	if err := Write(&buf, orig); err != nil {
		t.Fatal(err)
	}
	if buf.Len() == 0 { t.Fatal("empty output") }

	// Read back
	got, err := Read(&buf)
	if err != nil { t.Fatal(err) }

	if got.Bitmap != orig.Bitmap { t.Errorf("bitmap: %q", got.Bitmap) }
	if len(got.Sets) != 2 { t.Fatalf("sets: %d", len(got.Sets)) }
	if len(got.Sets[0].Frames) != 2 { t.Errorf("set0 frames: %d", len(got.Sets[0].Frames)) }
	if len(got.Sets[1].Frames) != 1 { t.Errorf("set1 frames: %d", len(got.Sets[1].Frames)) }

	// Check frame values
	f0 := got.Sets[0].Frames[0].Attrs
	if f0.Pattern == nil || *f0.Pattern != 0 { t.Error("f0 pattern") }
	if f0.X == nil || *f0.X != 10 { t.Error("f0 x") }
	if f0.Time == nil || *f0.Time != 100 { t.Error("f0 time") }

	f1 := got.Sets[0].Frames[1].Attrs
	if f1.Pattern == nil || *f1.Pattern != 1 { t.Error("f1 pattern") }
	if f1.Y == nil || *f1.Y != 40 { t.Error("f1 y") }
}

func TestToXML(t *testing.T) {
	g := &GAN{
		Bitmap: "sprite.g00",
		Sets: []Set{
			{
				Defaults: FrameAttrs{Time: i32(100)},
				Frames: []Frame{
					{Attrs: FrameAttrs{Pattern: i32(0), X: i32(0), Y: i32(0), Time: i32(100)}},
					{Attrs: FrameAttrs{Pattern: i32(1), X: i32(10), Y: i32(0), Time: i32(100)}},
				},
			},
		},
	}

	xmlStr, err := ToXML(g)
	if err != nil { t.Fatal(err) }

	if !bytes.Contains([]byte(xmlStr), []byte("vas_gan")) {
		t.Error("missing vas_gan element")
	}
	if !bytes.Contains([]byte(xmlStr), []byte(`bitmap="sprite.g00"`)) {
		t.Error("missing bitmap attr")
	}
	if !bytes.Contains([]byte(xmlStr), []byte("<set")) {
		t.Error("missing set element")
	}
	if !bytes.Contains([]byte(xmlStr), []byte("<frame")) {
		t.Error("missing frame element")
	}
}

func TestFromXML(t *testing.T) {
	xmlData := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<vas_gan bitmap="test.g00">
  <set time="100">
    <frame pattern="0" x="10" y="20"/>
    <frame pattern="1" x="30" y="40"/>
  </set>
</vas_gan>`)

	g, err := FromXML(xmlData)
	if err != nil { t.Fatal(err) }
	if g.Bitmap != "test.g00" { t.Errorf("bitmap: %q", g.Bitmap) }
	if len(g.Sets) != 1 { t.Fatalf("sets: %d", len(g.Sets)) }
	if len(g.Sets[0].Frames) != 2 { t.Fatalf("frames: %d", len(g.Sets[0].Frames)) }

	f0 := g.Sets[0].Frames[0].Attrs
	if f0.Pattern == nil || *f0.Pattern != 0 { t.Error("f0 pattern") }
	if f0.X == nil || *f0.X != 10 { t.Error("f0 x") }
	if f0.Time == nil || *f0.Time != 100 { t.Error("f0 time (inherited from set)") }
}

func TestXMLRoundtrip(t *testing.T) {
	orig := &GAN{
		Bitmap: "ani.g00",
		Sets: []Set{
			{Frames: []Frame{
				{Attrs: FrameAttrs{Pattern: i32(0), X: i32(0), Time: i32(50), Alpha: i32(255)}},
				{Attrs: FrameAttrs{Pattern: i32(1), X: i32(10), Time: i32(50), Alpha: i32(128)}},
			}},
		},
	}
	for i := range orig.Sets {
		orig.Sets[i].Defaults = computeDefaults(orig.Sets[i].Frames)
	}

	xmlStr, _ := ToXML(orig)
	got, err := FromXML([]byte(xmlStr))
	if err != nil { t.Fatal(err) }

	if got.Bitmap != "ani.g00" { t.Error("bitmap") }
	if len(got.Sets[0].Frames) != 2 { t.Error("frame count") }

	// Values should survive roundtrip
	f0 := got.Sets[0].Frames[0].Attrs
	if f0.Alpha == nil || *f0.Alpha != 255 { t.Error("f0 alpha") }
	f1 := got.Sets[0].Frames[1].Attrs
	if f1.Alpha == nil || *f1.Alpha != 128 { t.Error("f1 alpha") }
}

func TestBinaryXMLBinaryRoundtrip(t *testing.T) {
	orig := &GAN{
		Bitmap: "full.g00",
		Sets: []Set{
			{Frames: []Frame{
				{Attrs: FrameAttrs{Pattern: i32(0), X: i32(5), Y: i32(10), Time: i32(200), Alpha: i32(255), Other: i32(0)}},
			}},
		},
	}

	// Binary → XML
	var binBuf bytes.Buffer
	Write(&binBuf, orig)
	g1, _ := Read(&binBuf)
	xmlStr, _ := ToXML(g1)

	// XML → Binary
	g2, _ := FromXML([]byte(xmlStr))
	var binBuf2 bytes.Buffer
	Write(&binBuf2, g2)

	// Binary → Read again
	g3, _ := Read(&binBuf2)
	if g3.Bitmap != "full.g00" { t.Error("bitmap") }
	f := g3.Sets[0].Frames[0].Attrs
	if f.Other == nil || *f.Other != 0 { t.Error("other field lost in roundtrip") }
}

func TestComputeDefaults(t *testing.T) {
	frames := []Frame{
		{Attrs: FrameAttrs{Pattern: i32(0), Time: i32(100)}},
		{Attrs: FrameAttrs{Pattern: i32(1), Time: i32(100)}},
		{Attrs: FrameAttrs{Pattern: i32(2), Time: i32(100)}},
	}
	d := computeDefaults(frames)
	// Pattern varies → nil; Time constant → 100
	if d.Pattern != nil { t.Error("pattern should vary") }
	if d.Time == nil || *d.Time != 100 { t.Error("time should be constant 100") }
}

func TestXMLMarshal(t *testing.T) {
	xg := XMLGAN{Bitmap: "test.g00", Sets: []XMLSet{
		{Time: "100", Frames: []XMLFrame{{Pattern: "0"}, {Pattern: "1"}}},
	}}
	data, err := xml.MarshalIndent(xg, "", "  ")
	if err != nil { t.Fatal(err) }
	if !bytes.Contains(data, []byte("vas_gan")) { t.Error("root element") }
}

func TestEmptyGAN(t *testing.T) {
	g := &GAN{Bitmap: "empty.g00"}
	var buf bytes.Buffer
	Write(&buf, g)
	got, err := Read(&buf)
	if err != nil { t.Fatal(err) }
	if got.Bitmap != "empty.g00" { t.Error("bitmap") }
	if len(got.Sets) != 0 { t.Error("should have 0 sets") }
}
