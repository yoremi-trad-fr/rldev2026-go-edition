// Package avg32 disassembles the AVG32/TPC32 bytecode used by early
// VisualArt's games such as Kanon.
package avg32

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/yoremi/rldev-go/pkg/disasm"
	"github.com/yoremi/rldev-go/pkg/texttransforms"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/transform"
)

const defaultSystemVersion = 0

type Options struct {
	OutDir          string
	SrcExt          string
	FuncReg         *disasm.FuncRegistry
	Annotate        bool
	SeparateStrings bool
	TextTransform   texttransforms.EncMode
	ForceTransform  bool
}

type Result struct {
	Header       Header
	Instructions []Instruction
	Labels       map[int]string
	Raw          []byte
}

type Header struct {
	LabelCount   int
	CounterStart uint32
	Labels       []uint32
	MenuCount    int
	CodeOffset   int
}

type Instruction struct {
	Offset  int
	Code    byte
	Subcode int
	Name    string
	Args    []string
	Targets []int
	Raw     []byte
}

type ResourceEntry struct {
	Key  string
	Text string
}

type parser struct {
	data       []byte
	pos        int
	codeStart  int
	funcReg    *disasm.FuncRegistry
	sysVersion int
	textMode   texttransforms.EncMode
}

func Disassemble(data []byte, opts Options) (*Result, error) {
	p := &parser{data: data, funcReg: opts.FuncReg, sysVersion: defaultSystemVersion, textMode: opts.TextTransform}
	header, err := p.parseHeader()
	if err != nil {
		return nil, err
	}

	var insts []Instruction
	for {
		if p.atEnd() {
			return nil, fmt.Errorf("missing TPC32 terminator")
		}
		if p.peek() == 0x00 {
			p.pos++
			break
		}
		inst, err := p.parseInstruction()
		if err != nil {
			return nil, err
		}
		insts = append(insts, inst)
	}

	labels := collectLabels(header, insts)
	return &Result{Header: header, Instructions: insts, Labels: labels, Raw: append([]byte(nil), data...)}, nil
}

func WriteSource(name string, result *Result, opts Options) error {
	outDir := opts.OutDir
	if outDir == "" {
		outDir = "."
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}
	ext := opts.SrcExt
	if ext == "" || ext == "org" {
		ext = "avg"
	}
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	outPath := filepath.Join(outDir, base+"."+ext)
	resName := ""
	if opts.SeparateStrings {
		resName = base + ".utf"
	}
	src, resources := renderSource(result, opts, resName)
	if err := os.WriteFile(outPath, []byte(src), 0644); err != nil {
		return err
	}
	if opts.SeparateStrings && len(resources) > 0 {
		return os.WriteFile(filepath.Join(outDir, resName), []byte(renderResourceFile(name, resources)), 0644)
	}
	return nil
}

func Render(result *Result, opts Options) string {
	src, _ := renderSource(result, opts, "")
	return src
}

func renderSource(result *Result, opts Options, resName string) (string, []ResourceEntry) {
	var b strings.Builder
	var resources *resourceCollector
	if opts.SeparateStrings && resName != "" {
		resources = &resourceCollector{}
	}
	body := renderBody(result, opts, resources)

	fmt.Fprintln(&b, "#target AVG32")
	fmt.Fprintf(&b, "#format TPC32 labels=%d menus=%d code_offset=0x%X\n\n",
		result.Header.LabelCount, result.Header.MenuCount, result.Header.CodeOffset)
	if resources != nil && len(resources.entries) > 0 {
		fmt.Fprintf(&b, "#resource '%s'\n\n", resName)
	}
	b.WriteString(body)
	return b.String(), resourceEntries(resources)
}

func renderBody(result *Result, opts Options, resources *resourceCollector) string {
	var b strings.Builder
	labelOffsets := make([]int, 0, len(result.Labels))
	for off := range result.Labels {
		labelOffsets = append(labelOffsets, off)
	}
	sort.Ints(labelOffsets)

	nextLabel := 0
	for _, inst := range result.Instructions {
		for nextLabel < len(labelOffsets) && labelOffsets[nextLabel] <= inst.Offset {
			off := labelOffsets[nextLabel]
			if off == inst.Offset {
				fmt.Fprintf(&b, "@%s:\n", result.Labels[off])
			}
			nextLabel++
		}
		if opts.Annotate {
			fmt.Fprintf(&b, "  // %04X: %s\n", inst.Offset, hexBytes(inst.Raw))
		}
		fmt.Fprintf(&b, "  %s\n", renderInstruction(inst, resources))
	}
	fmt.Fprintln(&b)
	writeRawHexBlock(&b, result.Raw)
	return b.String()
}

func renderInstruction(inst Instruction, resources *resourceCollector) string {
	args := renderInstructionArgs(inst, resources)
	if len(args) == 0 {
		return inst.Name
	}
	return fmt.Sprintf("%s(%s)", inst.Name, strings.Join(args, ", "))
}

func renderInstructionArgs(inst Instruction, resources *resourceCollector) []string {
	args := append([]string(nil), inst.Args...)
	if resources == nil {
		return args
	}
	if (inst.Code == 0xfe || inst.Code == 0xff) && len(args) == 1 {
		if text, ok := unquoteSourceString(args[0]); ok {
			args[0] = resources.add(text)
		}
		return args
	}
	if inst.Code == 0x60 && inst.Subcode == 0x04 && len(args) == 1 {
		args[0] = replaceFormattedBlockStringsWithResources(args[0], resources)
		return args
	}
	if inst.Code == 0x58 && (inst.Subcode == 0x01 || inst.Subcode == 0x02) {
		for i, arg := range args {
			if strings.HasPrefix(strings.TrimSpace(arg), "[") {
				args[i] = replaceFormattedBlockStringsWithResources(arg, resources)
			}
		}
	}
	return args
}

