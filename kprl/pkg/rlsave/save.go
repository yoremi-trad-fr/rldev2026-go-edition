// Package rlsave reads and edits RealLive save files.
package rlsave

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/yoremi/rldev-go/pkg/compression"
)

const (
	KindUnknown    Kind = "unknown"
	KindGame       Kind = "game"
	KindGlobal     Kind = "global"
	KindSystem     Kind = "system"
	GlobalIntCount      = 4000
)

type Kind string

type Save struct {
	Path             string
	Kind             Kind
	Label            string
	HeaderLen        int
	CompressedSize   int
	UncompressedSize int
	Body             []byte
	raw              []byte
}

type WriteOptions struct {
	Backup bool
}

type WriteResult struct {
	Path       string
	BackupPath string
	OldSize    int
	NewSize    int
}

func ReadFile(path string) (*Save, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	save, err := Parse(raw)
	if err != nil {
		return nil, err
	}
	save.Path = path
	return save, nil
}

func Parse(raw []byte) (*Save, error) {
	if len(raw) < 32 {
		return nil, fmt.Errorf("save file too small: %d bytes", len(raw))
	}

	headerLen := int(le32(raw, 0))
	if headerLen < 32 || headerLen+8 > len(raw) {
		return nil, fmt.Errorf("invalid save header length %d for file size %d", headerLen, len(raw))
	}

	compSize := int(le32(raw, headerLen))
	uncompSize := int(le32(raw, headerLen+4))
	if compSize < 9 || uncompSize <= 0 {
		return nil, fmt.Errorf("invalid compressed body sizes: compressed=%d uncompressed=%d", compSize, uncompSize)
	}
	if headerLen+compSize > len(raw) {
		return nil, fmt.Errorf("compressed body overruns file: header=%d compressed=%d file=%d", headerLen, compSize, len(raw))
	}

	body := make([]byte, uncompSize)
	if err := compression.Decompress(raw[headerLen:headerLen+compSize], body); err != nil {
		return nil, fmt.Errorf("decompress save body: %w", err)
	}

	label := readCString(raw, 0x18, 64)
	return &Save{
		Kind:             detectKind(label),
		Label:            label,
		HeaderLen:        headerLen,
		CompressedSize:   compSize,
		UncompressedSize: uncompSize,
		Body:             body,
		raw:              bytes.Clone(raw),
	}, nil
}

func (s *Save) Bytes() ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("nil save")
	}
	if s.HeaderLen < 32 || s.HeaderLen > len(s.raw) {
		return nil, fmt.Errorf("invalid header length %d", s.HeaderLen)
	}
	if len(s.Body) == 0 {
		return nil, fmt.Errorf("empty save body")
	}

	payload := compression.Compress(s.Body)
	compSize := len(payload) + 8
	trailingOffset := s.HeaderLen + s.CompressedSize
	var trailing []byte
	if trailingOffset < len(s.raw) {
		trailing = s.raw[trailingOffset:]
	}

	out := make([]byte, s.HeaderLen+compSize+len(trailing))
	copy(out[:s.HeaderLen], s.raw[:s.HeaderLen])

	// RealLive saves duplicate the body sizes just before the compressed body.
	// Updating these offsets covers regular slots and save999.sav.
	if s.HeaderLen >= 8 {
		put32(out, s.HeaderLen-8, uint32(len(s.Body)))
		put32(out, s.HeaderLen-4, uint32(compSize))
	}
	put32(out, s.HeaderLen, uint32(compSize))
	put32(out, s.HeaderLen+4, uint32(len(s.Body)))
	copy(out[s.HeaderLen+8:], payload)
	copy(out[s.HeaderLen+compSize:], trailing)

	return out, nil
}

func (s *Save) WriteFile(path string, opts WriteOptions) (WriteResult, error) {
	if path == "" {
		path = s.Path
	}
	if path == "" {
		return WriteResult{}, fmt.Errorf("no output path")
	}
	out, err := s.Bytes()
	if err != nil {
		return WriteResult{}, err
	}

	result := WriteResult{Path: path, OldSize: len(s.raw), NewSize: len(out)}
	if opts.Backup {
		backup := path + ".bak-" + time.Now().Format("20060102-150405")
		if err := os.WriteFile(backup, s.raw, 0666); err != nil {
			return result, fmt.Errorf("write backup: %w", err)
		}
		if abs, err := filepath.Abs(backup); err == nil {
			result.BackupPath = abs
		} else {
			result.BackupPath = backup
		}
	}

	if err := os.WriteFile(path, out, 0666); err != nil {
		return result, err
	}
	s.raw = bytes.Clone(out)
	s.CompressedSize = int(le32(out, s.HeaderLen))
	s.UncompressedSize = int(le32(out, s.HeaderLen+4))
	return result, nil
}

func (s *Save) GlobalInt(index int) (int32, error) {
	if err := s.checkGlobalIntIndex(index); err != nil {
		return 0, err
	}
	return int32(le32(s.Body, index*4)), nil
}

func (s *Save) SetGlobalInt(index int, value int32) error {
	if err := s.checkGlobalIntIndex(index); err != nil {
		return err
	}
	put32(s.Body, index*4, uint32(value))
	return nil
}

func (s *Save) NonZeroGlobalInts() ([]GlobalInt, error) {
	if s.Kind != KindGlobal {
		return nil, fmt.Errorf("intG editing is currently supported only for AVG_GLOBAL_SAVE/save999.sav")
	}
	if len(s.Body) < GlobalIntCount*4 {
		return nil, fmt.Errorf("global body too small for intG table: %d bytes", len(s.Body))
	}
	var ints []GlobalInt
	for i := 0; i < GlobalIntCount; i++ {
		v := int32(le32(s.Body, i*4))
		if v != 0 {
			ints = append(ints, GlobalInt{Index: i, Value: v})
		}
	}
	return ints, nil
}

type GlobalInt struct {
	Index int   `json:"index"`
	Value int32 `json:"value"`
}

func (s *Save) checkGlobalIntIndex(index int) error {
	if s.Kind != KindGlobal {
		return fmt.Errorf("intG editing is currently supported only for AVG_GLOBAL_SAVE/save999.sav")
	}
	if index < 0 || index >= GlobalIntCount {
		return fmt.Errorf("intG index %d out of range 0..%d", index, GlobalIntCount-1)
	}
	if len(s.Body) < GlobalIntCount*4 {
		return fmt.Errorf("global body too small for intG table: %d bytes", len(s.Body))
	}
	return nil
}

func detectKind(label string) Kind {
	switch label {
	case "AVG_GLOBAL_SAVE":
		return KindGlobal
	case "AVG_SYSTEM_SAVE":
		return KindSystem
	case "":
		return KindUnknown
	default:
		return KindGame
	}
}

func le32(buf []byte, off int) uint32 {
	return binary.LittleEndian.Uint32(buf[off : off+4])
}

func put32(buf []byte, off int, v uint32) {
	binary.LittleEndian.PutUint32(buf[off:off+4], v)
}

func readCString(buf []byte, off, max int) string {
	if off >= len(buf) || max <= 0 {
		return ""
	}
	end := off + max
	if end > len(buf) {
		end = len(buf)
	}
	window := buf[off:end]
	if idx := bytes.IndexByte(window, 0); idx >= 0 {
		window = window[:idx]
	}
	return string(window)
}
