// Package g00 handles the G00 image format used by RealLive visual novels.
//
// Ported from vaconv (OCaml + C++):
//   - g00.ml (657 lines)      — format read/write logic
//   - g00-bt.cpp (268 lines)  — LZSS compression/decompression
//
// Three G00 formats exist:
//   Format 0: RGB bitmap (24-bit, LZSS compressed with 3-byte stride)
//   Format 1: Paletted bitmap (8-bit + RGBA palette, LZSS compressed)
//   Format 2: Composite RGBA with regions/sub-images
package g00

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
)

// Image represents a decoded G00 image.
type Image struct {
	Width, Height int
	Format        int           // 0, 1, or 2
	RGBA          *image.NRGBA  // decoded pixel data
	Regions       []Region      // format 2 regions
}

// Region describes a sub-image region in format 2.
type Region struct {
	X1, Y1, X2, Y2 int
	OriginX, OriginY int
}

// ============================================================
// Reading
// ============================================================

// ReadFile reads a G00 file and returns the decoded image.
func ReadFile(path string) (*Image, error) {
	data, err := os.ReadFile(path)
	if err != nil { return nil, err }
	return Decode(data)
}

// Decode decodes G00 data from a byte slice.
func Decode(data []byte) (*Image, error) {
	if len(data) < 9 { return nil, fmt.Errorf("g00: too short") }
	format := int(data[0])
	w := int(binary.LittleEndian.Uint16(data[1:3]))
	h := int(binary.LittleEndian.Uint16(data[3:5]))
	
	switch format {
	case 0: return decodeFormat0(data, w, h)
	case 1: return decodeFormat1(data, w, h)
	case 2: return decodeFormat2(data, w, h)
	default: return nil, fmt.Errorf("g00: unknown format %d", format)
	}
}

// --- Format 0: RGB bitmap ---

func decodeFormat0(data []byte, w, h int) (*Image, error) {
	compSize := int(binary.LittleEndian.Uint32(data[5:9]))
	uncompSize := int(binary.LittleEndian.Uint32(data[9:13]))
	if uncompSize != w*h*3 { // RGB, 3 bytes per pixel
		// Some files report w*h*4
	}
	if compSize+5 > len(data) {
		return nil, fmt.Errorf("g00/0: compressed data exceeds file size")
	}
	compressed := data[13 : 5+compSize]
	rgb := decompressG00_0(compressed, w*h*3)
	
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			off := (y*w + x) * 3
			if off+2 < len(rgb) {
				img.SetNRGBA(x, y, color.NRGBA{R: rgb[off], G: rgb[off+1], B: rgb[off+2], A: 255})
			}
		}
	}
	return &Image{Width: w, Height: h, Format: 0, RGBA: img}, nil
}

// --- Format 1: Paletted bitmap ---

func decodeFormat1(data []byte, w, h int) (*Image, error) {
	compSize := int(binary.LittleEndian.Uint32(data[5:9]))
	// uncompSize at data[9:13]
	if 5+compSize > len(data) {
		return nil, fmt.Errorf("g00/1: data truncated")
	}
	compressed := data[13 : 5+compSize]
	decompressed := decompressG00_1(compressed, 2+256*4+w*h) // max possible

	// Parse palette
	palLen := int(binary.LittleEndian.Uint16(decompressed[0:2]))
	if palLen > 256 { palLen = 256 }
	palette := make([]color.NRGBA, palLen)
	for i := 0; i < palLen; i++ {
		off := 2 + i*4
		palette[i] = color.NRGBA{R: decompressed[off], G: decompressed[off+1], B: decompressed[off+2], A: decompressed[off+3]}
	}

	// Parse pixel indices
	pixelOff := 2 + palLen*4
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			idx := int(decompressed[pixelOff+y*w+x])
			if idx < len(palette) {
				img.SetNRGBA(x, y, palette[idx])
			}
		}
	}
	return &Image{Width: w, Height: h, Format: 1, RGBA: img}, nil
}

// --- Format 2: Composite RGBA ---

