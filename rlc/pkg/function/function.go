// Package function implements function call assembly and type checking
// for the Kepago compiler.
//
// Transposed from OCaml:
//   - rlc/funcAsm.ml (210 lines)  — parameter serialization, overload by count, assembly
//   - rlc/function.ml (726 lines) — type checking, overload by params, high-level compilation
//
// The function compilation pipeline:
//   1. Look up the function definition in the KFN registry (LookupFuncDef)
//   2. Select the correct overload (ChooseOverloadByCount / ChooseOverloadByParams)
//   3. Type-check each parameter against the prototype (CheckParamType)
//   4. Serialize parameters into bytecode (SerializeParams)
//   5. Build the opcode header + serialized params (Assemble)
package function

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/yoremi/rldev-go/rlc/pkg/ast"
	"github.com/yoremi/rldev-go/rlc/pkg/codegen"
	"github.com/yoremi/rldev-go/rlc/pkg/kfn"
)

// ============================================================
// Assembled parameter types (from funcAsm.ml)
// ============================================================

// AsmParamKind identifies the kind of assembled parameter.
type AsmParamKind int

const (
	AsmString  AsmParamKind = iota // raw bytecode (variable refs, etc.)
	AsmInteger                      // integer expression bytecode
	AsmUnknown                      // untyped bytecode
	AsmList                         // tuple: (param, param, ...)
	AsmSpecial                      // special tagged: a<id>(params)
	AsmLiteral                      // quoted string literal
)

// AsmParam is one assembled parameter ready for bytecode emission.
type AsmParam struct {
	Kind     AsmParamKind
	Code     string     // bytecode for String/Integer/Unknown/Literal
	Items    []AsmParam // for List and Special
	SpecID   int        // for Special: tag byte(s)
	NoParens bool       // for Special: emit without wrapping parens
}

// ============================================================
// Parameter serialization (from funcAsm.ml parameter_to_string)
// ============================================================

// SerializeParams converts assembled parameters to a bytecode string.
func SerializeParams(params []AsmParam) string {
	var b strings.Builder
	var prev *AsmParam
	for i := range params {
		serializeOne(&b, prev, &params[i])
		prev = &params[i]
	}
	return b.String()
}

func serializeOne(b *strings.Builder, prev, p *AsmParam) {
	switch p.Kind {
	case AsmString, AsmUnknown:
		b.WriteString(p.Code)

	case AsmList:
		b.WriteByte('(')
		var ip *AsmParam
		for i := range p.Items {
			serializeOne(b, ip, &p.Items[i])
			ip = &p.Items[i]
		}
		b.WriteByte(')')

	case AsmSpecial:
		// Emit special tag byte(s) — from funcAsm.ml lines 54-72
		if p.SpecID > 255 {
			// Two-byte special ID: a<b0> a<b1>
			b0 := byte(p.SpecID & 0xff)
			b1 := byte(((p.SpecID >> 8) & 0xff) - 1)
			b.WriteByte('a')
			b.WriteByte(b0)
			b.WriteByte('a')
			b.WriteByte(b1)
		} else {
			b.WriteByte('a')
			b.WriteByte(byte(p.SpecID))
		}
		// NoParens → emit children directly; otherwise wrap in List
		if p.NoParens {
			var ip *AsmParam
			for i := range p.Items {
				serializeOne(b, ip, &p.Items[i])
				ip = &p.Items[i]
			}
		} else {
			lp := AsmParam{Kind: AsmList, Items: p.Items}
			serializeOne(b, nil, &lp)
		}

	case AsmInteger:
		// Precede unary operators with commas if necessary (funcAsm.ml lines 74-81)
		if len(p.Code) > 0 && p.Code[0] == '\\' && prev != nil {
			needComma := true
			switch prev.Kind {
			case AsmList, AsmLiteral:
				needComma = false
			case AsmSpecial:
				if !prev.NoParens {
					needComma = false
				}
			}
			if needComma {
				b.WriteByte(',')
			}
		}
		b.WriteString(p.Code)

	case AsmLiteral:
		// Precede literals with commas if necessary (funcAsm.ml lines 83-89)
		if b.Len() > 0 && prev != nil {
			switch prev.Kind {
			case AsmLiteral:
				b.WriteByte(',')
			case AsmSpecial:
				if prev.NoParens {
					b.WriteByte(',')
				}
			}
		}
		if p.Code == "" {
			b.WriteString("\"\"")
		} else {
			b.WriteString(p.Code)
		}
	}
}

