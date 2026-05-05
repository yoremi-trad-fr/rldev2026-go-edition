// Package gan handles GAN animation file format for RealLive.
//
// Transposed from OCaml:
//   - rlxml/gan.ml (219 lines)     — GAN binary read/write
//   - rlxml/convert.ml (89 lines)  — GAN ↔ XML conversion
//
// GAN files (.gan) store animation data for RealLive visual novels.
// Each GAN file references a G00 bitmap and contains a list of
// animation sets, each with a sequence of frames.
//
// Binary format:
//   Header: 3 × int32 (10000, 10000, 10100)
//   G00 filename: int32 length + null-terminated string
//   Data header: int32 (20000) + int32 set_count
//   For each set:
//     int32 (30000) + int32 frame_count
//     For each frame:
//       Repeated: int32 tag + int32 value
//       Terminated by: int32 (999999)
//
// Frame tags:
//   30100 = pattern (sprite index)
//   30101 = x position
//   30102 = y position
//   30103 = time (ms)
//   30104 = alpha (0-255)
//   30105 = other
//
// XML format (vas_gan.dtd):
//   <vas_gan bitmap="filename.g00">
//     <set pattern="0" time="100">
//       <frame x="10" y="20"/>
//       <frame x="30" y="40"/>
//     </set>
//   </vas_gan>
package gan

