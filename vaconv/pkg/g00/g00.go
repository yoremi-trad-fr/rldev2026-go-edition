// Package g00 handles the G00 image format used by RealLive visual novels.
//
// Ported from vaconv (OCaml + C++):
//   - g00.ml (657 lines)      — format read/write logic
//   - g00-bt.cpp (268 lines)  — LZSS compression/decompression
//
// Three G00 formats exist:
//
//	Format 0: BGR bitmap (24-bit, LZSS compressed with 3-byte stride)
//	Format 1: Paletted bitmap (8-bit + RGBA palette, LZSS compressed)
//	Format 2: Composite RGBA with regions/sub-images
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
	Format        int          // 0, 1, or 2
	RGBA          *image.NRGBA // decoded pixel data
	Regions       []Region     // format 2 regions
}

// Region describes a sub-image region in format 2.
type Region struct {
	X1, Y1, X2, Y2   int
	OriginX, OriginY int
	Parts            []Part
}

// Part describes a format 2 part inside a region.
type Part struct {
	X, Y          int
	Width, Height int
	Trans         int
}

func leI16(b []byte) int {
	return int(int16(binary.LittleEndian.Uint16(b)))
}

func leI32(b []byte) int {
	return int(int32(binary.LittleEndian.Uint32(b)))
}

func defaultRegion(w, h int) Region {
	return Region{X1: 0, Y1: 0, X2: w - 1, Y2: h - 1}
}

func regionWidth(r Region) int {
	return r.X2 - r.X1 + 1
}

func regionHeight(r Region) int {
	return r.Y2 - r.Y1 + 1
}

// ============================================================
// Reading
// ============================================================

// ReadFile reads a G00 file and returns the decoded image.
func ReadFile(path string) (*Image, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Decode(data)
}

// Decode decodes G00 data from a byte slice.
func Decode(data []byte) (*Image, error) {
	if len(data) < 9 {
		return nil, fmt.Errorf("g00: too short")
	}
	format := int(data[0])
	w := int(binary.LittleEndian.Uint16(data[1:3]))
	h := int(binary.LittleEndian.Uint16(data[3:5]))

	switch format {
	case 0:
		return decodeFormat0(data, w, h)
	case 1:
		return decodeFormat1(data, w, h)
	case 2:
		return decodeFormat2(data, w, h)
	default:
		return nil, fmt.Errorf("g00: unknown format %d", format)
	}
}

// --- Format 0: BGR bitmap ---

func decodeFormat0(data []byte, w, h int) (*Image, error) {
	compSize := int(binary.LittleEndian.Uint32(data[5:9]))
	uncompSize := int(binary.LittleEndian.Uint32(data[9:13]))
	if uncompSize != w*h*3 { // BGR, 3 bytes per pixel
		// Some files report w*h*4
	}
	if compSize+5 > len(data) {
		return nil, fmt.Errorf("g00/0: compressed data exceeds file size")
	}
	compressed := data[13 : 5+compSize]
	bgr := decompressG00_0(compressed, w*h*3)

	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			off := (y*w + x) * 3
			if off+2 < len(bgr) {
				img.SetNRGBA(x, y, color.NRGBA{R: bgr[off+2], G: bgr[off+1], B: bgr[off], A: 255})
			}
		}
	}
	return &Image{Width: w, Height: h, Format: 0, RGBA: img}, nil
}

// --- Format 1: Paletted bitmap ---