// ============================================================
// Prototype length computation (from funcAsm.ml get_prototype_lengths)
// ============================================================

// ProtoLen holds min/max parameter counts for one prototype.
// Min == -1 means undefined or arbitrary length.
type ProtoLen struct {
	Min int
	Max int
}

// GetPrototypeLengths computes the min/max param count for each overload.
// Filters out Return params, handles Optional and Argc flags.
func GetPrototypeLengths(fd *kfn.FuncDef) []ProtoLen {
	result := make([]ProtoLen, len(fd.Prototypes))
	for i, proto := range fd.Prototypes {
		if !proto.Defined {
			result[i] = ProtoLen{-1, -1}
			continue
		}
		min, max := 0, 0
		arb := false
		for _, p := range proto.Params {
			if arb {
				break
			}
			hasArgc, isOpt := false, false
			for _, f := range p.Flags {
				if f == kfn.FArgc {
					hasArgc = true
				}
				if f == kfn.FOptional {
					isOpt = true
				}
			}
			if hasArgc {
				arb = true
				break
			}
			max++
			if !isOpt {
				min++
			}
		}
		if arb {
			result[i] = ProtoLen{-1, -1}
		} else {
			result[i] = ProtoLen{min, max}
		}
	}
	return result
}

// ============================================================
// Overload selection by parameter count (from funcAsm.ml choose_overload)
// ============================================================

// ChooseOverloadByCount selects an overload by parameter count.
// Builds a list of (index, count) pairs for fixed-length overloads,
// expanding ranges, and searches for an exact argc match.
// Falls back to the arbitrary-length overload if no fixed match.
func ChooseOverloadByCount(fd *kfn.FuncDef, argc int) (int, error) {
	if len(fd.Prototypes) <= 1 {
		return 0, nil
	}
	lens := GetPrototypeLengths(fd)
	arbIdx := -1
	type idxLen struct{ idx, len int }
	var nonarbs []idxLen

	for i, pl := range lens {
		if pl.Min == -1 {
			arbIdx = i
			continue
		}
		// Expand the range [min..max] into individual entries
		for n := pl.Min; n <= pl.Max; n++ {
			nonarbs = append(nonarbs, idxLen{i, n})
		}
	}

	// Search for exact match
	for _, il := range nonarbs {
		if il.len == argc {
			return il.idx, nil
		}
	}

	if arbIdx >= 0 {
		return arbIdx, nil
	}
	return 0, fmt.Errorf("unable to find a prototype for '%s' matching %d parameters", fd.Ident, argc)
}

// ============================================================
// Overload selection by parameter types (from function.ml choose_overload)
// ============================================================

// OverloadInfo holds computed info about one prototype overload.
type OverloadInfo struct {
	Total      int  // total non-return simple params
	Optional   int  // number of optional params
	HasRepeated bool // has Special/Complex/Argc params
	Index      int  // original prototype index
	Defined    bool // false = undefined/arbitrary prototype
}

// AnalyzeOverloads computes OverloadInfo for each prototype.
func AnalyzeOverloads(protos []kfn.Prototype) []OverloadInfo {
	result := make([]OverloadInfo, len(protos))
	for idx, proto := range protos {
		if !proto.Defined {
			result[idx] = OverloadInfo{Index: idx, Defined: false}
			continue
		}
		t, o := 0, 0
		hasR := false
		for _, p := range proto.Params {
			if p.Type == kfn.PSpecial || p.Type == kfn.PComplex {
				hasR = true
				continue
			}
			isReturn, isOpt, isArgc := false, false, false
			for _, f := range p.Flags {
				switch f {
				case kfn.FReturn:
					isReturn = true
				case kfn.FOptional:
					isOpt = true
				case kfn.FArgc:
					isArgc = true
				}
			}
			if isArgc {
				hasR = true
				continue
			}
			if isReturn {
				continue
			}
			t++
			if isOpt {
				o++
			}
		}
		result[idx] = OverloadInfo{
			Total: t, Optional: o, HasRepeated: hasR,
			Index: idx, Defined: true,
		}
	}
	return result
}

