// Package memory implements the symbol table, scoping, and register
// allocation for the Kepago compiler.
//
// Transposed from OCaml:
//   - rlc/memory.ml (346 lines)    — symbol table, scoping, static allocation
//   - rlc/variables.ml (319 lines) — variable declarations, address computation
//
// The RealLive engine uses a fixed set of register banks (intA–intZ, strK/M/S)
// with integer indices. Variables declared in Kepago source are mapped to
// slots within these banks. The memory system handles:
//
//   - Scoped symbol definitions (#define, #const, #bind, int/str decls)
//   - Nested scope open/close with automatic deallocation
//   - Static register allocation (first-fit within a bank range)
//   - Block allocation (contiguous slots for arrays)
//   - Sub-int types: bit, bit2, bit4, byte packed into int32 slots
//   - Temporary variable allocation for compiler-generated code
package memory

import (
	"fmt"

	"github.com/yoremi/rldev-go/rlc/pkg/ast"
)

// ============================================================
// Symbol types
// ============================================================

// SymbolKind distinguishes what a symbol represents.
type SymbolKind int

const (
	KindMacro     SymbolKind = iota // expression macro (#define X = expr)
	KindInline                       // inline block (#inline f(args) body)
	KindInteger                      // integer constant (#const X = 42)
	KindString                       // string constant (#const S = "...")
	KindStaticVar                    // allocated register variable (int/str decl)
)

// Symbol is one entry in the symbol table.
type Symbol struct {
	Kind   SymbolKind
	Scoped bool // true if defined in current scope (vs global #define)

	// For KindInteger
	IntVal int32

	// For KindString
	StrVal string

	// For KindMacro
	Expr ast.Expr

	// For KindInline
	InlineParams []ast.InlineParam
	InlineBody   ast.Stmt

	// For KindStaticVar
	Var *StaticVar
}

// StaticVar describes an allocated register slot.
type StaticVar struct {
	TypedSpace int   // register bank index for access (may differ from alloc space for sub-int)
	Index      int32 // access index within the typed space
	ArrayLen   int   // 0 for scalar, >0 for array
	AllocSpace int   // allocation space index (into staticVars array)
	AllocIndex int   // allocation index within alloc space
	AllocLen   int   // number of int32 slots allocated
	IsStr      bool  // true for string variables
}

// ============================================================
// Memory (symbol table with scoping)
// ============================================================

// Memory is the compiler symbol table with nested scoping and register allocation.
type Memory struct {
	symbols map[string][]symbolEntry // name → stack of definitions (innermost first)
	scopes  []map[string]bool        // stack of scopes: each tracks names defined in that scope
	alloc   *Allocator

	// Configuration: allocation ranges for temp variables
	IntAllocSpace int // default int bank (e.g., 0x0b = intL)
	IntAllocFirst int // first index
	IntAllocLast  int // last index (exclusive)
	StrAllocSpace int // default str bank (e.g., 0x12 = strS)
	StrAllocFirst int
	StrAllocLast  int
}

type symbolEntry struct {
	sym    Symbol
	scoped bool
}

// New creates a fresh Memory with default allocation settings.
func New() *Memory {
	m := &Memory{
		symbols: make(map[string][]symbolEntry),
		alloc:   newAllocator(),
		// Defaults matching RLdev's __int_alloc_*__ / __str_alloc_*__
		IntAllocSpace: 0x0b, // intL
		IntAllocFirst: 0,
		IntAllocLast:  2000,
		StrAllocSpace: 0x0c, // strM
		StrAllocFirst: 0,
		StrAllocLast:  2000,
	}
	m.OpenScope() // top-level scope
	return m
}

// ============================================================
// Scope management
// ============================================================

// OpenScope pushes a new scope level.
func (m *Memory) OpenScope() {
	m.scopes = append(m.scopes, make(map[string]bool))
}

// CloseScope pops the current scope and deallocates its symbols.
func (m *Memory) CloseScope() {
	if len(m.scopes) <= 1 {
		return // never close the top-level scope
	}
	top := m.scopes[len(m.scopes)-1]
	m.scopes = m.scopes[:len(m.scopes)-1]

	for name := range top {
		entries := m.symbols[name]
		if len(entries) == 0 {
			continue
		}
		// Find and remove the innermost scoped entry
		for i := 0; i < len(entries); i++ {
			if entries[i].scoped {
				// Deallocate if it's a static var
				if entries[i].sym.Kind == KindStaticVar && entries[i].sym.Var != nil {
					m.alloc.Free(entries[i].sym.Var)
				}
				entries = append(entries[:i], entries[i+1:]...)
				break
			}
		}
		if len(entries) == 0 {
			delete(m.symbols, name)
		} else {
			m.symbols[name] = entries
		}
	}
}