func replaceFormattedBlockStringsWithResources(block string, resources *resourceCollector) string {
	var out strings.Builder
	for i := 0; i < len(block); {
		if block[i] != '"' {
			out.WriteByte(block[i])
			i++
			continue
		}
		lit, consumed, err := readQuotedLiteral(block[i:])
		if err != nil {
			out.WriteString(block[i:])
			break
		}
		if text, err := strconv.Unquote(lit); err == nil {
			out.WriteString(resources.add(text))
		} else {
			out.WriteString(lit)
		}
		i += consumed
	}
	return out.String()
}

func unquoteSourceString(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, `"`) {
		return "", false
	}
	text, err := strconv.Unquote(s)
	return text, err == nil
}

type resourceCollector struct {
	entries []ResourceEntry
}

func (r *resourceCollector) add(text string) string {
	key := fmt.Sprintf("%04d", len(r.entries))
	r.entries = append(r.entries, ResourceEntry{Key: key, Text: text})
	return fmt.Sprintf("#res<%s>", key)
}

func resourceEntries(r *resourceCollector) []ResourceEntry {
	if r == nil {
		return nil
	}
	return append([]ResourceEntry(nil), r.entries...)
}

func renderResourceFile(name string, entries []ResourceEntry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "// Resources for %s\r\n\r\n", filepath.Base(name))
	for _, entry := range entries {
		fmt.Fprintf(&b, "<%s> %s\r\n", entry.Key, escapeResourceLineText(entry.Text))
	}
	return b.String()
}

