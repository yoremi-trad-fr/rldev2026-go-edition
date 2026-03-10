// Package disasm implements the RealLive bytecode disassembler.
// Transposed from OCaml's kprl/disassembler.ml (~5400 lines).
//
// The disassembler reads compiled RealLive bytecode and produces
// human-readable kepago source code, with separate resource strings
// for translation workflows.
package disasm

import (
	"fmt"
	"strings"
)

// --- Target engine modes ---

// EngineMode represents the target engine type.
type EngineMode int

const (
	ModeNone     EngineMode = iota
	ModeRealLive            // Standard RealLive
	ModeAvg2000             // AVG2000 (older)
	ModeKinetic             // Kinetic (subset)
)

func (m EngineMode) String() string {
	switch m {
	case ModeRealLive:
		return "RealLive"
	case ModeAvg2000:
		return "AVG2000"
	case ModeKinetic:
		return "Kinetic"
	default:
		return "[unknown]"
	}
}

// Version is a 4-part version number.
type Version [4]int

func (v Version) String() string {
	if v[2] == 0 && v[3] == 0 {
		return fmt.Sprintf("%d.%d", v[0], v[1])
	}
	if v[3] == 0 {
		return fmt.Sprintf("%d.%d.%d", v[0], v[1], v[2])
	}
	return fmt.Sprintf("%d.%d.%d.%d", v[0], v[1], v[2], v[3])
}

// --- Opcode representation ---

// Opcode identifies a specific RealLive bytecode instruction.
type Opcode struct {
	Type     int // op_type (0 or 1)
	Module   int // op_module (0-255)
	Function int // op_function (0-65535)
	Overload int // overload index (0-255)
}

// Special well-known opcode patterns.
var (
	Strcpy = OpcodePattern{Module: 10, Function: 0}
	Strcat = OpcodePattern{Module: 10, Function: 1}
	Ruby   = OpcodePattern{Module: 3, Function: 120}
	Select = OpcodePattern{Module: 2}
)

// OpcodePattern is used for matching opcodes by module/function.
type OpcodePattern struct {
	Module   int
	Function int
}

// Matches checks if an opcode matches this pattern.
func (p OpcodePattern) Matches(op Opcode) bool {
	if p.Module != op.Module {
		return false
	}
	// Function 0 in pattern means match any function in that module
	// (used for Select which matches all of module 2)
	if p == Select {
		return op.Module == 2
	}
	return p.Function == op.Function
}

func (op Opcode) String() string {
	return fmt.Sprintf("%d:%03d:%05d,%d", op.Type, op.Module, op.Function, op.Overload)
}

// --- Command (disassembled instruction) ---

// CommandElem is an element in a command's kepago representation.
type CommandElem interface {
	isCommandElem()
}

// ElemString is a string element in a command.
type ElemString struct{ Value string }

func (ElemString) isCommandElem() {}

// ElemPointer is a pointer/label reference in a command.
type ElemPointer struct{ Offset int }

func (ElemPointer) isCommandElem() {}

// ElemStore is a store-string element.
type ElemStore struct{ Value string }

func (ElemStore) isCommandElem() {}

// Command represents one disassembled instruction.
type Command struct {
	Offset  int           // Byte offset from start of code section
	Kepago  []CommandElem // Instruction representation
	Hidden  bool          // Hidden from output (debug lines, etc.)
	Unhide  bool          // Force-unhide (entrypoints)
	IsJmp   bool          // Is a jump target (affects suppression)
	CType   string        // Command type annotation
	Opcode  string        // Opcode string for annotation
	LineNo  int           // Debug line number
	ResIdx  int           // Resource string index (-1 if none)
}

// Text returns the text representation of the command's kepago elements.
func (c *Command) Text() string {
	var sb strings.Builder
	for _, e := range c.Kepago {
		switch v := e.(type) {
		case ElemString:
			sb.WriteString(v.Value)
		case ElemStore:
			sb.WriteString(v.Value)
		case ElemPointer:
			sb.WriteString(fmt.Sprintf("@ptr_%d", v.Offset))
		}
	}
	return sb.String()
}

// --- Disassembler options ---