// ChooseOverloadByParams selects an overload by analyzing the actual
// parameter list. Counts simple (non-function-call) parameters and
// matches against the range [total-optional .. total] for each overload.
// Falls back to undefined/arbitrary overload if no match.
func ChooseOverloadByParams(protos []kfn.Prototype, params []ast.Param) (int, error) {
	if len(protos) <= 1 {
		return 0, nil
	}
	infos := AnalyzeOverloads(protos)

	// Count simple params (skip nested function calls)
	paramCount := 0
	for _, p := range params {
		if sp, ok := p.(ast.SimpleParam); ok {
			if _, isFunc := sp.Expr.(ast.FuncCall); !isFunc {
				paramCount++
			}
		}
	}

	// Search defined overloads by param count range
	for _, info := range infos {
		if !info.Defined {
			continue
		}
		if paramCount >= info.Total-info.Optional && paramCount <= info.Total {
			return info.Index, nil
		}
	}

	// Fallback: find undefined (arbitrary) prototype
	for _, info := range infos {
		if !info.Defined {
			return info.Index, nil
		}
	}

	return 0, fmt.Errorf("no overload matches %d parameters", paramCount)
}

// ============================================================
// Function assembly (from funcAsm.ml compile_function)
// ============================================================

// AssembleResult holds the output of function assembly.
type AssembleResult struct {
	Code   []byte // main opcode + params bytecode
	Append []byte // optional append (for simulated return values)
}

// Assemble builds the bytecode for a function call.
// Handles: overload selection, return value injection/simulation,
// argc adjustment for Uncount params, opcode header encoding.
func Assemble(fd *kfn.FuncDef, params []AsmParam, overload int, returnVal string) (AssembleResult, error) {
	hasPushStore := fd.HasFlag(kfn.FlagPushStore)
	argc := len(params)
	if returnVal != "" && !hasPushStore {
		argc++ // return value counts as a param
	}

	finalParams := params
	finalArgc := argc
	appendCode := ""

	// If we have a valid prototype, handle return value and argc adjustments
	if overload < len(fd.Prototypes) && fd.Prototypes[overload].Defined {
		proto := fd.Prototypes[overload].Params

		// Find return value position and uncount modifier
		rvPos := -1
		argcMod := 0
		for i, p := range proto {
			for _, f := range p.Flags {
				if f == kfn.FReturn {
					rvPos = i
				}
				if f == kfn.FUncount {
					argcMod++
				}
			}
		}

		// Handle return value
		isStore := returnVal == "" || returnVal == "$\xc8"
		switch {
		case rvPos == -1 && isStore:
			// No return position, store or no return → nothing to do
		case rvPos == -1 && !isStore:
			if hasPushStore {
				// Simulate: append "returnval \= store"
				appendCode = returnVal + "\\\x1e$\xc8"
			} else {
				return AssembleResult{}, fmt.Errorf("function '%s' does not return a value", fd.Ident)
			}
		case rvPos >= 0 && returnVal == "":
			return AssembleResult{}, fmt.Errorf("return value of function '%s' cannot be ignored", fd.Ident)
		case rvPos >= 0 && returnVal != "":
			// Insert returnVal at rvPos in parameter list
			newParams := make([]AsmParam, 0, len(params)+1)
			for i := 0; i < rvPos && i < len(params); i++ {
				newParams = append(newParams, params[i])
			}
			newParams = append(newParams, AsmParam{Kind: AsmUnknown, Code: returnVal})
			for i := rvPos; i < len(params); i++ {
				newParams = append(newParams, params[i])
			}
			finalParams = newParams
		}

		finalArgc = argc - argcMod
	} else {
		// No prototype: if there's a return value and no PushStore → error
		if returnVal != "" && !hasPushStore {
			return AssembleResult{}, fmt.Errorf("assignment syntax only valid for functions with prototypes")
		}
	}

	// Build opcode header + serialized parameters
	opcodeBytes := codegen.EncodeOpcode(fd.OpType, fd.OpModule, fd.OpCode, finalArgc, overload)
	var b strings.Builder
	b.Write(opcodeBytes)
	if len(finalParams) > 0 {
		b.WriteByte('(')
		b.WriteString(SerializeParams(finalParams))
		b.WriteByte(')')
	}

	result := AssembleResult{Code: []byte(b.String())}
	if appendCode != "" {
		result.Append = []byte(appendCode)
	}
	return result, nil
}