func decodeFormat2(data []byte, w, h int) (*Image, error) {
	regionCount := int(binary.LittleEndian.Uint32(data[5:9]))
	off := 9
	regions := make([]Region, regionCount)
	for i := 0; i < regionCount; i++ {
		if off+24 > len(data) { return nil, fmt.Errorf("g00/2: region data truncated") }
		regions[i] = Region{
			X1: int(binary.LittleEndian.Uint32(data[off:])),
			Y1: int(binary.LittleEndian.Uint32(data[off+4:])),
			X2: int(binary.LittleEndian.Uint32(data[off+8:])),
			Y2: int(binary.LittleEndian.Uint32(data[off+12:])),
			OriginX: int(binary.LittleEndian.Uint32(data[off+16:])),
			OriginY: int(binary.LittleEndian.Uint32(data[off+20:])),
		}
		off += 24
	}

	if off+8 > len(data) { return nil, fmt.Errorf("g00/2: missing compressed header") }
	compSize := int(binary.LittleEndian.Uint32(data[off:]))
	off += 4
	// uncompSize at data[off:off+4]
	off += 4
	
	if off+compSize-8 > len(data) {
		return nil, fmt.Errorf("g00/2: compressed data truncated")
	}
	compressed := data[off : off+compSize-8]
	decompressed := decompressG00_1(compressed, w*h*4+regionCount*1024)

	// Parse block index
	if len(decompressed) < 4 { return nil, fmt.Errorf("g00/2: decompressed too short") }
	blockCount := int(binary.LittleEndian.Uint32(decompressed[0:4]))
	
	// Composite image from blocks
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	
	// Read block index
	bidxOff := 4
	type blockEntry struct{ offset, length int }
	blocks := make([]blockEntry, blockCount)
	for i := 0; i < blockCount; i++ {
		if bidxOff+8 > len(decompressed) { break }
		blocks[i].offset = int(binary.LittleEndian.Uint32(decompressed[bidxOff:]))
		blocks[i].length = int(int32(binary.LittleEndian.Uint32(decompressed[bidxOff+4:])))
		bidxOff += 8
	}

	// Process each block
	for i, blk := range blocks {
		if blk.length <= 0 { continue } // negative = duplicate, skip
		if blk.offset >= len(decompressed) { continue }
		
		bd := decompressed[blk.offset:]
		if len(bd) < 0x74 { continue }
		
		// Read block header
		partCount := int(binary.LittleEndian.Uint16(bd[2:4]))
		bx := int(binary.LittleEndian.Uint32(bd[4:8]))
		by := int(binary.LittleEndian.Uint32(bd[8:12]))
		bw := int(binary.LittleEndian.Uint32(bd[12:16]))
		bh := int(binary.LittleEndian.Uint32(bd[16:20]))
		
		// Skip 0x74 header bytes, then read part data
		partOff := 0x74
		for p := 0; p < partCount; p++ {
			if partOff+8 > len(bd) { break }
			px := int(binary.LittleEndian.Uint16(bd[partOff:]))
			py := int(binary.LittleEndian.Uint16(bd[partOff+2:]))
			// transparency at bd[partOff+4:6]
			pw := int(binary.LittleEndian.Uint16(bd[partOff+6:]))
			ph := pw // approximate
			_ = ph
			partOff += 8
			
			// Read RGBA pixel data for this part
			pixelCount := pw * pw // approximate
			for pi := 0; pi < pixelCount && partOff+4 <= len(bd); pi++ {
				dx := bx + px + (pi % pw)
				dy := by + py + (pi / pw)
				if dx >= 0 && dx < w && dy >= 0 && dy < h {
					img.SetNRGBA(dx, dy, color.NRGBA{
						B: bd[partOff], G: bd[partOff+1], R: bd[partOff+2], A: bd[partOff+3],
					})
				}
				partOff += 4
			}
		}
		_ = bw
		_ = bh
		_ = i
	}

	return &Image{Width: w, Height: h, Format: 2, RGBA: img, Regions: regions}, nil
}

// ============================================================
// LZSS Decompression (from g00-bt.cpp)
// ============================================================

