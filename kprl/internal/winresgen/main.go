package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf16"
)

const (
	coffMachineAMD64 = 0x8664

	imageScnCntInitializedData = 0x00000040
	imageScnMemRead            = 0x40000000

	imageSymClassStatic   = 3
	imageRelAMD64Addr32NB = 0x0003

	rtVersion  = 16
	rtManifest = 24

	langEnglishUS = 0x0409
	codepageUTF16 = 0x04B0
)

type toolInfo struct {
	OutPath      string
	InternalName string
	OriginalName string
	Description  string
}

type resource struct {
	TypeID   uint16
	NameID   uint16
	LangID   uint16
	Data     []byte
	Codepage uint32
}

type sectionReloc struct {
	Offset uint32
}

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()

	version := [4]uint16{1, 3, 4, 0}
	tools := []toolInfo{
		{
			OutPath:      filepath.Join(*root, "kprl", "cmd", "kprl", "rldev_windows_amd64.syso"),
			InternalName: "kprl16",
			OriginalName: "kprl16.exe",
			Description:  "KPRL16 - RealLive archive and disassembly tool",
		},
		{
			OutPath:      filepath.Join(*root, "rlc", "cmd", "rlc", "rldev_windows_amd64.syso"),
			InternalName: "rlc2026",
			OriginalName: "rlc2026.exe",
			Description:  "RLC2026 - RealLive Kepago compiler",
		},
		{
			OutPath:      filepath.Join(*root, "rlxml", "cmd", "rlxml", "rldev_windows_amd64.syso"),
			InternalName: "rlxml",
			OriginalName: "rlxml.exe",
			Description:  "RLXML - RealLive GAN XML converter",
		},
		{
			OutPath:      filepath.Join(*root, "vaconv", "cmd", "vaconv", "rldev_windows_amd64.syso"),
			InternalName: "vaconv",
			OriginalName: "vaconv.exe",
			Description:  "Vaconv - VisualArt's G00 image converter",
		},
		{
			OutPath:      filepath.Join(*root, "kprl", "cmd", "rlsave", "rldev_windows_amd64.syso"),
			InternalName: "rlsave",
			OriginalName: "rlsave.exe",
			Description:  "RlSave - RealLive save inspector and editor",
		},
	}

	for _, tool := range tools {
		data, err := buildCOFF(tool, version)
		if err != nil {
			fatal("%s: %v", tool.OriginalName, err)
		}
		if err := os.MkdirAll(filepath.Dir(tool.OutPath), 0o755); err != nil {
			fatal("%s: %v", tool.OutPath, err)
		}
		if err := os.WriteFile(tool.OutPath, data, 0o644); err != nil {
			fatal("%s: %v", tool.OutPath, err)
		}
		fmt.Printf("  wrote %s\n", rel(tool.OutPath, *root))
	}
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "winresgen: "+format+"\n", args...)
	os.Exit(1)
}

func rel(path, root string) string {
	if r, err := filepath.Rel(root, path); err == nil {
		return r
	}
	return path
}

func buildCOFF(tool toolInfo, version [4]uint16) ([]byte, error) {
	versionInfo := buildVersionInfo(tool, version)
	manifest := []byte(buildManifest(tool.InternalName, version))
	resources := []resource{
		{TypeID: rtVersion, NameID: 1, LangID: langEnglishUS, Data: versionInfo, Codepage: codepageUTF16},
		{TypeID: rtManifest, NameID: 1, LangID: langEnglishUS, Data: manifest, Codepage: 65001},
	}

	section, relocs := buildResourceSection(resources)
	return buildCOFFObject(section, relocs), nil
}