func decodeFormat1(data []byte, w, h int) (*Image, error) {
	compSize := int(binary.LittleEndian.Uint32(data[5:9]))
	uncompSize := int(binary.LittleEndian.Uint32(data[9:13]))
	if 5+compSize > len(data) {
		return nil, fmt.Errorf("g00/1: data truncated")
	}
	compressed := data[13 : 5+compSize]
	decompressed := decompressG00_1(compressed, uncompSize)
	if len(decompressed) < 2 {
		return nil, fmt.Errorf("g00/1: decompressed data too short")
	}

	// Parse palette
	palLen := int(binary.LittleEndian.Uint16(decompressed[0:2]))
	if palLen > 256 {
		palLen = 256
	}
	if len(decompressed) < 2+palLen*4+w*h {
		return nil, fmt.Errorf("g00/1: decompressed data truncated")
	}
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
		if off+24 > len(data) {
			return nil, fmt.Errorf("g00/2: region data truncated")
		}
		regions[i] = Region{
			X1:      leI32(data[off:]),
			Y1:      leI32(data[off+4:]),
			X2:      leI32(data[off+8:]),
			Y2:      leI32(data[off+12:]),
			OriginX: leI32(data[off+16:]),
			OriginY: leI32(data[off+20:]),
		}
		off += 24
	}

	if off+8 > len(data) {
		return nil, fmt.Errorf("g00/2: missing compressed header")
	}
	compSize := int(binary.LittleEndian.Uint32(data[off:]))
	off += 4
	uncompSize := int(binary.LittleEndian.Uint32(data[off:]))
	off += 4

	if compSize < 8 || off+compSize-8 > len(data) {
		return nil, fmt.Errorf("g00/2: compressed data truncated")
	}
	compressed := data[off : off+compSize-8]
	decompressed := decompressG00_1(compressed, uncompSize)
	if len(decompressed) < uncompSize {
		return nil, fmt.Errorf("g00/2: decompressed data truncated")
	}

	// Parse block index
	if len(decompressed) < 4 {
		return nil, fmt.Errorf("g00/2: decompressed too short")
	}
	blockCount := int(binary.LittleEndian.Uint32(decompressed[0:4]))

	// Composite image from blocks
	img := image.NewNRGBA(image.Rect(0, 0, w, h))

	// Read block index
	bidxOff := 4
	type blockEntry struct{ offset, length int }
	blocks := make([]blockEntry, blockCount)
	for i := 0; i < blockCount; i++ {
		if bidxOff+8 > len(decompressed) {
			return nil, fmt.Errorf("g00/2: block index truncated")
		}
		blocks[i].offset = leI32(decompressed[bidxOff:])
		blocks[i].length = leI32(decompressed[bidxOff+4:])
		bidxOff += 8
	}

	// Process each block
	for i, blk := range blocks {
		if blk.length <= 0 {
			continue
		} // negative = duplicate, skip
		if blk.offset < 0 || blk.offset >= len(decompressed) {
			continue
		}
		if blk.offset+blk.length > len(decompressed) {
			return nil, fmt.Errorf("g00/2: block %d exceeds decompressed data", i)
		}

		bd := decompressed[blk.offset : blk.offset+blk.length]
		if len(bd) < 0x74 {
			continue
		}

		// Read block header
		if binary.LittleEndian.Uint16(bd[0:2]) != 1 {
			return nil, fmt.Errorf("g00/2: unsupported block type %d", binary.LittleEndian.Uint16(bd[0:2]))
		}
		partCount := int(binary.LittleEndian.Uint16(bd[2:4]))
		r := Region{}
		if i < len(regions) {
			r = regions[i]
		}

		// Skip 0x74 header bytes, then read part data
		partOff := 0x74
		for p := 0; p < partCount; p++ {
			if partOff+0x5c > len(bd) {
				return nil, fmt.Errorf("g00/2: part header truncated")
			}
			px := leI16(bd[partOff:])
			py := leI16(bd[partOff+2:])
			trans := leI16(bd[partOff+4:])
			pw := int(binary.LittleEndian.Uint16(bd[partOff+6:]))
			ph := int(binary.LittleEndian.Uint16(bd[partOff+8:]))
			partOff += 0x5c

			pixelBytes := pw * ph * 4
			if pixelBytes < 0 || partOff+pixelBytes > len(bd) {
				return nil, fmt.Errorf("g00/2: part pixels truncated")
			}
			if i < len(regions) {
				regions[i].Parts = append(regions[i].Parts, Part{
					X: px, Y: py, Width: pw, Height: ph, Trans: trans,
				})
			}
			for y := 0; y < ph; y++ {
				for x := 0; x < pw; x++ {
					src := partOff + (y*pw+x)*4
					dx := r.X1 + px + x
					dy := r.Y1 + py + y
					if dx >= 0 && dx < w && dy >= 0 && dy < h {
						img.SetNRGBA(dx, dy, color.NRGBA{
							R: bd[src+2], G: bd[src+1], B: bd[src], A: bd[src+3],
						})
					}
				}
			}
			partOff += pixelBytes
		}
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
	if len(src) == 0 {
		return buf
	}

	flag := src[si]
	si++
	bit := 1

	for si < len(src) && di < maxOut {
		if bit == 256 {
			if si >= len(src) {
				break
			}
			flag = src[si]
			si++
			bit = 1
		}
		if flag&byte(bit) != 0 {
			// Raw pixel (3 bytes)
			if si+3 > len(src) || di+3 > maxOut {
				break
			}
			buf[di] = src[si]
			di++
			si++
			buf[di] = src[si]
			di++
			si++
			buf[di] = src[si]
			di++
			si++
		} else {
			// Back-reference
			if si+2 > len(src) {
				break
			}
			count := int(src[si]) | int(src[si+1])<<8
			si += 2
			offset := (count >> 4) * 3         // offset in pixels * 3 bytes
			length := ((count & 0x0f) + 1) * 3 // length in pixels * 3 bytes
			rp := di - offset
			if rp < 0 {
				break
			}
			for i := 0; i < length && di < maxOut; i++ {
				buf[di] = buf[rp]
				di++
				rp++
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
	if len(src) == 0 {
		return buf
	}

	flag := src[si]
	si++
	bit := 1

	for si < len(src) && di < maxOut {
		if bit == 256 {
			if si >= len(src) {
				break
			}
			flag = src[si]
			si++
			bit = 1
		}
		if flag&byte(bit) != 0 {
			// Raw byte
			if si >= len(src) || di >= maxOut {
				break
			}
			buf[di] = src[si]
			di++
			si++
		} else {
			// Back-reference
			if si+2 > len(src) {
				break
			}
			count := int(src[si]) | int(src[si+1])<<8
			si += 2
			offset := count >> 4
			length := (count & 0x0f) + 2
			rp := di - offset
			if rp < 0 {
				break
			}
			for i := 0; i < length && di < maxOut; i++ {
				buf[di] = buf[rp]
				di++
				rp++
			}
		}
		bit <<= 1
	}
	return buf[:di]
}

// ============================================================
// LZSS Compression (from g00-bt.cpp)
// ============================================================

// compressG00_0 compresses BGR data for format 0.
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
			if j > best {
				best = j
				bestOff = i
			}
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
			if j > best {
				best = j
				bestOff = i
			}
		}
	}
	return bestOff, best
}