// ScopeDepth returns the current nesting depth.
func (m *Memory) ScopeDepth() int { return len(m.scopes) }

// ============================================================
// Symbol definition and lookup
// ============================================================

// Define adds a symbol to the current scope.
func (m *Memory) Define(name string, sym Symbol) {
	sym.Scoped = true
	m.symbols[name] = append([]symbolEntry{{sym: sym, scoped: true}}, m.symbols[name]...)
	if len(m.scopes) > 0 {
		m.scopes[len(m.scopes)-1][name] = true
	}
}

// DefineGlobal adds a symbol to the global (unscoped) level.
func (m *Memory) DefineGlobal(name string, sym Symbol) {
	sym.Scoped = false
	m.symbols[name] = append([]symbolEntry{{sym: sym, scoped: false}}, m.symbols[name]...)
}

// Defined returns true if a symbol exists.
func (m *Memory) Defined(name string) bool {
	entries := m.symbols[name]
	return len(entries) > 0
}

// Get retrieves the most recent definition of a symbol.
func (m *Memory) Get(name string) (Symbol, bool) {
	entries := m.symbols[name]
	if len(entries) == 0 {
		return Symbol{}, false
	}
	return entries[0].sym, true
}

// Undefine removes a symbol from the current scope.
func (m *Memory) Undefine(name string) error {
	entries := m.symbols[name]
	if len(entries) == 0 {
		return fmt.Errorf("undefined symbol: %s", name)
	}
	// Remove the first (innermost) entry
	entry := entries[0]
	if entry.sym.Kind == KindStaticVar && entry.sym.Var != nil {
		m.alloc.Free(entry.sym.Var)
	}
	m.symbols[name] = entries[1:]
	if len(m.symbols[name]) == 0 {
		delete(m.symbols, name)
	}
	// Remove from scope tracking
	for i := len(m.scopes) - 1; i >= 0; i-- {
		if m.scopes[i][name] {
			delete(m.scopes[i], name)
			break
		}
	}
	return nil
}

// Mutate replaces the value of an existing symbol in-place.
func (m *Memory) Mutate(name string, sym Symbol) error {
	entries := m.symbols[name]
	if len(entries) == 0 {
		return fmt.Errorf("undefined symbol: %s", name)
	}
	entries[0].sym = sym
	return nil
}

// Describe returns a human-readable description of a symbol.
func (m *Memory) Describe(name string) string {
	sym, ok := m.Get(name)
	if !ok {
		return "undeclared identifier"
	}
	switch sym.Kind {
	case KindMacro:
		return "macro"
	case KindInline:
		return "inline block"
	case KindInteger:
		return "integer constant"
	case KindString:
		return "string constant"
	case KindStaticVar:
		if sym.Var == nil {
			return "variable"
		}
		isStr := sym.Var.IsStr
		isArray := sym.Var.ArrayLen > 0
		switch {
		case isStr && isArray:
			return "string array"
		case isStr:
			return "string variable"
		case isArray:
			return "integer array"
		default:
			return "integer variable"
		}
	}
	return "unknown"
}

// ============================================================
// Expression resolution (get_as_expression)
// ============================================================

// GetAsExpr returns the expression representation of a symbol.
// For constants, returns IntLit/StrLit.
// For static vars, returns IVar/SVar with the allocated index.
// For macros, returns the macro expression.
func (m *Memory) GetAsExpr(name string, loc ast.Loc) (ast.Expr, error) {
	sym, ok := m.Get(name)
	if !ok {
		return nil, fmt.Errorf("undefined symbol: %s", name)
	}
	switch sym.Kind {
	case KindInteger:
		return ast.IntLit{Loc: loc, Val: sym.IntVal}, nil
	case KindString:
		return ast.StrLit{Loc: loc, Tokens: []ast.StrToken{
			ast.TextToken{Loc: loc, Text: sym.StrVal},
		}}, nil
	case KindStaticVar:
		if sym.Var == nil {
			return nil, fmt.Errorf("variable %s has no allocation", name)
		}
		if sym.Var.IsStr {
			return ast.StrVar{Loc: loc, Bank: sym.Var.TypedSpace, Index: ast.IntLit{Loc: loc, Val: sym.Var.Index}}, nil
		}
		return ast.IntVar{Loc: loc, Bank: sym.Var.TypedSpace, Index: ast.IntLit{Loc: loc, Val: sym.Var.Index}}, nil
	case KindMacro:
		if sym.Expr != nil {
			return sym.Expr, nil
		}
		return ast.IntLit{Loc: loc, Val: 0}, nil
	case KindInline:
		return nil, fmt.Errorf("cannot use inline block %s as expression", name)
	}
	return ast.IntLit{Loc: loc, Val: 0}, nil
}