// decompressG00_0 decompresses format 0 data (3-byte RGB stride).
// Port of va_decompress_g00_0 from g00-bt.cpp.
func decompressG00_0(src []byte, maxOut int) []byte {
	buf := make([]byte, maxOut)
	si, di := 0, 0
	if len(src) == 0 { return buf }
	
	flag := src[si]; si++
	bit := 1

	for si < len(src) && di < maxOut {
		if bit == 256 {
			if si >= len(src) { break }
			flag = src[si]; si++
			bit = 1
		}
		if flag&byte(bit) != 0 {
			// Raw pixel (3 bytes)
			if si+3 > len(src) || di+3 > maxOut { break }
			buf[di] = src[si]; di++; si++
			buf[di] = src[si]; di++; si++
			buf[di] = src[si]; di++; si++
		} else {
			// Back-reference
			if si+2 > len(src) { break }
			count := int(src[si]) | int(src[si+1])<<8
			si += 2
			offset := (count >> 4) * 3  // offset in pixels * 3 bytes
			length := ((count & 0x0f) + 1) * 3  // length in pixels * 3 bytes
			rp := di - offset
			if rp < 0 { break }
			for i := 0; i < length && di < maxOut; i++ {
				buf[di] = buf[rp]; di++; rp++
			}
		}
		bit <<= 1
	}
	return buf[:di]
}

// decompressG00_1 decompresses format 1 data (1-byte stride).
// Port of va_decompress_g00_1 from g00-bt.cpp.
func decompressG00_1(src []byte, maxOut int) []byte {
	buf := make([]byte, maxOut)
	si, di := 0, 0
	if len(src) == 0 { return buf }

	flag := src[si]; si++
	bit := 1

	for si < len(src) && di < maxOut {
		if bit == 256 {
			if si >= len(src) { break }
			flag = src[si]; si++
			bit = 1
		}
		if flag&byte(bit) != 0 {
			// Raw byte
			if si >= len(src) || di >= maxOut { break }
			buf[di] = src[si]; di++; si++
		} else {
			// Back-reference
			if si+2 > len(src) { break }
			count := int(src[si]) | int(src[si+1])<<8
			si += 2
			offset := count >> 4
			length := (count & 0x0f) + 2
			rp := di - offset
			if rp < 0 { break }
			for i := 0; i < length && di < maxOut; i++ {
				buf[di] = buf[rp]; di++; rp++
			}
		}
		bit <<= 1
	}
	return buf[:di]
}

// ============================================================
// LZSS Compression (from g00-bt.cpp)
// ============================================================

// compressG00_0 compresses RGB data for format 0.
func compressG00_0(src []byte) []byte {
	dst := make([]byte, 0, len(src)*9/8+16)
	si := 0
	bitcount := 8
	var controlIdx int

	for si < len(src) {
		if bitcount > 7 {
			controlIdx = len(dst)
			dst = append(dst, 0)
			bitcount = 0
		}
		offset, length := findBestMatch0(src, si)
		if length > 0 {
			// Back-reference
			code := (offset&0x0f)<<4 | (length - 1)
			dst = append(dst, byte(code), byte(offset>>4))
			si += length * 3
		} else {
			// Raw pixel
			dst[controlIdx] |= 1 << bitcount
			dst = append(dst, src[si], src[si+1], src[si+2])
			si += 3
		}
		bitcount++
	}
	return dst
}

func findBestMatch0(src []byte, pos int) (offset, length int) {
	best := 0
	bestOff := 0
	for i := 1; pos-i*3 >= 0 && i < 4096 && best < 16; i++ {
		j := 0
		for pos+j*3+2 < len(src) &&
			src[pos+j*3] == src[pos-i*3+j*3] &&
			src[pos+j*3+1] == src[pos-i*3+j*3+1] &&
			src[pos+j*3+2] == src[pos-i*3+j*3+2] &&
			j < 16 {
			j++
			if j > best { best = j; bestOff = i }
		}
	}
	return bestOff, best
}