// Options controls the disassembler behavior.
type Options struct {
	SeparateStrings  bool   // Write strings to separate .res file
	SeparateAll      bool   // Separate all strings (not just textout)
	IDStrings        bool   // Add IDs to resource strings
	ReadDebugSymbols bool   // Include #line directives
	Annotate         bool   // Add offset annotations
	ControlCodes     bool   // Process control codes in text
	SuppressUncalled bool   // Hide code after unconditional jumps
	ForcedTarget     EngineMode
	UsesExclKidoku   bool
	StartAddress     int    // -1 = auto
	EndAddress       int    // -1 = auto
	ShowOpcodes      bool   // Show opcode annotations
	HexDump          bool   // Generate hex dump
	RawStrings       bool   // Don't process text encoding
	MakeMap          bool   // Generate seen map
	SrcExt           string // Source file extension (default "org")
	Encoding         string // Output encoding (default "CP932")
	BOM              bool   // Write UTF-8 BOM
	Verbose          int
}

// DefaultOptions returns the default disassembler options.
func DefaultOptions() Options {
	return Options{
		SeparateStrings:  true,
		ControlCodes:     true,
		StartAddress:     -1,
		EndAddress:       -1,
		SrcExt:           "org",
		Encoding:         "CP932",
	}
}

// --- Map types for seen-map generation ---

// Location represents a source location.
type Location struct {
	Seen int // SEEN file index
	Line int // Line number within the file
}

// Address represents a target address (for jumps/calls).
type Address struct {
	Scene int // Target SEEN index
	Entry int // Entry point index (-1 = none)
}

// Jump represents a cross-scene jump or call.
type Jump struct {
	Origin Location
	Target Address
	Kind   string // "goto", "gosub", "call", etc.
}

// SeenMap holds navigation information for one SEEN file.
type SeenMap struct {
	EntryPoints []int              // Entry point line numbers
	Calls       []Jump             // Outgoing calls
	Gotos       []Jump             // Outgoing gotos
	Entries     map[int]Jump       // Incoming calls/gotos (keyed by entry index)
}

// NewSeenMap creates an empty SeenMap.
func NewSeenMap() *SeenMap {
	return &SeenMap{
		Entries: make(map[int]Jump),
	}
}

// --- KFN function definition types ---

// ParamType describes a function parameter type.
type ParamType int

const (
	ParamAny    ParamType = iota
	ParamInt              // Integer expression
	ParamIntC             // Integer constant
	ParamIntV             // Integer variable
	ParamStr              // String expression
	ParamStrC             // String constant
	ParamStrV             // String variable
	ParamResStr           // Resource string
)

// FuncFlag describes properties of a function.
type FuncFlag int

const (
	FlagPushStore FuncFlag = iota
	FlagIsJump
	FlagIsGoto
	FlagIsCond
	FlagIsNeg
	FlagIsTextout
	FlagNoBraces
	FlagIsLbr
	FlagHasGotos
	FlagHasCases
	FlagIsCall
	FlagIsSkip
	FlagIsRet
)

// FuncDef describes a known RealLive function.
type FuncDef struct {
	Name       string
	Flags      []FuncFlag
	Prototypes [][]ParamType // One or more parameter lists
}

// FuncRegistry holds known function definitions, loaded from KFN files.
type FuncRegistry struct {
	funcs   map[string]FuncDef
	modules map[int]string
}

// NewFuncRegistry creates an empty function registry.
func NewFuncRegistry() *FuncRegistry {
	return &FuncRegistry{
		funcs:   make(map[string]FuncDef),
		modules: make(map[int]string),
	}
}

// Register adds a function definition.
func (r *FuncRegistry) Register(opStr string, def FuncDef) {
	r.funcs[opStr] = def
}

// Lookup finds a function definition by opcode string.
func (r *FuncRegistry) Lookup(opStr string) (FuncDef, bool) {
	d, ok := r.funcs[opStr]
	return d, ok
}

// RegisterModule sets the name for a module number.
func (r *FuncRegistry) RegisterModule(num int, name string) {
	r.modules[num] = name
}

// ModuleName returns the name for a module, or its number as string.
func (r *FuncRegistry) ModuleName(num int) string {
	if name, ok := r.modules[num]; ok {
		return name
	}
	return fmt.Sprintf("%03d", num)
}

// HasFlag checks if a function definition has a specific flag.
func (d FuncDef) HasFlag(f FuncFlag) bool {
	for _, flag := range d.Flags {
		if flag == f {
			return true
		}
	}
	return false
}