func escapeResourceLineText(s string) string {
	if s == "" {
		return s
	}
	if strings.HasPrefix(s, "//") {
		return `\` + s
	}
	switch []rune(s)[0] {
	case ' ', '\t', '<':
		return `\` + s
	}
	return s
}

func writeRawHexBlock(b *strings.Builder, data []byte) {
	if len(data) == 0 {
		return
	}
	fmt.Fprintln(b, "#rawhex begin")
	for off := 0; off < len(data); off += 16 {
		end := off + 16
		if end > len(data) {
			end = len(data)
		}
		fmt.Fprintf(b, "#rawhex %s\n", hexBytes(data[off:end]))
	}
	fmt.Fprintln(b, "#rawhex end")
	fmt.Fprintln(b)
}

func collectLabels(header Header, insts []Instruction) map[int]string {
	labels := map[int]string{0: "start"}
	for i, off := range header.Labels {
		labels[int(off)] = fmt.Sprintf("entry_%02d", i)
	}
	for _, inst := range insts {
		for _, target := range inst.Targets {
			if _, ok := labels[target]; !ok {
				labels[target] = fmt.Sprintf("label_%04X", target)
			}
		}
	}
	return labels
}

func (p *parser) parseHeader() (Header, error) {
	if len(p.data) < 5 || string(p.data[:5]) != "TPC32" {
		return Header{}, fmt.Errorf("not an AVG32 TPC32 scene")
	}
	p.pos = 5
	if err := p.skip(0x13); err != nil {
		return Header{}, err
	}
	labelCount, err := p.readU32()
	if err != nil {
		return Header{}, err
	}
	counterStart, err := p.readU32()
	if err != nil {
		return Header{}, err
	}
	labels := make([]uint32, int(labelCount))
	for i := range labels {
		labels[i], err = p.readU32()
		if err != nil {
			return Header{}, err
		}
	}
	if err := p.skip(0x30); err != nil {
		return Header{}, err
	}
	menuCount, err := p.readU32()
	if err != nil {
		return Header{}, err
	}
	menuStringCount := 0
	for i := 0; i < int(menuCount); i++ {
		if err := p.parseMenu(&menuStringCount); err != nil {
			return Header{}, err
		}
	}
	for i := 0; i < menuStringCount; i++ {
		if _, err := p.readCString(); err != nil {
			return Header{}, err
		}
	}
	if err := p.skip(0x05); err != nil {
		return Header{}, err
	}
	p.codeStart = p.pos
	return Header{
		LabelCount:   int(labelCount),
		CounterStart: counterStart,
		Labels:       labels,
		MenuCount:    int(menuCount),
		CodeOffset:   p.codeStart,
	}, nil
}

func (p *parser) parseMenu(stringCount *int) error {
	if err := p.skip(1); err != nil {
		return err
	}
	submenus, err := p.readByte()
	if err != nil {
		return err
	}
	if err := p.skip(2); err != nil {
		return err
	}
	*stringCount = *stringCount + 1
	for i := 0; i < int(submenus); i++ {
		if err := p.skip(1); err != nil {
			return err
		}
		flagCount, err := p.readByte()
		if err != nil {
			return err
		}
		if err := p.skip(2); err != nil {
			return err
		}
		*stringCount = *stringCount + 1
		for j := 0; j < int(flagCount); j++ {
			count, err := p.readByte()
			if err != nil {
				return err
			}
			if err := p.skip(1 + int(count)*4); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *parser) parseInstruction() (Instruction, error) {
	start := p.pos
	rel := start - p.codeStart
	code, err := p.readByte()
	if err != nil {
		return Instruction{}, err
	}
	inst := Instruction{Offset: rel, Code: code, Subcode: -1}
	name := p.lookup(code, -1, fmt.Sprintf("op_%02X", code))

	switch code {
	case 0x01, 0x02, 0x03, 0x05, 0x06, 0x08, 0x18, 0x1a, 0x21, 0x22, 0x23, 0x24, 0x25,
		0x26, 0x27, 0x28, 0x29, 0x2c, 0x2d, 0x30, 0x5b, 0x5e, 0x63,
		0x65, 0x66, 0x69, 0x6f, 0x7f:
		// no operands
	case 0x0c:
		name, inst.Subcode, inst.Args, err = p.parseOp0C(code)
	case 0x04:
		name, inst.Subcode, inst.Args, err = p.parseTextWin(code)
	case 0x0b:
		name, inst.Subcode, inst.Args, err = p.parseGraphics(code)
	case 0x0e:
		name, inst.Subcode, inst.Args, err = p.parseSound(code)
	case 0x10:
		name, inst.Subcode, inst.Args, err = p.parseFormattedTextCmd(code)
	case 0x13:
		name, inst.Subcode, inst.Args, err = p.parseFade(code)
	case 0x15:
		var cond string
		cond, err = p.parseConditions()
		if err == nil {
			var target int
			target, err = p.readPos()
			inst.Targets = append(inst.Targets, target)
			inst.Args = []string{cond, fmt.Sprintf("@%04X", target)}
		}
	case 0x16:
		name, inst.Subcode, inst.Args, err = p.parseJumpToScene(code)
	case 0x17:
		name, inst.Subcode, inst.Args, err = p.parseOneValSub(code)
	case 0x19:
		name, inst.Subcode, inst.Args, err = p.parseWait(code)
	case 0x1b, 0x1c:
		var target int
		target, err = p.readPos()
		inst.Targets = append(inst.Targets, target)
		inst.Args = []string{fmt.Sprintf("@%04X", target)}
	case 0x1d, 0x1e:
		var count byte
		count, err = p.readByte()
		if err == nil {
			var val string
			val, err = p.readVal()
			inst.Args = append(inst.Args, val)
		}
		for i := 0; err == nil && i < int(count); i++ {
			var target int
			target, err = p.readPos()
			inst.Targets = append(inst.Targets, target)
			inst.Args = append(inst.Args, fmt.Sprintf("@%04X", target))
		}
	case 0x20:
		name, inst.Subcode, inst.Args, err = p.parseNoArgSub(code)
	case 0x2e, 0x2f, 0x31:
		name, inst.Subcode, inst.Args, err = p.parseScenarioOrTextRank(code)
	case 0x37, 0x39, 0x3b, 0x3c, 0x3d, 0x3e, 0x3f, 0x40, 0x41, 0x42, 0x43,
		0x49, 0x4a, 0x4b, 0x4c, 0x4d, 0x4e, 0x4f, 0x50, 0x51:
		inst.Args, err = p.readVals(2)
	case 0x56, 0xea:
		inst.Args, err = p.readVals(1)
	case 0x57:
		inst.Args, err = p.readVals(3)
	case 0x58:
		name, inst.Subcode, inst.Args, err = p.parseChoice(code)
	case 0x59:
		name, inst.Subcode, inst.Args, err = p.parseString(code)
	case 0x5c:
		name, inst.Subcode, inst.Args, err = p.parseSetMulti(code)
	case 0x5d:
		name, inst.Subcode, inst.Args, err = p.parseOp5D(code)
	case 0x5f:
		name, inst.Subcode, inst.Args, err = p.parseOp5F(code)
	case 0x60:
		name, inst.Subcode, inst.Args, err = p.parseSystem(code)
	case 0x61:
		name, inst.Subcode, inst.Args, err = p.parseName(code)
	case 0x64:
		name, inst.Subcode, inst.Args, err = p.parseBufferRegion(code)
	case 0x67:
		name, inst.Subcode, inst.Args, err = p.parseBuffer(code)
	case 0x68:
		name, inst.Subcode, inst.Args, err = p.parseFlash(code)
	case 0x6a:
		name, inst.Subcode, inst.Args, err = p.parseMultiPdt(code)
	case 0x6c:
		name, inst.Subcode, inst.Args, err = p.parseAreaBuffer(code)
	case 0x6d:
		name, inst.Subcode, inst.Args, err = p.parseMouse(code)
	case 0x6e:
		name, inst.Subcode, inst.Args, err = p.parseOp6E(code)
	case 0x70:
		name, inst.Subcode, inst.Args, err = p.parseWindowVar(code)
	case 0x72:
		name, inst.Subcode, inst.Args, err = p.parseMessageWin(code)
	case 0x73:
		name, inst.Subcode, inst.Args, err = p.parseSystemVar(code)
	case 0x74:
		name, inst.Subcode, inst.Args, err = p.parsePopup(code)
	case 0x75:
		name, inst.Subcode, inst.Args, err = p.parseVolume(code)
	case 0x76:
		name, inst.Subcode, inst.Args, err = p.parseNovel(code)
	case 0xfe, 0xff:
		if p.sysVersion >= 1714 {
			var idx uint32
			idx, err = p.readU32()
			inst.Args = append(inst.Args, fmt.Sprintf("id:%d", idx))
		}
		if err == nil {
			var text string
			text, err = p.readSceneText()
			inst.Args = append(inst.Args, text)
		}
	default:
		err = fmt.Errorf("unknown AVG32 opcode 0x%02X at 0x%04X", code, rel)
	}
	if err != nil {
		return Instruction{}, fmt.Errorf("%w at 0x%04X (opcode 0x%02X)", err, rel, code)
	}
	inst.Name = name
	inst.Raw = append([]byte(nil), p.data[start:p.pos]...)
	return inst, nil
}

func (p *parser) parseTextWin(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	return p.lookup(code, int(sub), textWinNames[sub]), int(sub), nil, nil
}

func (p *parser) parseNoArgSub(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	return p.lookup(code, int(sub), fmt.Sprintf("op_%02X_%02X", code, sub)), int(sub), nil, nil
}

func (p *parser) parseOneValSub(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	args, err := p.readVals(1)
	return p.lookup(code, int(sub), fmt.Sprintf("op_%02X_%02X", code, sub)), int(sub), args, err
}

func (p *parser) parseOp0C(code byte) (string, int, []string, error) {
	if p.pos+1 < len(p.data) && p.peek() == 0x10 && isLikelyTextStart(p.data[p.pos+1]) {
		sub, err := p.readByte()
		if err != nil {
			return "", 0, nil, err
		}
		text, err := p.readSceneText()
		if err != nil {
			return "", 0, nil, err
		}
		vals, err := p.readVals(1)
		return p.lookup(code, int(sub), "op_0c_10"), int(sub), append([]string{text}, vals...), err
	}
	return p.lookup(code, -1, "op_0C"), -1, nil, nil
}

func (p *parser) parseOp6E(code byte) (string, int, []string, error) {
	if p.pos+1 < len(p.data) && p.peek() == 0x03 && isValueMarker(p.data[p.pos+1]) {
		sub, err := p.readByte()
		if err != nil {
			return "", 0, nil, err
		}
		args, err := p.readVals(1)
		return p.lookup(code, int(sub), "op_6e_03"), int(sub), args, err
	}
	return p.lookup(code, -1, "op_6E"), -1, nil, nil
}

func (p *parser) parseJumpToScene(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	args, err := p.readVals(1)
	return p.lookup(code, int(sub), jumpSceneNames[sub]), int(sub), args, err
}

func (p *parser) parseFormattedTextCmd(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	var args []string
	switch sub {
	case 0x01, 0x03, 0x11:
		args, err = p.readVals(1)
	case 0x02:
		args, err = p.readVals(2)
	case 0x13:
	default:
		err = fmt.Errorf("unknown formatted text subcommand 0x%02X", sub)
	}
	return p.lookup(code, int(sub), formattedTextNames[sub]), int(sub), args, err
}

func (p *parser) parseFade(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	count := map[byte]int{0x01: 1, 0x02: 2, 0x03: 3, 0x04: 4, 0x10: 1, 0x11: 3}[sub]
	if count == 0 {
		return "", 0, nil, fmt.Errorf("unknown fade subcommand 0x%02X", sub)
	}
	args, err := p.readVals(count)
	return p.lookup(code, int(sub), fadeNames[sub]), int(sub), args, err
}

func (p *parser) parseWait(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	count := map[byte]int{0x01: 1, 0x02: 2, 0x03: 0, 0x04: 1, 0x05: 1, 0x06: 1, 0x10: 0, 0x11: 0, 0x12: 0, 0x13: 0}[sub]
	if _, ok := waitNames[sub]; !ok {
		return "", 0, nil, fmt.Errorf("unknown wait subcommand 0x%02X", sub)
	}
	args, err := p.readVals(count)
	return p.lookup(code, int(sub), waitNames[sub]), int(sub), args, err
}

func (p *parser) parseScenarioOrTextRank(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	count := 0
	switch code {
	case 0x2e, 0x2f:
		if sub == 0x01 {
			count = 1
		} else if sub == 0x02 {
			count = 2
		} else if code == 0x2f && sub == 0x03 {
			count = 0
		} else if code == 0x2f && sub == 0x1d {
			return p.lookup(code, int(sub), scenarioExtNames[sub]), int(sub), nil, nil
		} else if code == 0x2f && sub == 0x3d {
			args, err := p.readVals(2)
			return p.lookup(code, int(sub), scenarioExtNames[sub]), int(sub), args, err
		} else {
			return "", 0, nil, fmt.Errorf("unknown scenario menu subcommand 0x%02X", sub)
		}
	case 0x31:
		if sub == 0x01 {
			count = 1
		} else if sub != 0x02 {
			return "", 0, nil, fmt.Errorf("unknown text rank subcommand 0x%02X", sub)
		}
	}
	args, err := p.readVals(count)
	fallback := fmt.Sprintf("op_%02X_%02X", code, sub)
	if code == 0x2f {
		if name := scenarioExtNames[sub]; name != "" {
			fallback = name
		}
	}
	return p.lookup(code, int(sub), fallback), int(sub), args, err
}

func (p *parser) parseGraphics(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	var args []string
	switch sub {
	case 0x01, 0x03, 0x05, 0x09, 0x10, 0x54:
		var text string
		text, err = p.readSceneText()
		if err == nil {
			var vals []string
			vals, err = p.readVals(1)
			args = append([]string{text}, vals...)
		}
	case 0x02, 0x04, 0x06:
		args, err = p.parseGrpEffect()
	case 0x08, 0x13, 0x30, 0x50:
	case 0x11:
		var text string
		text, err = p.readSceneText()
		args = []string{text}
	case 0x22:
		args, err = p.parseGrpComposite(false)
	case 0x24:
		args, err = p.parseGrpComposite(true)
	case 0x31, 0x32, 0x33, 0x52:
		args, err = p.readVals(1)
	default:
		err = fmt.Errorf("unknown graphics subcommand 0x%02X", sub)
	}
	return p.lookup(code, int(sub), graphicsNames[sub]), int(sub), args, err
}

func (p *parser) parseGrpEffect() ([]string, error) {
	text, err := p.readSceneText()
	if err != nil {
		return nil, err
	}
	vals, err := p.readVals(15)
	return append([]string{text}, vals...), err
}

func (p *parser) parseGrpComposite(indexed bool) ([]string, error) {
	count, err := p.readByte()
	if err != nil {
		return nil, err
	}
	var args []string
	args = append(args, fmt.Sprintf("count:%d", count))
	if indexed {
		v, err := p.readVal()
		if err != nil {
			return nil, err
		}
		args = append(args, v)
	} else {
		t, err := p.readSceneText()
		if err != nil {
			return nil, err
		}
		args = append(args, t)
	}
	idx, err := p.readVal()
	if err != nil {
		return nil, err
	}
	args = append(args, idx)
	for i := 0; i < int(count); i++ {
		child, err := p.parseGrpCompositeChild()
		if err != nil {
			return nil, err
		}
		args = append(args, child)
	}
	return args, nil
}

func (p *parser) parseGrpCompositeChild() (string, error) {
	method, err := p.readByte()
	if err != nil {
		return "", err
	}
	text, err := p.readSceneText()
	if err != nil {
		return "", err
	}
	count := map[byte]int{0x01: 0, 0x02: 1, 0x03: 6, 0x04: 7}[method]
	if _, ok := map[byte]bool{0x01: true, 0x02: true, 0x03: true, 0x04: true}[method]; !ok {
		return "", fmt.Errorf("unknown composite method 0x%02X", method)
	}
	vals, err := p.readVals(count)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("{method:0x%02X, file:%s, args:[%s]}", method, text, strings.Join(vals, ", ")), nil
}

func (p *parser) parseSound(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	var args []string
	switch sub {
	case 0x01, 0x02, 0x03, 0x30, 0x32, 0x34:
		var text string
		text, err = p.readSceneText()
		args = []string{text}
	case 0x05, 0x06, 0x07, 0x31, 0x33, 0x35, 0x37, 0x39, 0x40:
		if sub == 0x05 || sub == 0x06 || sub == 0x07 || sub == 0x31 || sub == 0x33 || sub == 0x35 {
			var text string
			text, err = p.readSceneText()
			if err == nil {
				var vals []string
				vals, err = p.readVals(1)
				args = append([]string{text}, vals...)
			}
		} else {
			args, err = p.readVals(1)
		}
	case 0x10, 0x20, 0x21:
		args, err = p.readVals(1)
	case 0x11, 0x12, 0x16, 0x36, 0x38, 0x60:
	case 0x22:
		args, err = p.readVals(2)
	case 0x50, 0x51, 0x52, 0x53:
		var text string
		text, err = p.readSceneText()
		if err == nil {
			var vals []string
			vals, err = p.readVals(4)
			args = append([]string{text}, vals...)
		}
	case 0x54, 0x55:
		var a, b string
		a, err = p.readSceneText()
		if err == nil {
			b, err = p.readSceneText()
		}
		if err == nil {
			var vals []string
			vals, err = p.readVals(4)
			args = append([]string{a, b}, vals...)
		}
	default:
		err = fmt.Errorf("unknown sound subcommand 0x%02X", sub)
	}
	return p.lookup(code, int(sub), soundNames[sub]), int(sub), args, err
}

func (p *parser) parseChoice(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	switch sub {
	case 0x01, 0x02:
		idx, err := p.readVal()
		if err != nil {
			return "", 0, nil, err
		}
		flag, err := p.readByte()
		if err != nil {
			return "", 0, nil, err
		}
		args := []string{idx, fmt.Sprintf("flag:0x%02X", flag)}
		if flag == 0x22 {
			pad, err := p.readByte()
			if err != nil {
				return "", 0, nil, err
			}
			args = append(args, fmt.Sprintf("pad:0x%02X", pad))
			for !p.atEnd() && p.peek() != 0x23 {
				text, err := p.parseFormattedText()
				if err != nil {
					return "", 0, nil, err
				}
				args = append(args, text)
			}
			if err := p.expect(0x23); err != nil {
				return "", 0, nil, err
			}
		}
		return p.lookup(code, int(sub), choiceNames[sub]), int(sub), args, nil
	case 0x04:
		args, err := p.readVals(1)
		return p.lookup(code, int(sub), choiceNames[sub]), int(sub), args, err
	default:
		return "", 0, nil, fmt.Errorf("unknown choice subcommand 0x%02X", sub)
	}
}

func (p *parser) parseString(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	var args []string
	switch sub {
	case 0x01:
		dst, err := p.readVal()
		if err == nil {
			var text string
			text, err = p.readSceneText()
			args = []string{dst, text}
		}
	case 0x02, 0x04, 0x05, 0x08:
		args, err = p.readVals(2)
	case 0x03, 0x06:
		args, err = p.readVals(3)
	case 0x07:
		args, err = p.readVals(1)
	default:
		err = fmt.Errorf("unknown string subcommand 0x%02X", sub)
	}
	return p.lookup(code, int(sub), stringNames[sub]), int(sub), args, err
}

func (p *parser) parseSetMulti(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	if sub != 0x01 && sub != 0x02 {
		return "", 0, nil, fmt.Errorf("unknown set_multi subcommand 0x%02X", sub)
	}
	args, err := p.readVals(3)
	return p.lookup(code, int(sub), setMultiNames[sub]), int(sub), args, err
}

func (p *parser) parseOp5D(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	if sub != 0x01 {
		return "", 0, nil, fmt.Errorf("unknown op_5d subcommand 0x%02X", sub)
	}
	args, err := p.readVals(3)
	return p.lookup(code, int(sub), op5DNames[sub]), int(sub), args, err
}

func (p *parser) parseOp5F(code byte) (string, int, []string, error) {
	if !p.atEnd() && p.peek() == 0x01 {
		sub, err := p.readByte()
		if err != nil {
			return "", 0, nil, err
		}
		var vals []string
		for !p.atEnd() && p.peek() != 0x00 {
			v, err := p.readVal()
			if err != nil {
				return "", 0, nil, err
			}
			vals = append(vals, v)
		}
		if err := p.expect(0x00); err != nil {
			return "", 0, nil, err
		}
		return p.lookup(code, int(sub), op5FNames[sub]), int(sub), []string{"[" + strings.Join(vals, ", ") + "]"}, nil
	}
	args, err := p.readVals(8)
	return p.lookup(code, -1, "op_5f"), -1, args, err
}

func (p *parser) parseSystem(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	var args []string
	switch sub {
	case 0x02, 0x03:
		args, err = p.readVals(1)
	case 0x04:
		var text string
		text, err = p.parseFormattedText()
		args = []string{text}
	case 0x05, 0x20:
	case 0x30, 0x31, 0x35, 0x36, 0x37:
		args, err = p.readVals(2)
	default:
		err = fmt.Errorf("unknown system subcommand 0x%02X", sub)
	}
	return p.lookup(code, int(sub), systemNames[sub]), int(sub), args, err
}

func (p *parser) parseName(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	var args []string
	switch sub {
	case 0x01:
		args, err = p.readVals(10)
	case 0x02, 0x03, 0x04, 0x20:
		args, err = p.readVals(1)
	case 0x10, 0x11, 0x12:
		args, err = p.readVals(2)
	case 0x21:
		args, err = p.readVals(1)
		if err == nil {
			var text string
			text, err = p.readSceneText()
			args = append(args, text)
		}
		if err == nil {
			var vals []string
			vals, err = p.readVals(9)
			args = append(args, vals...)
		}
	case 0x24:
		var count byte
		count, err = p.readByte()
		args = []string{fmt.Sprintf("count:%d", count)}
		for i := 0; err == nil && i < int(count); i++ {
			var v, text string
			v, err = p.readVal()
			if err == nil {
				text, err = p.parseFormattedText()
			}
			args = append(args, fmt.Sprintf("{%s, %s}", v, text))
		}
	case 0x30, 0x31:
	default:
		err = fmt.Errorf("unknown name subcommand 0x%02X", sub)
	}
	return p.lookup(code, int(sub), nameNames[sub]), int(sub), args, err
}

func (p *parser) parseBufferRegion(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	count := map[byte]int{0x02: 8, 0x04: 8, 0x07: 5, 0x10: 8, 0x11: 5, 0x12: 5, 0x15: 9, 0x20: 5, 0x30: 10, 0x32: 16}[sub]
	if count == 0 {
		return "", 0, nil, fmt.Errorf("unknown buffer_region subcommand 0x%02X", sub)
	}
	args, err := p.readVals(count)
	return p.lookup(code, int(sub), bufferRegionNames[sub]), int(sub), args, err
}

func (p *parser) parseBuffer(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	count := map[byte]int{
		0x00: 6, 0x01: 8, 0x02: 8, 0x03: 11, 0x05: 8, 0x08: 9, 0x11: 2, 0x12: 2,
		0x20: 15, 0x21: 16, 0x22: 18,
	}[sub]
	if _, ok := bufferNames[sub]; !ok {
		return "", 0, nil, fmt.Errorf("unknown buffer subcommand 0x%02X", sub)
	}
	args, err := p.readVals(count)
	if err == nil && (sub == 0x01 || sub == 0x02 || sub == 0x11 || sub == 0x12) && p.shouldReadOptionalVal() {
		var flag string
		flag, err = p.readVal()
		args = append(args, flag)
	}
	return p.lookup(code, int(sub), bufferNames[sub]), int(sub), args, err
}

func (p *parser) parseFlash(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	count := map[byte]int{0x01: 4, 0x10: 5}[sub]
	if count == 0 {
		return "", 0, nil, fmt.Errorf("unknown flash subcommand 0x%02X", sub)
	}
	args, err := p.readVals(count)
	return p.lookup(code, int(sub), flashNames[sub]), int(sub), args, err
}

func (p *parser) parseMultiPdt(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	var args []string
	switch sub {
	case 0x03, 0x04:
		var count byte
		count, err = p.readByte()
		args = append(args, fmt.Sprintf("count:%d", count))
		if err == nil {
			var vals []string
			vals, err = p.readVals(2)
			args = append(args, vals...)
		}
		for i := 0; err == nil && i < int(count); i++ {
			var entry string
			entry, err = p.parseMultiPdtEntry()
			args = append(args, entry)
		}
	case 0x05:
	case 0x10, 0x20:
		args, err = p.parseMultiPdtScroll(false)
	case 0x30:
		args, err = p.parseMultiPdtScroll(true)
	default:
		err = fmt.Errorf("unknown multi_pdt subcommand 0x%02X", sub)
	}
	return p.lookup(code, int(sub), multiPdtNames[sub]), int(sub), args, err
}

func (p *parser) parseMultiPdtScroll(withCancel bool) ([]string, error) {
	poscmd, err := p.readByte()
	if err != nil {
		return nil, err
	}
	count, err := p.readByte()
	if err != nil {
		return nil, err
	}
	args := []string{fmt.Sprintf("poscmd:0x%02X", poscmd), fmt.Sprintf("count:%d", count)}
	valCount := 3
	if withCancel {
		valCount = 4
	}
	vals, err := p.readVals(valCount)
	args = append(args, vals...)
	for i := 0; err == nil && i < int(count); i++ {
		var entry string
		entry, err = p.parseMultiPdtEntry()
		args = append(args, entry)
	}
	return args, err
}

func (p *parser) parseMultiPdtEntry() (string, error) {
	text, err := p.readSceneText()
	if err != nil {
		return "", err
	}
	val, err := p.readVal()
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("{%s, %s}", text, val), nil
}

func (p *parser) parseAreaBuffer(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	var args []string
	switch sub {
	case 0x02:
		a, err := p.readSceneText()
		if err == nil {
			var b string
			b, err = p.readSceneText()
			args = []string{a, b}
		}
	case 0x03:
	case 0x04, 0x05, 0x20:
		args, err = p.readVals(2)
	case 0x10, 0x11:
		args, err = p.readVals(1)
	case 0x15:
		args, err = p.readVals(3)
	default:
		err = fmt.Errorf("unknown area_buffer subcommand 0x%02X", sub)
	}
	return p.lookup(code, int(sub), areaNames[sub]), int(sub), args, err
}

func (p *parser) parseMouse(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	var args []string
	switch sub {
	case 0x01, 0x03, 0x20, 0x21:
	case 0x02:
		args, err = p.readVals(3)
	default:
		err = fmt.Errorf("unknown mouse subcommand 0x%02X", sub)
	}
	return p.lookup(code, int(sub), mouseNames[sub]), int(sub), args, err
}

func (p *parser) parseWindowVar(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	count := map[byte]int{0x01: 4, 0x02: 4, 0x03: 1, 0x04: 1, 0x05: 1, 0x06: 1, 0x10: 1, 0x11: 1}[sub]
	if count == 0 {
		return "", 0, nil, fmt.Errorf("unknown window_var subcommand 0x%02X", sub)
	}
	args, err := p.readVals(count)
	return p.lookup(code, int(sub), windowVarNames[sub]), int(sub), args, err
}

func (p *parser) parseMessageWin(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	if _, ok := messageWinNames[sub]; !ok {
		return "", 0, nil, fmt.Errorf("unknown message_win subcommand 0x%02X", sub)
	}
	args, err := p.readVals(2)
	return p.lookup(code, int(sub), messageWinNames[sub]), int(sub), args, err
}

func (p *parser) parseSystemVar(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	count := 1
	if sub == 0x01 || sub == 0x02 || sub == 0x05 || sub == 0x06 || sub == 0x31 {
		count = 2
	}
	if _, ok := systemVarNames[sub]; !ok {
		return "", 0, nil, fmt.Errorf("unknown system_var subcommand 0x%02X", sub)
	}
	args, err := p.readVals(count)
	return p.lookup(code, int(sub), systemVarNames[sub]), int(sub), args, err
}

func (p *parser) parsePopup(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	count := map[byte]int{0x01: 1, 0x02: 1, 0x03: 2, 0x04: 2}[sub]
	if count == 0 {
		return "", 0, nil, fmt.Errorf("unknown popup subcommand 0x%02X", sub)
	}
	args, err := p.readVals(count)
	return p.lookup(code, int(sub), popupNames[sub]), int(sub), args, err
}

func (p *parser) parseVolume(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	if _, ok := volumeNames[sub]; !ok {
		return "", 0, nil, fmt.Errorf("unknown volume subcommand 0x%02X", sub)
	}
	args, err := p.readVals(1)
	return p.lookup(code, int(sub), volumeNames[sub]), int(sub), args, err
}

func (p *parser) parseNovel(code byte) (string, int, []string, error) {
	sub, err := p.readByte()
	if err != nil {
		return "", 0, nil, err
	}
	if sub == 0x10 {
		var vals []string
		for !p.atEnd() && p.peek() != 0x00 {
			v, err := p.readVal()
			if err != nil {
				return "", 0, nil, err
			}
			vals = append(vals, v)
		}
		if err := p.expect(0x00); err != nil {
			return "", 0, nil, err
		}
		return p.lookup(code, int(sub), novelNames[sub]), int(sub), []string{"[" + strings.Join(vals, ", ") + "]"}, nil
	}
	count := map[byte]int{0x01: 1, 0x02: 1, 0x03: 0, 0x04: 0, 0x05: 0}[sub]
	if _, ok := novelNames[sub]; !ok {
		return "", 0, nil, fmt.Errorf("unknown novel subcommand 0x%02X", sub)
	}
	args, err := p.readVals(count)
	return p.lookup(code, int(sub), novelNames[sub]), int(sub), args, err
}

func (p *parser) parseFormattedText() (string, error) {
	var parts []string
	for {
		if p.atEnd() {
			return "", fmt.Errorf("unterminated formatted text")
		}
		if p.peek() == 0x00 {
			p.pos++
			break
		}
		tag, err := p.readByte()
		if err != nil {
			return "", err
		}
		switch tag {
		case 0x10:
			_, _, args, err := p.parseFormattedTextCmd(0x10)
			if err != nil {
				return "", err
			}
			parts = append(parts, "cmd("+strings.Join(args, ", ")+")")
		case 0x12:
			parts = append(parts, "unknown")
		case 0x28:
			cond, err := p.parseConditions()
			if err != nil {
				return "", err
			}
			parts = append(parts, "if("+cond+")")
		case 0xfd:
			v, err := p.readVal()
			if err != nil {
				return "", err
			}
			parts = append(parts, "ptr("+v+")")
		case 0xfe, 0xff:
			s, err := p.readCString()
			if err != nil {
				return "", err
			}
			parts = append(parts, quoteString(s))
		default:
			return "", fmt.Errorf("unknown formatted text tag 0x%02X", tag)
		}
	}
	return "[" + strings.Join(parts, " ") + "]", nil
}

func (p *parser) parseConditions() (string, error) {
	depth := 0
	var parts []string
	for {
		op, err := p.readByte()
		if err != nil {
			return "", err
		}
		switch op {
		case 0x26:
			parts = append(parts, "and")
		case 0x27:
			parts = append(parts, "or")
		case 0x28:
			depth++
			parts = append(parts, "(")
		case 0x29:
			depth--
			parts = append(parts, ")")
			if depth <= 0 {
				return strings.Join(parts, " "), nil
			}
		case 0x36, 0x37, 0x38, 0x39, 0x3a, 0x3b, 0x41, 0x42, 0x43, 0x44, 0x45,
			0x46, 0x47, 0x48, 0x49, 0x4f, 0x50, 0x51, 0x52, 0x53, 0x54, 0x55:
			a, err := p.readVal()
			if err != nil {
				return "", err
			}
			b, err := p.readVal()
			if err != nil {
				return "", err
			}
			parts = append(parts, fmt.Sprintf("%s(%s,%s)", conditionNames[op], a, b))
		case 0x58:
			attr, err := p.readByte()
			if err != nil {
				return "", err
			}
			switch attr {
			case 0x20, 0x22:
				v, err := p.readVal()
				if err != nil {
					return "", err
				}
				parts = append(parts, fmt.Sprintf("ret_%02X(%s)", attr, v))
			case 0x21:
				parts = append(parts, "ret_choice")
			default:
				return "", fmt.Errorf("unknown return condition 0x%02X", attr)
			}
		default:
			return "", fmt.Errorf("unknown condition opcode 0x%02X", op)
		}
	}
}

func (p *parser) readVals(n int) ([]string, error) {
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		v, err := p.readVal()
		if err != nil {
			return out, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (p *parser) readVal() (string, error) {
	if p.atEnd() {
		return "", fmt.Errorf("unexpected end while reading value")
	}
	num := p.data[p.pos]
	length := int((num >> 4) & 7)
	if length == 0 {
		return "", fmt.Errorf("invalid zero-length value marker 0x%02X", num)
	}
	if p.pos+length > len(p.data) {
		return "", fmt.Errorf("truncated value")
	}
	isVar := num&0x80 == 0x80
	var ret uint32
	for i := length - 2; i >= 0; i-- {
		ret <<= 8
		ret |= uint32(p.data[p.pos+1+i])
	}
	ret <<= 4
	ret |= uint32(num & 0x0f)
	p.pos += length
	if isVar {
		return fmt.Sprintf("$%d", ret), nil
	}
	return strconv.FormatUint(uint64(ret), 10), nil
}

func isValueMarker(num byte) bool {
	return ((num >> 4) & 7) != 0
}

func isLikelyTextStart(num byte) bool {
	return num == 0x40 || num >= 0x20
}

func (p *parser) shouldReadOptionalVal() bool {
	if p.atEnd() || !isValueMarker(p.peek()) {
		return false
	}
	return !p.looksLikeOpcodeAt(p.pos)
}

func (p *parser) looksLikeOpcodeAt(pos int) bool {
	if pos >= len(p.data) {
		return false
	}
	code := p.data[pos]
	switch code {
	case 0x01, 0x02, 0x03, 0x05, 0x06, 0x08, 0x18, 0x1a, 0x21, 0x22, 0x23, 0x24, 0x25,
		0x26, 0x27, 0x28, 0x29, 0x2c, 0x2d, 0x30, 0x5b, 0x5e, 0x63, 0x65, 0x66, 0x69, 0x6f, 0x7f:
		return true
	case 0x0b:
		return pos+1 < len(p.data) && graphicsNames[p.data[pos+1]] != ""
	case 0x0e:
		return pos+1 < len(p.data) && soundNames[p.data[pos+1]] != ""
	case 0x10:
		return pos+1 < len(p.data) && formattedTextNames[p.data[pos+1]] != ""
	case 0x13:
		return pos+1 < len(p.data) && fadeNames[p.data[pos+1]] != ""
	case 0x15, 0x1b, 0x1c, 0x1d, 0x1e, 0x37, 0x39, 0x3b, 0x3c, 0x3d, 0x3e, 0x3f, 0x40, 0x41, 0x42,
		0x43, 0x49, 0x4a, 0x4b, 0x4c, 0x4d, 0x4e, 0x4f, 0x50, 0x51, 0x56, 0x57, 0x58, 0x59,
		0x5c, 0x5d, 0x5f, 0x64, 0x67, 0x68, 0x6a, 0x6c, 0x70, 0x72, 0x73, 0x74, 0x75, 0x76,
		0xea, 0xfe, 0xff:
		return true
	case 0x04:
		return pos+1 < len(p.data) && textWinNames[p.data[pos+1]] != ""
	case 0x16:
		return pos+1 < len(p.data) && jumpSceneNames[p.data[pos+1]] != ""
	case 0x19:
		return pos+1 < len(p.data) && waitNames[p.data[pos+1]] != ""
	case 0x2e, 0x2f:
		return pos+1 < len(p.data) && (p.data[pos+1] == 0x01 || p.data[pos+1] == 0x02 || scenarioExtNames[p.data[pos+1]] != "")
	case 0x31:
		return pos+1 < len(p.data) && (p.data[pos+1] == 0x01 || p.data[pos+1] == 0x02)
	case 0x60:
		return pos+1 < len(p.data) && systemNames[p.data[pos+1]] != ""
	case 0x61:
		return pos+1 < len(p.data) && nameNames[p.data[pos+1]] != ""
	case 0x6d:
		return pos+1 < len(p.data) && mouseNames[p.data[pos+1]] != ""
	}
	return false
}

func (p *parser) readSceneText() (string, error) {
	if p.atEnd() {
		return "", fmt.Errorf("unexpected end while reading text")
	}
	if p.peek() == 0x40 {
		p.pos++
		v, err := p.readVal()
		if err != nil {
			return "", err
		}
		return "ptr(" + v + ")", nil
	}
	s, err := p.readCString()
	if err != nil {
		return "", err
	}
	return quoteString(s), nil
}

func (p *parser) readCString() (string, error) {
	end := bytes.IndexByte(p.data[p.pos:], 0)
	if end < 0 {
		return "", fmt.Errorf("unterminated string")
	}
	raw := p.data[p.pos : p.pos+end]
	p.pos += end + 1
	return decodeAVG32Text(raw, p.textMode)
}

func decodeAVG32Text(raw []byte, mode texttransforms.EncMode) (string, error) {
	if mode == texttransforms.EncNone {
		decoded, _, err := transform.Bytes(japanese.ShiftJIS.NewDecoder(), raw)
		if err != nil {
			return string(raw), nil
		}
		return string(decoded), nil
	}
	previous := texttransforms.GetMode()
	texttransforms.SetMode(mode)
	defer texttransforms.SetMode(previous)
	decoded, err := texttransforms.ReadBytecode(raw)
	if err != nil {
		return string(raw), nil
	}
	return string(decoded), nil
}

func (p *parser) readPos() (int, error) {
	v, err := p.readU32()
	return int(v), err
}

func (p *parser) readByte() (byte, error) {
	if p.pos >= len(p.data) {
		return 0, fmt.Errorf("unexpected end of file")
	}
	v := p.data[p.pos]
	p.pos++
	return v, nil
}

func (p *parser) readU32() (uint32, error) {
	if p.pos+4 > len(p.data) {
		return 0, fmt.Errorf("unexpected end of file")
	}
	v := binary.LittleEndian.Uint32(p.data[p.pos:])
	p.pos += 4
	return v, nil
}

func (p *parser) skip(n int) error {
	if p.pos+n > len(p.data) {
		return fmt.Errorf("unexpected end of file")
	}
	p.pos += n
	return nil
}

func (p *parser) expect(v byte) error {
	got, err := p.readByte()
	if err != nil {
		return err
	}
	if got != v {
		return fmt.Errorf("expected 0x%02X, got 0x%02X", v, got)
	}
	return nil
}

func (p *parser) peek() byte {
	return p.data[p.pos]
}

func (p *parser) atEnd() bool {
	return p.pos >= len(p.data)
}

func (p *parser) lookup(code byte, sub int, fallback string) string {
	if fallback == "" {
		fallback = fmt.Sprintf("op_%02X", code)
		if sub >= 0 {
			fallback = fmt.Sprintf("op_%02X_%02X", code, sub)
		}
	}
	if p.funcReg == nil {
		return fallback
	}
	overload := 0
	if sub >= 0 {
		overload = sub
	}
	op := disasm.Opcode{Type: 0, Module: 0, Function: int(code), Overload: overload}
	if def, ok := p.funcReg.LookupOpcode(op); ok && def.Name != "" {
		return def.Name
	}
	return fallback
}

func quoteString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func hexBytes(data []byte) string {
	parts := make([]string, len(data))
	for i, b := range data {
		parts[i] = fmt.Sprintf("%02X", b)
	}
	return strings.Join(parts, " ")
}