func compressG00_1Raw(src []byte) []byte {
	dst := make([]byte, 0, len(src)+len(src)/8+1)
	for i := 0; i < len(src); {
		controlIdx := len(dst)
		dst = append(dst, 0)
		for bit := 0; bit < 8 && i < len(src); bit++ {
			dst[controlIdx] |= 1 << bit
			dst = append(dst, src[i])
			i++
		}
	}
	return dst
}

// ============================================================
// Writing
// ============================================================

// Encode encodes an image to G00 format.
func Encode(img *Image) ([]byte, error) {
	switch img.Format {
	case 0:
		return encodeFormat0(img)
	case 1:
		return encodeFormat1(img)
	case 2:
		return encodeFormat2(img)
	default:
		return nil, fmt.Errorf("g00: unknown format %d", img.Format)
	}
}

func encodeFormat0(img *Image) ([]byte, error) {
	// Convert RGBA to the BGR byte order expected by RealLive G00 format 0.
	bgr := make([]byte, img.Width*img.Height*3)
	for y := 0; y < img.Height; y++ {
		for x := 0; x < img.Width; x++ {
			c := img.RGBA.NRGBAAt(x, y)
			off := (y*img.Width + x) * 3
			bgr[off] = c.B
			bgr[off+1] = c.G
			bgr[off+2] = c.R
		}
	}

	compressed := compressG00_0(bgr)

	out := make([]byte, 13+len(compressed))
	out[0] = 0
	binary.LittleEndian.PutUint16(out[1:], uint16(img.Width))
	binary.LittleEndian.PutUint16(out[3:], uint16(img.Height))
	binary.LittleEndian.PutUint32(out[5:], uint32(len(compressed)+8))
	binary.LittleEndian.PutUint32(out[9:], uint32(img.Width*img.Height*4)) // w*h*4 per OCaml convention
	copy(out[13:], compressed)
	return out, nil
}