func buildManifest(name string, version [4]uint16) string {
	fullVersion := fmt.Sprintf("%d.%d.%d.%d", version[0], version[1], version[2], version[3])
	identity := "Yoremi.RLdev2026Go." + sanitizeManifestName(name)
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly manifestVersion="1.0" xmlns="urn:schemas-microsoft-com:asm.v1" xmlns:asmv3="urn:schemas-microsoft-com:asm.v3">
  <assemblyIdentity type="win32" name="%s" version="%s" processorArchitecture="*"/>
  <trustInfo xmlns="urn:schemas-microsoft-com:asm.v3">
    <security>
      <requestedPrivileges>
        <requestedExecutionLevel level="asInvoker" uiAccess="false"/>
      </requestedPrivileges>
    </security>
  </trustInfo>
  <asmv3:application>
    <asmv3:windowsSettings>
      <longPathAware xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">true</longPathAware>
    </asmv3:windowsSettings>
  </asmv3:application>
</assembly>
`, identity, fullVersion)
}

func sanitizeManifestName(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "tool"
	}
	return b.String()
}

func buildVersionInfo(tool toolInfo, version [4]uint16) []byte {
	versionText := fmt.Sprintf("%d.%d.%d.%d", version[0], version[1], version[2], version[3])
	strings := [][2]string{
		{"CompanyName", "Yoremi"},
		{"FileDescription", tool.Description},
		{"FileVersion", versionText},
		{"InternalName", tool.InternalName},
		{"LegalCopyright", "Copyright (c) 2026 Yoremi"},
		{"OriginalFilename", tool.OriginalName},
		{"ProductName", "RLdev2026-Go"},
		{"ProductVersion", versionText},
		{"Comments", "Open source RealLive and VisualArt's command line tool."},
	}

	var root blockBuilder
	root.word(0)  // length placeholder
	root.word(52) // VS_FIXEDFILEINFO size in bytes
	root.word(0)
	root.utf16z("VS_VERSION_INFO")
	root.align4()
	root.fixedFileInfo(version)
	root.align4()
	root.bytes(buildStringFileInfo(strings))
	root.bytes(buildVarFileInfo())
	root.patchLength(0)
	return root.buf.Bytes()
}

func buildStringFileInfo(items [][2]string) []byte {
	var b blockBuilder
	b.word(0)
	b.word(0)
	b.word(1)
	b.utf16z("StringFileInfo")
	b.align4()
	b.bytes(buildStringTable(items))
	b.patchLength(0)
	return b.buf.Bytes()
}

func buildStringTable(items [][2]string) []byte {
	var b blockBuilder
	b.word(0)
	b.word(0)
	b.word(1)
	b.utf16z("040904B0")
	b.align4()
	for _, item := range items {
		b.bytes(buildStringEntry(item[0], item[1]))
	}
	b.patchLength(0)
	return b.buf.Bytes()
}

func buildStringEntry(key, value string) []byte {
	var b blockBuilder
	b.word(0)
	b.word(uint16(len(utf16.Encode([]rune(value))) + 1))
	b.word(1)
	b.utf16z(key)
	b.align4()
	b.utf16z(value)
	b.align4()
	b.patchLength(0)
	return b.buf.Bytes()
}

func buildVarFileInfo() []byte {
	var b blockBuilder
	b.word(0)
	b.word(0)
	b.word(1)
	b.utf16z("VarFileInfo")
	b.align4()
	b.bytes(buildTranslationVar())
	b.patchLength(0)
	return b.buf.Bytes()
}

func buildTranslationVar() []byte {
	var b blockBuilder
	b.word(0)
	b.word(4)
	b.word(0)
	b.utf16z("Translation")
	b.align4()
	b.word(langEnglishUS)
	b.word(codepageUTF16)
	b.align4()
	b.patchLength(0)
	return b.buf.Bytes()
}

type blockBuilder struct {
	buf bytes.Buffer
}

func (b *blockBuilder) word(v uint16) {
	_ = binary.Write(&b.buf, binary.LittleEndian, v)
}

func (b *blockBuilder) dword(v uint32) {
	_ = binary.Write(&b.buf, binary.LittleEndian, v)
}

func (b *blockBuilder) bytes(data []byte) {
	_, _ = b.buf.Write(data)
}

func (b *blockBuilder) align4() {
	for b.buf.Len()%4 != 0 {
		b.buf.WriteByte(0)
	}
}

func (b *blockBuilder) utf16z(s string) {
	for _, v := range utf16.Encode([]rune(s)) {
		b.word(v)
	}
	b.word(0)
}

func (b *blockBuilder) patchLength(pos int) {
	data := b.buf.Bytes()
	binary.LittleEndian.PutUint16(data[pos:], uint16(len(data)-pos))
}

func (b *blockBuilder) fixedFileInfo(version [4]uint16) {
	fileMS := uint32(version[0])<<16 | uint32(version[1])
	fileLS := uint32(version[2])<<16 | uint32(version[3])
	b.dword(0xFEEF04BD) // dwSignature
	b.dword(0x00010000) // dwStrucVersion
	b.dword(fileMS)
	b.dword(fileLS)
	b.dword(fileMS)
	b.dword(fileLS)
	b.dword(0x0000003F) // dwFileFlagsMask
	b.dword(0x00000000) // dwFileFlags
	b.dword(0x00040004) // VOS_NT_WINDOWS32
	b.dword(0x00000001) // VFT_APP
	b.dword(0x00000000)
	b.dword(0x00000000)
	b.dword(0x00000000)
}

func buildResourceSection(resources []resource) ([]byte, []sectionReloc) {
	sort.Slice(resources, func(i, j int) bool {
		if resources[i].TypeID != resources[j].TypeID {
			return resources[i].TypeID < resources[j].TypeID
		}
		if resources[i].NameID != resources[j].NameID {
			return resources[i].NameID < resources[j].NameID
		}
		return resources[i].LangID < resources[j].LangID
	})

	type group struct {
		typeID uint16
		res    []resource
	}
	var groups []group
	for _, r := range resources {
		if len(groups) == 0 || groups[len(groups)-1].typeID != r.TypeID {
			groups = append(groups, group{typeID: r.TypeID})
		}
		groups[len(groups)-1].res = append(groups[len(groups)-1].res, r)
	}

	var buf bytes.Buffer
	var relocs []sectionReloc

	writeDir := func(named, ids uint16) {
		writeU32(&buf, 0) // Characteristics
		writeU32(&buf, 0) // TimeDateStamp
		writeU16(&buf, 0) // MajorVersion
		writeU16(&buf, 0) // MinorVersion
		writeU16(&buf, named)
		writeU16(&buf, ids)
	}
	writeEntry := func(id uint16, off uint32, isDir bool) {
		writeU32(&buf, uint32(id))
		if isDir {
			off |= 0x80000000
		}
		writeU32(&buf, off)
	}

	rootOff := buf.Len()
	writeDir(0, uint16(len(groups)))
	rootEntriesOff := buf.Len()
	for range groups {
		writeEntry(0, 0, true)
	}

	type dataPatch struct {
		res          resource
		dataEntryOff int
	}
	var patches []dataPatch

	for gi, g := range groups {
		typeDirOff := buf.Len()
		patchEntry(buf.Bytes(), rootEntriesOff+gi*8, g.typeID, uint32(typeDirOff), true)

		writeDir(0, uint16(len(g.res)))
		typeEntriesOff := buf.Len()
		for range g.res {
			writeEntry(0, 0, true)
		}

		for ri, r := range g.res {
			nameDirOff := buf.Len()
			patchEntry(buf.Bytes(), typeEntriesOff+ri*8, r.NameID, uint32(nameDirOff), true)

			writeDir(0, 1)
			writeEntry(r.LangID, uint32(0), false)
			langEntryOff := buf.Len() - 8

			dataEntryOff := alignOffset(buf.Len(), 4)
			padTo(&buf, dataEntryOff)
			patchEntry(buf.Bytes(), langEntryOff, r.LangID, uint32(dataEntryOff), false)

			writeU32(&buf, 0) // OffsetToData, patched after data layout
			writeU32(&buf, uint32(len(r.Data)))
			writeU32(&buf, r.Codepage)
			writeU32(&buf, 0)
			patches = append(patches, dataPatch{res: r, dataEntryOff: dataEntryOff})
		}
	}

	padTo(&buf, alignOffset(buf.Len(), 4))
	for _, p := range patches {
		dataOff := alignOffset(buf.Len(), 4)
		padTo(&buf, dataOff)
		data := buf.Bytes()
		binary.LittleEndian.PutUint32(data[p.dataEntryOff:], uint32(dataOff))
		relocs = append(relocs, sectionReloc{Offset: uint32(p.dataEntryOff)})
		buf.Write(p.res.Data)
	}

	_ = rootOff
	return buf.Bytes(), relocs
}

func patchEntry(data []byte, off int, id uint16, target uint32, isDir bool) {
	binary.LittleEndian.PutUint32(data[off:], uint32(id))
	if isDir {
		target |= 0x80000000
	}
	binary.LittleEndian.PutUint32(data[off+4:], target)
}

func buildCOFFObject(section []byte, relocs []sectionReloc) []byte {
	const fileHeaderSize = 20
	const sectionHeaderSize = 40
	rawPtr := fileHeaderSize + sectionHeaderSize
	relocPtr := rawPtr + len(section)
	symbolPtr := relocPtr + len(relocs)*10
	numberOfSymbols := uint32(2) // .rsrc symbol + one aux symbol

	var buf bytes.Buffer

	writeU16(&buf, coffMachineAMD64)
	writeU16(&buf, 1) // NumberOfSections
	writeU32(&buf, 0) // TimeDateStamp
	writeU32(&buf, uint32(symbolPtr))
	writeU32(&buf, numberOfSymbols)
	writeU16(&buf, 0) // SizeOfOptionalHeader
	writeU16(&buf, 0) // Characteristics

	writeName(&buf, ".rsrc")
	writeU32(&buf, uint32(len(section))) // VirtualSize / PhysicalAddress
	writeU32(&buf, 0)                    // VirtualAddress
	writeU32(&buf, uint32(len(section)))
	writeU32(&buf, uint32(rawPtr))
	writeU32(&buf, uint32(relocPtr))
	writeU32(&buf, 0) // PointerToLinenumbers
	writeU16(&buf, uint16(len(relocs)))
	writeU16(&buf, 0) // NumberOfLinenumbers
	writeU32(&buf, imageScnCntInitializedData|imageScnMemRead)

	buf.Write(section)

	for _, r := range relocs {
		writeU32(&buf, r.Offset)
		writeU32(&buf, 0) // symbol index for .rsrc
		writeU16(&buf, imageRelAMD64Addr32NB)
	}

	writeName(&buf, ".rsrc")
	writeU32(&buf, 0) // Value
	writeU16(&buf, 1) // SectionNumber
	writeU16(&buf, 0) // Type
	buf.WriteByte(imageSymClassStatic)
	buf.WriteByte(1) // aux symbols

	writeU32(&buf, uint32(len(section))) // Length
	writeU16(&buf, uint16(len(relocs)))
	writeU16(&buf, 0) // NumberOfLinenumbers
	writeU32(&buf, 0) // CheckSum
	writeU16(&buf, 1) // Number
	buf.WriteByte(0)  // Selection
	buf.Write(make([]byte, 3))

	writeU32(&buf, 4) // string table length

	return buf.Bytes()
}

func writeName(buf *bytes.Buffer, name string) {
	var raw [8]byte
	copy(raw[:], []byte(name))
	buf.Write(raw[:])
}

func writeU16(buf *bytes.Buffer, v uint16) {
	_ = binary.Write(buf, binary.LittleEndian, v)
}

func writeU32(buf *bytes.Buffer, v uint32) {
	_ = binary.Write(buf, binary.LittleEndian, v)
}

func alignOffset(n, align int) int {
	if n%align == 0 {
		return n
	}
	return n + align - n%align
}

func padTo(buf *bytes.Buffer, target int) {
	for buf.Len() < target {
		buf.WriteByte(0)
	}
}
