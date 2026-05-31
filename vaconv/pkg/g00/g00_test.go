package g00

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestDecodeFormat0UsesBGR(t *testing.T) {
	data := make([]byte, 17)
	data[0] = 0
	binary.LittleEndian.PutUint16(data[1:], 1)
	binary.LittleEndian.PutUint16(data[3:], 1)
	binary.LittleEndian.PutUint32(data[5:], 12)
	binary.LittleEndian.PutUint32(data[9:], 4)
	copy(data[13:], []byte{0x01, 0x03, 0x02, 0x01})

	img, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode() error: %v", err)
	}

	got := img.RGBA.NRGBAAt(0, 0)
	want := color.NRGBA{R: 0x01, G: 0x02, B: 0x03, A: 0xff}
	if got != want {
		t.Fatalf("pixel = %#v, want %#v", got, want)
	}
}

func TestEncodeFormat0WritesBGR(t *testing.T) {
	rgba := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	rgba.SetNRGBA(0, 0, color.NRGBA{R: 0x01, G: 0x02, B: 0x03, A: 0xff})
	img := &Image{Width: 1, Height: 1, Format: 0, RGBA: rgba}

	data, err := Encode(img)
	if err != nil {
		t.Fatalf("Encode() error: %v", err)
	}

	compSize := int(binary.LittleEndian.Uint32(data[5:9]))
	got := decompressG00_0(data[13:5+compSize], 3)
	want := []byte{0x03, 0x02, 0x01}
	if !bytes.Equal(got, want) {
		t.Fatalf("encoded bytes = % x, want % x", got, want)
	}
}

func TestFromPNGPreservesUnassociatedAlpha(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	want := color.NRGBA{R: 0x78, G: 0x42, B: 0x24, A: 0x05}
	src.SetNRGBA(0, 0, want)

	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatalf("png.Encode() error: %v", err)
	}
	img, err := FromPNG(&buf)
	if err != nil {
		t.Fatalf("FromPNG() error: %v", err)
	}

	if got := img.RGBA.NRGBAAt(0, 0); got != want {
		t.Fatalf("pixel = %#v, want %#v", got, want)
	}
}

func TestEncodeFormat1RoundTrip(t *testing.T) {
	rgba := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	rgba.SetNRGBA(0, 0, color.NRGBA{R: 1, G: 2, B: 3, A: 4})
	rgba.SetNRGBA(1, 0, color.NRGBA{R: 5, G: 6, B: 7, A: 8})
	img := &Image{Width: 2, Height: 1, Format: 1, RGBA: rgba}

	data, err := Encode(img)
	if err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode() error: %v", err)
	}

	if got.Format != 1 {
		t.Fatalf("format = %d, want 1", got.Format)
	}
	for x := 0; x < 2; x++ {
		if got.RGBA.NRGBAAt(x, 0) != rgba.NRGBAAt(x, 0) {
			t.Fatalf("pixel %d = %#v, want %#v", x, got.RGBA.NRGBAAt(x, 0), rgba.NRGBAAt(x, 0))
		}
	}
}

func TestEncodeFormat2RoundTripUsesBGRA(t *testing.T) {
	rgba := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	rgba.SetNRGBA(0, 0, color.NRGBA{R: 0x10, G: 0x20, B: 0x30, A: 0x40})
	rgba.SetNRGBA(1, 0, color.NRGBA{R: 0x50, G: 0x60, B: 0x70, A: 0x80})
	img := &Image{
		Width:  2,
		Height: 1,
		Format: 2,
		RGBA:   rgba,
		Regions: []Region{{
			X1: 0, Y1: 0, X2: 1, Y2: 0, OriginX: 12, OriginY: 34,
		}},
	}

	data, err := Encode(img)
	if err != nil {
		t.Fatalf("Encode() error: %v", err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode() error: %v", err)
	}

	if got.Format != 2 {
		t.Fatalf("format = %d, want 2", got.Format)
	}
	if len(got.Regions) != 1 || got.Regions[0].OriginX != 12 || got.Regions[0].OriginY != 34 {
		t.Fatalf("regions = %#v", got.Regions)
	}
	for x := 0; x < 2; x++ {
		if got.RGBA.NRGBAAt(x, 0) != rgba.NRGBAAt(x, 0) {
			t.Fatalf("pixel %d = %#v, want %#v", x, got.RGBA.NRGBAAt(x, 0), rgba.NRGBAAt(x, 0))
		}
	}
}