func encodeFormat1(img *Image) ([]byte, error) {
	paletteMap := map[color.NRGBA]int{}
	palette := make([]color.NRGBA, 0, 256)
	indices := make([]byte, img.Width*img.Height)
	for y := 0; y < img.Height; y++ {
		for x := 0; x < img.Width; x++ {
			c := img.RGBA.NRGBAAt(x, y)
			idx, ok := paletteMap[c]
			if !ok {
				if len(palette) >= 256 {
					return nil, fmt.Errorf("g00/1: image has more than 256 unique colours")
				}
				idx = len(palette)
				paletteMap[c] = idx
				palette = append(palette, c)
			}
			indices[y*img.Width+x] = byte(idx)
		}
	}

	uncompressed := make([]byte, 2+len(palette)*4+len(indices))
	binary.LittleEndian.PutUint16(uncompressed[0:2], uint16(len(palette)))
	for i, c := range palette {
		off := 2 + i*4
		uncompressed[off] = c.R
		uncompressed[off+1] = c.G
		uncompressed[off+2] = c.B
		uncompressed[off+3] = c.A
	}
	copy(uncompressed[2+len(palette)*4:], indices)
	compressed := compressG00_1(uncompressed)

	out := make([]byte, 13+len(compressed))
	out[0] = 1
	binary.LittleEndian.PutUint16(out[1:], uint16(img.Width))
	binary.LittleEndian.PutUint16(out[3:], uint16(img.Height))
	binary.LittleEndian.PutUint32(out[5:], uint32(len(compressed)+8))
	binary.LittleEndian.PutUint32(out[9:], uint32(len(uncompressed)))
	copy(out[13:], compressed)
	return out, nil
}

func encodeFormat2(img *Image) ([]byte, error) {
	regions := img.Regions
	if len(regions) == 0 {
		regions = []Region{defaultRegion(img.Width, img.Height)}
	}

	dataHeaderLen := 4 + len(regions)*8
	data := make([]byte, dataHeaderLen)
	binary.LittleEndian.PutUint32(data[0:], uint32(len(regions)))

	for i, r := range regions {
		w := regionWidth(r)
		h := regionHeight(r)
		offset := len(data)
		if w > 0 && h > 0 {
			block, err := encodeFormat2Block(img, r, w, h)
			if err != nil {
				return nil, err
			}
			data = append(data, block...)
		}
		length := len(data) - offset
		binary.LittleEndian.PutUint32(data[4+i*8:], uint32(offset))
		binary.LittleEndian.PutUint32(data[8+i*8:], uint32(length))
	}

	compressed := compressG00_1Raw(data)
	headerLen := 9 + len(regions)*24
	out := make([]byte, headerLen+8+len(compressed))
	out[0] = 2
	binary.LittleEndian.PutUint16(out[1:], uint16(img.Width))
	binary.LittleEndian.PutUint16(out[3:], uint16(img.Height))
	binary.LittleEndian.PutUint32(out[5:], uint32(len(regions)))
	for i, r := range regions {
		off := 9 + i*24
		binary.LittleEndian.PutUint32(out[off:], uint32(int32(r.X1)))
		binary.LittleEndian.PutUint32(out[off+4:], uint32(int32(r.Y1)))
		binary.LittleEndian.PutUint32(out[off+8:], uint32(int32(r.X2)))
		binary.LittleEndian.PutUint32(out[off+12:], uint32(int32(r.Y2)))
		binary.LittleEndian.PutUint32(out[off+16:], uint32(int32(r.OriginX)))
		binary.LittleEndian.PutUint32(out[off+20:], uint32(int32(r.OriginY)))
	}
	binary.LittleEndian.PutUint32(out[headerLen:], uint32(len(compressed)+8))
	binary.LittleEndian.PutUint32(out[headerLen+4:], uint32(len(data)))
	copy(out[headerLen+8:], compressed)
	return out, nil
}