// compressG00_1 compresses data for format 1.
func compressG00_1(src []byte) []byte {
	dst := make([]byte, 0, len(src)*9/8+16)
	si := 0
	bitcount := 8
	var controlIdx int

	for si < len(src) {
		if bitcount > 7 {
			controlIdx = len(dst)
			dst = append(dst, 0)
			bitcount = 0
		}
		offset, length := findBestMatch1(src, si)
		if length > 1 {
			code := (offset&0x0f)<<4 | (length - 2)
			dst = append(dst, byte(code), byte(offset>>4))
			si += length
		} else {
			dst[controlIdx] |= 1 << bitcount
			dst = append(dst, src[si])
			si++
		}
		bitcount++
	}
	return dst
}

func findBestMatch1(src []byte, pos int) (offset, length int) {
	best := 0
	bestOff := 0
	for i := 1; pos-i >= 0 && i < 4096 && best < 17; i++ {
		j := 0
		for pos+j < len(src) && src[pos+j] == src[pos-i+j] && j < 17 {
			j++
			if j > best { best = j; bestOff = i }
		}
	}
	return bestOff, best
}

// ============================================================
// Writing
// ============================================================

// Encode encodes an image to G00 format.
func Encode(img *Image) ([]byte, error) {
	switch img.Format {
	case 0: return encodeFormat0(img)
	case 1: return nil, fmt.Errorf("g00: format 1 encoding not yet implemented")
	case 2: return nil, fmt.Errorf("g00: format 2 encoding not yet implemented")
	default: return nil, fmt.Errorf("g00: unknown format %d", img.Format)
	}
}

func encodeFormat0(img *Image) ([]byte, error) {
	// Convert RGBA to RGB
	rgb := make([]byte, img.Width*img.Height*3)
	for y := 0; y < img.Height; y++ {
		for x := 0; x < img.Width; x++ {
			c := img.RGBA.NRGBAAt(x, y)
			off := (y*img.Width + x) * 3
			rgb[off] = c.R
			rgb[off+1] = c.G
			rgb[off+2] = c.B
		}
	}

	compressed := compressG00_0(rgb)
	
	out := make([]byte, 13+len(compressed))
	out[0] = 0
	binary.LittleEndian.PutUint16(out[1:], uint16(img.Width))
	binary.LittleEndian.PutUint16(out[3:], uint16(img.Height))
	binary.LittleEndian.PutUint32(out[5:], uint32(len(compressed)+8))
	binary.LittleEndian.PutUint32(out[9:], uint32(img.Width*img.Height*4)) // w*h*4 per OCaml convention
	copy(out[13:], compressed)
	return out, nil
}

// WriteFile writes a G00 to disk.
func WriteFile(path string, img *Image) error {
	data, err := Encode(img)
	if err != nil { return err }
	return os.WriteFile(path, data, 0644)
}

// ============================================================
// PNG conversion
// ============================================================

// ToPNG converts a G00 image to PNG format.
func ToPNG(img *Image, w io.Writer) error {
	return png.Encode(w, img.RGBA)
}

// ToPNGFile writes a G00 image as a PNG file.
func ToPNGFile(img *Image, path string) error {
	f, err := os.Create(path)
	if err != nil { return err }
	defer f.Close()
	return ToPNG(img, f)
}

// FromPNG reads a PNG file and creates a G00 Image (format 0).
func FromPNG(r io.Reader) (*Image, error) {
	pngImg, err := png.Decode(r)
	if err != nil { return nil, err }
	
	bounds := pngImg.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	rgba := image.NewNRGBA(image.Rect(0, 0, w, h))
	
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, a := pngImg.At(x+bounds.Min.X, y+bounds.Min.Y).RGBA()
			rgba.SetNRGBA(x, y, color.NRGBA{R: uint8(r>>8), G: uint8(g>>8), B: uint8(b>>8), A: uint8(a>>8)})
		}
	}
	
	return &Image{Width: w, Height: h, Format: 0, RGBA: rgba}, nil
}

// FromPNGFile reads a PNG file.
func FromPNGFile(path string) (*Image, error) {
	f, err := os.Open(path)
	if err != nil { return nil, err }
	defer f.Close()
	return FromPNG(f)
}