// GetDerefAsExpr returns an array dereference expression: name[offset].
func (m *Memory) GetDerefAsExpr(name string, offset ast.Expr, loc ast.Loc) (ast.Expr, error) {
	sym, ok := m.Get(name)
	if !ok {
		return nil, fmt.Errorf("undefined symbol: %s", name)
	}
	if sym.Kind != KindStaticVar || sym.Var == nil {
		return nil, fmt.Errorf("expected array, found %s for %s", m.Describe(name), name)
	}
	v := sym.Var
	idx := ast.BinOp{Loc: loc, LHS: ast.IntLit{Loc: loc, Val: v.Index}, Op: ast.OpAdd, RHS: offset}
	if v.IsStr {
		return ast.StrVar{Loc: loc, Bank: v.TypedSpace, Index: idx}, nil
	}
	return ast.IntVar{Loc: loc, Bank: v.TypedSpace, Index: idx}, nil
}

// ============================================================
// Register allocation
// ============================================================

// Allocator manages static register slot allocation.
type Allocator struct {
	// staticVars[space][index] = reference count
	// space is mapped via varIdx from the register bank number
	staticVars [12]map[int]int
}

func newAllocator() *Allocator {
	a := &Allocator{}
	for i := range a.staticVars {
		a.staticVars[i] = make(map[int]int)
	}
	return a
}

// varIdx maps a register bank number to a staticVars array index.
// Banks 0-6 (intA-intG) → 0-6, bank 25 (intZ) → 7,
// bank 18 (strS) → 8, bank 12 (strM) → 9, banks 10-11 (strK, intL) → 10-11
func varIdx(bank int) (int, error) {
	if bank >= 0 && bank < 7 {
		return bank, nil
	}
	switch bank {
	case 25:
		return 7, nil
	case 18:
		return 8, nil
	case 12:
		return 9, nil
	case 10, 11:
		return bank, nil
	}
	return -1, fmt.Errorf("cannot allocate variables in bank 0x%02x", bank)
}

// FindUnusedIndex finds the first unused slot in a space.
func (a *Allocator) FindUnusedIndex(space, first, last int) (int, error) {
	m := a.staticVars[space]
	for i := first; i < last; i++ {
		if m[i] <= 0 {
			return i, nil
		}
	}
	return -1, fmt.Errorf("failed to allocate static memory in space %d", space)
}

// FindUnusedBlock finds a contiguous block of unused slots.
func (a *Allocator) FindUnusedBlock(space, first, length int) (int, error) {
	m := a.staticVars[space]
	maxIdx := 2000 - length
	for i := first; i <= maxIdx; i++ {
		found := true
		for j := i; j < i+length; j++ {
			if m[j] > 0 {
				found = false
				break
			}
		}
		if found {
			return i, nil
		}
	}
	return -1, fmt.Errorf("failed to allocate block of %d in space %d", length, space)
}

// AllocateBlock marks a contiguous block of slots as used.
func (a *Allocator) AllocateBlock(bank, index, length int) error {
	space, err := varIdx(bank)
	if err != nil {
		return err
	}
	for i := index; i < index+length; i++ {
		a.staticVars[space][i]++
	}
	return nil
}

// Free deallocates a static variable's slots.
func (a *Allocator) Free(v *StaticVar) {
	m := a.staticVars[v.AllocSpace]
	for i := v.AllocIndex; i < v.AllocIndex+v.AllocLen; i++ {
		m[i]--
		if m[i] <= 0 {
			delete(m, i)
		}
	}
}

// ============================================================
// Variable declaration helpers (from variables.ml)
// ============================================================

// VarType describes a declared variable type.
type VarType struct {
	IsStr    bool
	BitWidth int // 1, 2, 4, 8, 32 (only for int)
}

// DeclDir is a declaration directive.
type DeclDir int

const (
	DirZero  DeclDir = iota // zero-initialize
	DirBlock                // block allocation
	DirExt                  // external (not scoped)
	DirLabel                // write to flag.ini
)