func encodeFormat2Block(img *Image, r Region, w, h int) ([]byte, error) {
	if w > 0xffff || h > 0xffff {
		return nil, fmt.Errorf("g00/2: region too large: %dx%d", w, h)
	}
	blockLen := 0x74 + 0x5c + w*h*4
	block := make([]byte, blockLen)
	binary.LittleEndian.PutUint16(block[0x00:], 1)
	binary.LittleEndian.PutUint16(block[0x02:], 1)
	binary.LittleEndian.PutUint32(block[0x04:], 0)
	binary.LittleEndian.PutUint32(block[0x08:], 0)
	binary.LittleEndian.PutUint32(block[0x0c:], uint32(w))
	binary.LittleEndian.PutUint32(block[0x10:], uint32(h))
	binary.LittleEndian.PutUint32(block[0x14:], uint32(int32(r.OriginX)))
	binary.LittleEndian.PutUint32(block[0x18:], uint32(int32(r.OriginY)))
	binary.LittleEndian.PutUint32(block[0x1c:], uint32(w))
	binary.LittleEndian.PutUint32(block[0x20:], uint32(h))

	part := block[0x74:]
	binary.LittleEndian.PutUint16(part[0x00:], 0)
	binary.LittleEndian.PutUint16(part[0x02:], 0)
	binary.LittleEndian.PutUint16(part[0x04:], 1)
	binary.LittleEndian.PutUint16(part[0x06:], uint16(w))
	binary.LittleEndian.PutUint16(part[0x08:], uint16(h))
	pixelOff := 0x74 + 0x5c
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst := pixelOff + (y*w+x)*4
			sx := r.X1 + x
			sy := r.Y1 + y
			if sx >= 0 && sx < img.Width && sy >= 0 && sy < img.Height {
				c := img.RGBA.NRGBAAt(sx, sy)
				block[dst] = c.B
				block[dst+1] = c.G
				block[dst+2] = c.R
				block[dst+3] = c.A
			}
		}
	}
	return block, nil
}

// WriteFile writes a G00 to disk.
func WriteFile(path string, img *Image) error {
	data, err := Encode(img)
	if err != nil {
		return err
	}
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
	if err != nil {
		return err
	}
	defer f.Close()
	return ToPNG(img, f)
}

// FromPNG reads a PNG file and creates a G00 Image (format 0).
func FromPNG(r io.Reader) (*Image, error) {
	pngImg, err := png.Decode(r)
	if err != nil {
		return nil, err
	}

	bounds := pngImg.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	rgba := image.NewNRGBA(image.Rect(0, 0, w, h))

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.NRGBAModel.Convert(pngImg.At(x+bounds.Min.X, y+bounds.Min.Y)).(color.NRGBA)
			rgba.SetNRGBA(x, y, c)
		}
	}

	return &Image{Width: w, Height: h, Format: 0, RGBA: rgba}, nil
}

// FromPNGFile reads a PNG file.
func FromPNGFile(path string) (*Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return FromPNG(f)
}