// AssembleStr is a convenience that concatenates Code and Append.
func AssembleStr(fd *kfn.FuncDef, params []AsmParam, overload int, returnVal string) (string, error) {
	r, err := Assemble(fd, params, overload, returnVal)
	if err != nil {
		return "", err
	}
	if r.Append != nil {
		return string(r.Code) + string(r.Append), nil
	}
	return string(r.Code), nil
}

// ============================================================
// Type checking helpers (from function.ml)
// ============================================================

// TypeName returns a human-readable name for a KFN parameter type.
func TypeName(pt kfn.ParamType) string {
	switch pt {
	case kfn.PAny:     return "any type"
	case kfn.PInt:     return "integer variable"
	case kfn.PIntC, kfn.PIntV: return "integer"
	case kfn.PStr:     return "string variable"
	case kfn.PStrC, kfn.PStrV, kfn.PResStr: return "string"
	case kfn.PSpecial: return "special function"
	case kfn.PComplex: return "tuple"
	}
	return "unknown"
}

// ExprType classifies a normalized expression for type checking.
type ExprType int

const (
	ETInt     ExprType = iota // integer value
	ETStr                     // string variable
	ETLiteral                 // string literal
	ETInvalid                 // unresolved/error
)

// ClassifyExpr determines the type of a normalized expression.
func ClassifyExpr(e ast.Expr) ExprType {
	switch x := e.(type) {
	case ast.IntLit, ast.StoreRef, ast.IntVar:
		return ETInt
	case ast.CmpExpr, ast.ChainExpr:
		return ETInt
	case ast.UnaryExpr:
		return ETInt
	case ast.StrLit:
		return ETLiteral
	case ast.StrVar:
		return ETStr
	case ast.BinOp:
		return ClassifyExpr(x.LHS)
	case ast.ParenExpr:
		return ClassifyExpr(x.Expr)
	}
	return ETInvalid
}

// CheckParamType verifies that an expression matches the expected KFN type.
// Returns empty string if OK, error message if mismatched.
func CheckParamType(expected kfn.ParamType, actual ExprType) string {
	switch expected {
	case kfn.PAny:
		return ""
	case kfn.PInt:
		if actual != ETInt {
			return fmt.Sprintf("expected integer variable, found %s", etName(actual))
		}
	case kfn.PIntC, kfn.PIntV:
		if actual != ETInt {
			return fmt.Sprintf("expected integer, found %s", etName(actual))
		}
	case kfn.PStr:
		if actual != ETStr {
			return fmt.Sprintf("expected string variable, found %s", etName(actual))
		}
	case kfn.PStrC, kfn.PStrV, kfn.PResStr:
		if actual != ETStr && actual != ETLiteral {
			return fmt.Sprintf("expected string, found %s", etName(actual))
		}
	}
	return ""
}

func etName(t ExprType) string {
	switch t {
	case ETInt:     return "integer"
	case ETStr:     return "string"
	case ETLiteral: return "string literal"
	case ETInvalid: return "invalid expression"
	}
	return "unknown"
}

// ============================================================
// Function definition lookup (from function.ml get_func_def)
// ============================================================