import (
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// ============================================================
// Data types
// ============================================================

// GAN represents a complete GAN animation file.
type GAN struct {
	Bitmap string // G00 filename reference
	Sets   []Set  // animation sets
}

// Set is one animation set containing multiple frames.
type Set struct {
	Defaults FrameAttrs // attributes common to all frames (for XML output)
	Frames   []Frame
}

// Frame is one animation frame with optional attributes.
type Frame struct {
	Attrs FrameAttrs
}

// FrameAttrs holds the 6 possible frame attributes.
// Each is a pointer — nil means not present.
type FrameAttrs struct {
	Pattern *int32
	X       *int32
	Y       *int32
	Time    *int32
	Alpha   *int32
	Other   *int32
}

// Tag constants
const (
	tagSetStart   = 30000
	tagPattern    = 30100
	tagX          = 30101
	tagY          = 30102
	tagTime       = 30103
	tagAlpha      = 30104
	tagOther      = 30105
	tagFrameEnd   = 999999
	headerA       = 10000
	headerB       = 10000
	headerC       = 10100
	dataSection   = 20000
)

// ============================================================
// Binary reading (from gan.ml of_input)
// ============================================================

// ReadFile reads a GAN file from disk.
func ReadFile(path string) (*GAN, error) {
	f, err := os.Open(path)
	if err != nil { return nil, err }
	defer f.Close()
	return Read(f)
}

// Read parses a GAN binary stream.
func Read(r io.Reader) (*GAN, error) {
	// Read header
	var hdr [3]int32
	if err := binary.Read(r, binary.LittleEndian, &hdr); err != nil {
		return nil, fmt.Errorf("reading GAN header: %w", err)
	}
	if hdr[0] != headerA || hdr[1] != headerB || hdr[2] != headerC {
		return nil, fmt.Errorf("unknown GAN header: %d %d %d", hdr[0], hdr[1], hdr[2])
	}

	// Read G00 filename
	var nameLen int32
	binary.Read(r, binary.LittleEndian, &nameLen)
	nameBuf := make([]byte, nameLen)
	io.ReadFull(r, nameBuf)
	g00 := string(nameBuf[:nameLen-1]) // strip null terminator

	// Read data header
	var dataHdr, setCount int32
	binary.Read(r, binary.LittleEndian, &dataHdr)
	if dataHdr != dataSection {
		return nil, fmt.Errorf("expected GAN data section marker (20000), got %d", dataHdr)
	}
	binary.Read(r, binary.LittleEndian, &setCount)

	// Read sets
	sets := make([]Set, setCount)
	for i := range sets {
		s, err := readSet(r)
		if err != nil { return nil, fmt.Errorf("reading set %d: %w", i, err) }
		sets[i] = s
	}

	return &GAN{Bitmap: g00, Sets: sets}, nil
}

func readSet(r io.Reader) (Set, error) {
	var marker, frameCount int32
	binary.Read(r, binary.LittleEndian, &marker)
	if marker != tagSetStart {
		return Set{}, fmt.Errorf("expected set start (30000), got %d", marker)
	}
	binary.Read(r, binary.LittleEndian, &frameCount)
	if frameCount == 0 {
		return Set{}, fmt.Errorf("animation set must contain at least one frame")
	}

	frames := make([]Frame, frameCount)
	for i := range frames {
		attrs, err := readFrame(r)
		if err != nil { return Set{}, err }
		frames[i] = Frame{Attrs: attrs}
	}

	// Compute default attrs (constant across all frames)
	defaults := computeDefaults(frames)
	return Set{Defaults: defaults, Frames: frames}, nil
}

func readFrame(r io.Reader) (FrameAttrs, error) {
	var attrs FrameAttrs
	for {
		var tag int32
		if err := binary.Read(r, binary.LittleEndian, &tag); err != nil {
			return attrs, err
		}
		if tag == tagFrameEnd { break }

		var val int32
		binary.Read(r, binary.LittleEndian, &val)
		v := val // copy for pointer

		switch tag {
		case tagPattern: attrs.Pattern = &v
		case tagX:       attrs.X = &v
		case tagY:       attrs.Y = &v
		case tagTime:    attrs.Time = &v
		case tagAlpha:   attrs.Alpha = &v
		case tagOther:   attrs.Other = &v
		default:
			return attrs, fmt.Errorf("unknown GAN frame tag %d", tag)
		}
	}
	return attrs, nil
}

// ============================================================
// Binary writing (from gan.ml write_gan)
// ============================================================

// WriteFile writes a GAN file to disk.
func WriteFile(path string, g *GAN) error {
	f, err := os.Create(path)
	if err != nil { return err }
	defer f.Close()
	return Write(f, g)
}

// Write serializes a GAN to binary format.
func Write(w io.Writer, g *GAN) error {
	// Header
	binary.Write(w, binary.LittleEndian, int32(headerA))
	binary.Write(w, binary.LittleEndian, int32(headerB))
	binary.Write(w, binary.LittleEndian, int32(headerC))

	// G00 filename (null-terminated)
	nameBytes := append([]byte(g.Bitmap), 0)
	binary.Write(w, binary.LittleEndian, int32(len(nameBytes)))
	w.Write(nameBytes)

	// Data section
	binary.Write(w, binary.LittleEndian, int32(dataSection))
	binary.Write(w, binary.LittleEndian, int32(len(g.Sets)))

	// Sets
	for _, s := range g.Sets {
		binary.Write(w, binary.LittleEndian, int32(tagSetStart))
		binary.Write(w, binary.LittleEndian, int32(len(s.Frames)))
		for _, f := range s.Frames {
			writeFrame(w, f.Attrs)
		}
	}
	return nil
}

func writeFrame(w io.Writer, a FrameAttrs) {
	writeTag := func(tag int32, val *int32) {
		if val != nil {
			binary.Write(w, binary.LittleEndian, tag)
			binary.Write(w, binary.LittleEndian, *val)
		}
	}
	writeTag(tagPattern, a.Pattern)
	writeTag(tagX, a.X)
	writeTag(tagY, a.Y)
	writeTag(tagTime, a.Time)
	writeTag(tagAlpha, a.Alpha)
	writeTag(tagOther, a.Other)
	binary.Write(w, binary.LittleEndian, int32(tagFrameEnd))
}

// ============================================================
// XML conversion (from convert.ml + gan.ml get_frame*)
// ============================================================

// XML types for marshaling/unmarshaling

// XMLGAN is the top-level XML element.
type XMLGAN struct {
	XMLName xml.Name   `xml:"vas_gan"`
	Bitmap  string     `xml:"bitmap,attr"`
	Sets    []XMLSet   `xml:"set"`
}

// XMLSet is one animation set in XML.
type XMLSet struct {
	Pattern string     `xml:"pattern,attr,omitempty"`
	X       string     `xml:"x,attr,omitempty"`
	Y       string     `xml:"y,attr,omitempty"`
	Time    string     `xml:"time,attr,omitempty"`
	Alpha   string     `xml:"alpha,attr,omitempty"`
	Other   string     `xml:"other,attr,omitempty"`
	Frames  []XMLFrame `xml:"frame"`
}

// XMLFrame is one frame in XML.
type XMLFrame struct {
	Pattern string `xml:"pattern,attr,omitempty"`
	X       string `xml:"x,attr,omitempty"`
	Y       string `xml:"y,attr,omitempty"`
	Time    string `xml:"time,attr,omitempty"`
	Alpha   string `xml:"alpha,attr,omitempty"`
	Other   string `xml:"other,attr,omitempty"`
}

// ToXML converts a GAN to its XML representation string.
func ToXML(g *GAN) (string, error) {
	xg := XMLGAN{Bitmap: g.Bitmap}
	for _, s := range g.Sets {
		xs := XMLSet{}
		// Set default attributes
		setAttrStr(&xs.Pattern, s.Defaults.Pattern)
		setAttrStr(&xs.X, s.Defaults.X)
		setAttrStr(&xs.Y, s.Defaults.Y)
		setAttrStr(&xs.Time, s.Defaults.Time)
		setAttrStr(&xs.Alpha, s.Defaults.Alpha)
		setAttrStr(&xs.Other, s.Defaults.Other)

		for _, f := range s.Frames {
			xf := XMLFrame{}
			// Only emit attributes that differ from set defaults
			setAttrIfDiff(&xf.Pattern, f.Attrs.Pattern, s.Defaults.Pattern)
			setAttrIfDiff(&xf.X, f.Attrs.X, s.Defaults.X)
			setAttrIfDiff(&xf.Y, f.Attrs.Y, s.Defaults.Y)
			setAttrIfDiff(&xf.Time, f.Attrs.Time, s.Defaults.Time)
			setAttrIfDiff(&xf.Alpha, f.Attrs.Alpha, s.Defaults.Alpha)
			setAttrIfDiff(&xf.Other, f.Attrs.Other, s.Defaults.Other)
			xs.Frames = append(xs.Frames, xf)
		}
		xg.Sets = append(xg.Sets, xs)
	}

	out, err := xml.MarshalIndent(xg, "", "  ")
	if err != nil { return "", err }

	header := `<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
		`<!DOCTYPE vas_gan SYSTEM "vas_gan.dtd">` + "\n"
	return header + string(out), nil
}

// FromXML parses an XML string into a GAN.
func FromXML(data []byte) (*GAN, error) {
	var xg XMLGAN
	if err := xml.Unmarshal(data, &xg); err != nil {
		return nil, fmt.Errorf("parsing GAN XML: %w", err)
	}

	g := &GAN{Bitmap: xg.Bitmap}
	for _, xs := range xg.Sets {
		s := Set{}
		for _, xf := range xs.Frames {
			f := Frame{}
			// Merge set defaults with frame overrides
			f.Attrs.Pattern = parseAttr(xf.Pattern, xs.Pattern)
			f.Attrs.X = parseAttr(xf.X, xs.X)
			f.Attrs.Y = parseAttr(xf.Y, xs.Y)
			f.Attrs.Time = parseAttr(xf.Time, xs.Time)
			f.Attrs.Alpha = parseAttr(xf.Alpha, xs.Alpha)
			f.Attrs.Other = parseAttr(xf.Other, xs.Other)
			s.Frames = append(s.Frames, f)
		}
		if len(s.Frames) > 0 {
			s.Defaults = computeDefaults(s.Frames)
		}
		g.Sets = append(g.Sets, s)
	}
	return g, nil
}

// ============================================================
// Helpers
// ============================================================

func computeDefaults(frames []Frame) FrameAttrs {
	if len(frames) == 0 { return FrameAttrs{} }
	d := frames[0].Attrs
	for _, f := range frames[1:] {
		if !int32PtrEq(d.Pattern, f.Attrs.Pattern) { d.Pattern = nil }
		if !int32PtrEq(d.X, f.Attrs.X) { d.X = nil }
		if !int32PtrEq(d.Y, f.Attrs.Y) { d.Y = nil }
		if !int32PtrEq(d.Time, f.Attrs.Time) { d.Time = nil }
		if !int32PtrEq(d.Alpha, f.Attrs.Alpha) { d.Alpha = nil }
		if !int32PtrEq(d.Other, f.Attrs.Other) { d.Other = nil }
	}
	return d
}

func int32PtrEq(a, b *int32) bool {
	if a == nil && b == nil { return true }
	if a == nil || b == nil { return false }
	return *a == *b
}

func setAttrStr(dst *string, v *int32) {
	if v != nil { *dst = strconv.FormatInt(int64(*v), 10) }
}

func setAttrIfDiff(dst *string, frame, def *int32) {
	if frame != nil && !int32PtrEq(frame, def) {
		*dst = strconv.FormatInt(int64(*frame), 10)
	}
}

func parseAttr(frame, set string) *int32 {
	s := frame
	if s == "" { s = set }
	if s == "" { return nil }
	s = strings.TrimSpace(s)
	v, err := strconv.ParseInt(s, 10, 32)
	if err != nil { return nil }
	i := int32(v)
	return &i
}