// AllocVar allocates a variable in a register bank and defines it.
// Returns the StaticVar descriptor.
func (m *Memory) AllocVar(name string, vt VarType, arrayLen int, fixedAddr *[2]int) (*StaticVar, error) {
	isStr := vt.IsStr
	blockSize := arrayLen
	if blockSize == 0 {
		blockSize = 1
	}
	if !isStr && vt.BitWidth < 32 {
		// Sub-int types: multiple elements pack into fewer int32 slots
		blockSize = (arrayLen*vt.BitWidth-1)/32 + 1
		if blockSize < 1 {
			blockSize = 1
		}
	}

	var space, index int
	if fixedAddr != nil {
		space = fixedAddr[0]
		index = fixedAddr[1]
	} else {
		if isStr {
			space = m.StrAllocSpace
		} else {
			space = m.IntAllocSpace
		}
		spaceIdx, err := varIdx(space)
		if err != nil {
			return nil, err
		}
		var max int
		if isStr {
			max = m.StrAllocLast
		} else {
			max = m.IntAllocLast
		}
		idx, err := m.alloc.FindUnusedBlock(spaceIdx, 0, blockSize)
		if err != nil {
			return nil, err
		}
		if idx+blockSize > max {
			return nil, fmt.Errorf("unable to allocate block for %s", name)
		}
		index = idx
	}

	// Compute typed space and access address for sub-int types
	typedSpace, accessIndex := getRealAddress(vt, space, index)

	spaceIdx, err := varIdx(space)
	if err != nil {
		return nil, err
	}

	// Mark slots as used
	if err := m.alloc.AllocateBlock(space, index, blockSize); err != nil {
		return nil, err
	}

	sv := &StaticVar{
		TypedSpace: typedSpace,
		Index:      int32(accessIndex),
		ArrayLen:   arrayLen,
		AllocSpace: spaceIdx,
		AllocIndex: index,
		AllocLen:   blockSize,
		IsStr:      isStr,
	}

	m.Define(name, Symbol{
		Kind:   KindStaticVar,
		Var:    sv,
		Scoped: true,
	})

	return sv, nil
}

// getRealAddress computes the typed space and access index for a variable.
// Sub-int types (bit, bit2, bit4, byte) use offset banks and multiplied indices.
func getRealAddress(vt VarType, space, allocIndex int) (typedSpace, accessIndex int) {
	if vt.IsStr || vt.BitWidth >= 32 {
		return space, allocIndex
	}
	switch vt.BitWidth {
	case 1:
		return space + 26, allocIndex * 32
	case 2:
		return space + 52, allocIndex * 16
	case 4:
		return space + 78, allocIndex * 8
	case 8:
		return space + 104, allocIndex * 4
	}
	return space, allocIndex
}

// AllocTempInt allocates a temporary integer variable and returns its expression.
func (m *Memory) AllocTempInt() (ast.Expr, error) {
	space := m.IntAllocSpace
	spaceIdx, err := varIdx(space)
	if err != nil {
		return nil, err
	}
	idx, err := m.alloc.FindUnusedIndex(spaceIdx, m.IntAllocFirst, m.IntAllocLast)
	if err != nil {
		return nil, err
	}
	m.alloc.staticVars[spaceIdx][idx] = 1

	name := fmt.Sprintf("[temp %d.%d]", space, idx)
	sv := &StaticVar{
		TypedSpace: space,
		Index:      int32(idx),
		AllocSpace: spaceIdx,
		AllocIndex: idx,
		AllocLen:   1,
		IsStr:      false,
	}
	m.Define(name, Symbol{Kind: KindStaticVar, Var: sv})

	return ast.IntVar{Loc: ast.Nowhere, Bank: space, Index: ast.IntLit{Loc: ast.Nowhere, Val: int32(idx)}}, nil
}

// AllocTempStr allocates a temporary string variable and returns its expression.
func (m *Memory) AllocTempStr() (ast.Expr, error) {
	space := m.StrAllocSpace
	spaceIdx, err := varIdx(space)
	if err != nil {
		return nil, err
	}
	idx, err := m.alloc.FindUnusedIndex(spaceIdx, m.StrAllocFirst, m.StrAllocLast)
	if err != nil {
		return nil, err
	}
	m.alloc.staticVars[spaceIdx][idx] = 1

	name := fmt.Sprintf("[temp %d.%d]", space, idx)
	sv := &StaticVar{
		TypedSpace: space,
		Index:      int32(idx),
		AllocSpace: spaceIdx,
		AllocIndex: idx,
		AllocLen:   1,
		IsStr:      true,
	}
	m.Define(name, Symbol{Kind: KindStaticVar, Var: sv})

	return ast.StrVar{Loc: ast.Nowhere, Bank: space, Index: ast.IntLit{Loc: ast.Nowhere, Val: int32(idx)}}, nil
}

// GetOrAllocTemp gets a named temp variable, allocating it if it doesn't exist.
func (m *Memory) GetOrAllocTemp(name string, isStr bool) (ast.Expr, error) {
	if expr, err := m.GetAsExpr(name, ast.Nowhere); err == nil {
		return expr, nil
	}
	var expr ast.Expr
	var err error
	if isStr {
		expr, err = m.AllocTempStr()
	} else {
		expr, err = m.AllocTempInt()
	}
	if err != nil {
		return nil, err
	}
	m.Define(name, Symbol{Kind: KindMacro, Expr: expr})
	return expr, nil
}