// LookupFuncDef retrieves a function definition from the registry.
// When multiple definitions exist for the same name (inter-opcode
// overloading, e.g. grpMulti or bgmLoop which has both a Kinetic and a
// RealLive variant), candidates are first filtered by target
// compatibility, then disambiguated by first parameter type.
//
// Filtering by target is critical: bgmLoop is defined as both
// `<0:Sys:2>` (under `ver Kinetic`) and `<1:Bgm:0>` (under `ver Avg2000,
// RealLive`). Without the filter, the first entry wins and the Clannad
// build gets the Kinetic opcode — which the engine doesn't expect and
// which decompresses to a completely different instruction.
func LookupFuncDef(reg *kfn.Registry, ident string, params []ast.Param, ctrlCode bool) (*kfn.FuncDef, error) {
	var table map[string][]*kfn.FuncDef
	if ctrlCode {
		table = reg.CtrlCodes
	} else {
		table = reg.Functions
	}

	allFns, ok := table[ident]
	if !ok || len(allFns) == 0 {
		// Synthesize a FuncDef from a literal opcode of the form
		//   op<TYPE:MODULE:FUNCTION, OVERLOAD>
		// emitted by the disassembler when the KFN had no name for an
		// opcode. We encode it directly with the parsed numeric coords.
		// Module is stored numerically by the disassembler ("035") but
		// may also have been re-named symbolically ("Sys"), so we
		// accept both forms here.
		if fn, ok := parseOpLiteralWithReg(ident, reg); ok {
			return fn, nil
		}
		return nil, fmt.Errorf("undefined function '%s'", ident)
	}

	// Filter candidates by target compatibility. Falls back to the full
	// list if nothing matches so legacy KFN entries (no target
	// constraints) still work.
	var fns []*kfn.FuncDef
	for _, fn := range allFns {
		if reg.ValidForTarget(fn) {
			fns = append(fns, fn)
		}
	}
	if len(fns) == 0 {
		fns = allFns
	}

	if len(fns) == 1 {
		return fns[0], nil
	}

	// Multiple definitions: disambiguate by first parameter type
	if len(params) > 0 {
		if sp, ok := params[0].(ast.SimpleParam); ok {
			et := ClassifyExpr(sp.Expr)
			for _, fn := range fns {
				for _, proto := range fn.Prototypes {
					if !proto.Defined || len(proto.Params) == 0 {
						continue
					}
					p1 := proto.Params[0].Type
					switch {
					case et == ETInt && isIntType(p1):
						return fn, nil
					case (et == ETStr || et == ETLiteral) && isStrType(p1):
						return fn, nil
					}
				}
			}
		}
	}

	// Default to first
	return fns[0], nil
}

func isIntType(pt kfn.ParamType) bool {
	return pt == kfn.PInt || pt == kfn.PIntC || pt == kfn.PIntV
}

func isStrType(pt kfn.ParamType) bool {
	return pt == kfn.PStr || pt == kfn.PStrC || pt == kfn.PStrV || pt == kfn.PResStr
}

// ============================================================
// Prototype definition array building (from function.ml check_and_compile)
// ============================================================

// BuildParamDefs creates an array of parameter definitions matching the
// actual parameter list length. Filters out Return params from the
// prototype, extends Argc params to fill remaining slots.
func BuildParamDefs(proto []kfn.Parameter, paramCount int) []kfn.Parameter {
	// Filter out Return params
	var nonReturn []kfn.Parameter
	for _, p := range proto {
		isReturn := false
		for _, f := range p.Flags {
			if f == kfn.FReturn {
				isReturn = true
				break
			}
		}
		if !isReturn {
			nonReturn = append(nonReturn, p)
		}
	}

	arr := make([]kfn.Parameter, paramCount)
	dlen := len(nonReturn)
	if dlen > paramCount {
		dlen = paramCount
	}
	for i := 0; i < dlen; i++ {
		arr[i] = nonReturn[i]
	}

	// If we have fewer defs than params and the last def has Argc,
	// replicate it to fill remaining slots
	if dlen < paramCount && dlen > 0 {
		lastDef := nonReturn[dlen-1]
		hasArgc := false
		for _, f := range lastDef.Flags {
			if f == kfn.FArgc {
				hasArgc = true
			}
		}
		if hasArgc {
			for i := dlen; i < paramCount; i++ {
				arr[i] = lastDef
			}
		}
	}

	return arr
}

// parseOpLiteralWithReg is the registry-aware variant of parseOpLiteral:
// it accepts a symbolic module name (e.g. "Shl") and resolves it
// against the registry's Modules table before bailing out.
func parseOpLiteralWithReg(ident string, reg *kfn.Registry) (*kfn.FuncDef, bool) {
	if fn, ok := parseOpLiteral(ident); ok {
		return fn, true
	}
	// Symbolic module fallback. Re-parse manually accepting a name.
	const prefix = "op<"
	if len(ident) < len(prefix)+1 || ident[:len(prefix)] != prefix || ident[len(ident)-1] != '>' {
		return nil, false
	}
	body := ident[len(prefix) : len(ident)-1]
	commaIdx := -1
	for i := 0; i < len(body); i++ {
		if body[i] == ',' {
			commaIdx = i
			break
		}
	}
	if commaIdx < 0 {
		return nil, false
	}
	left := body[:commaIdx]
	right := body[commaIdx+1:]
	parts := []string{}
	last := 0
	for i := 0; i < len(left); i++ {
		if left[i] == ':' {
			parts = append(parts, left[last:i])
			last = i + 1
		}
	}
	parts = append(parts, left[last:])
	if len(parts) != 3 {
		return nil, false
	}
	typ, err1 := strconv.Atoi(parts[0])
	if err1 != nil {
		return nil, false
	}
	// Resolve module name through registry.
	modNum, ok := reg.Modules[parts[1]]
	if !ok {
		return nil, false
	}
	fn, err3 := strconv.Atoi(parts[2])
	if err3 != nil {
		return nil, false
	}
	overload, err4 := strconv.Atoi(right)
	if err4 != nil {
		return nil, false
	}
	return &kfn.FuncDef{
		Ident:             ident,
		OpType:            typ,
		OpModule:          modNum,
		OpCode:            fn,
		SyntheticOverload: overload,
	}, true
}

// parseOpLiteral attempts to interpret an identifier as a literal opcode
// of the form "op<TYPE:MODULE:FUNCTION,OVERLOAD>" emitted by the
// disassembler when no symbolic name was available. MODULE may be either
// numeric ("035") or symbolic ("Sys"); without access to the registry
// here, we only accept numeric module values. (Symbolic-module lookup
// happens at call sites that have a *kfn.Registry.)
func parseOpLiteral(ident string) (*kfn.FuncDef, bool) {
	const prefix = "op<"
	if len(ident) < len(prefix)+1 || ident[:len(prefix)] != prefix || ident[len(ident)-1] != '>' {
		return nil, false
	}
	body := ident[len(prefix) : len(ident)-1]
	// body == "TYPE:MODULE:FUNCTION,OVERLOAD"
	commaIdx := -1
	for i := 0; i < len(body); i++ {
		if body[i] == ',' {
			commaIdx = i
			break
		}
	}
	if commaIdx < 0 {
		return nil, false
	}
	left := body[:commaIdx]
	right := body[commaIdx+1:]
	parts := []string{}
	last := 0
	for i := 0; i < len(left); i++ {
		if left[i] == ':' {
			parts = append(parts, left[last:i])
			last = i + 1
		}
	}
	parts = append(parts, left[last:])
	if len(parts) != 3 {
		return nil, false
	}
	typ, err1 := strconv.Atoi(parts[0])
	if err1 != nil {
		return nil, false
	}
	modNum, err2 := strconv.Atoi(parts[1])
	if err2 != nil {
		return nil, false
	}
	fn, err3 := strconv.Atoi(parts[2])
	if err3 != nil {
		return nil, false
	}
	overload, err4 := strconv.Atoi(right)
	if err4 != nil {
		return nil, false
	}
	return &kfn.FuncDef{
		Ident:             ident,
		OpType:            typ,
		OpModule:          modNum,
		OpCode:            fn,
		SyntheticOverload: overload,
	}, true
}
