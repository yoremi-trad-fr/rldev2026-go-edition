// Function typechecking and compilation.
// Transposed from OCaml's rlc/function.ml (~726 lines) + funcAsm.ml (~210 lines).
//
// Handles:
//   - Looking up function definitions in the KFN registry
//   - Overload resolution based on parameter types
//   - Parameter type checking and compilation
//   - Special parameter handling (condition flags, complex tuples)
//   - Bytecode assembly for function opcodes
package compiler

import (
	"fmt"

	"github.com/yoremi/rldev-go/pkg/kfn"
	"github.com/yoremi/rldev-go/pkg/rlc/ast"
	"github.com/yoremi/rldev-go/pkg/rlc/codegen"
)

// resolveFunc looks up a function by identifier and returns matching definitions.
func (c *Compiler) resolveFunc(ident string) []kfn.FuncDef {
	if c.Reg == nil { return nil }
	fns, ok := c.Reg.Functions[ident]
	if !ok { return nil }
	return fns
}

// chooseOverload selects the best function overload for the given parameters.
// In RealLive, functions can have multiple prototypes (overloads) with different
// parameter counts and types.
func (c *Compiler) chooseOverload(loc ast.Loc, fns []kfn.FuncDef, params []ast.Param) (kfn.FuncDef, int) {
	if len(fns) == 0 {
		return kfn.FuncDef{}, -1
	}
	if len(fns) == 1 {
		return fns[0], 0
	}

	// Try to find an exact parameter count match
	for i, fn := range fns {
		for _, proto := range fn.Prototypes {
			if len(proto.Params) == len(params) {
				return fn, i
			}
		}
	}

	// Fallback: return first definition
	return fns[0], 0
}

// compileResolvedFunc compiles a function call after resolving the definition.
func (c *Compiler) compileResolvedFunc(loc ast.Loc, fn kfn.FuncDef, params []ast.Param, label *ast.Label) {
	overload := 0
	if len(fn.Prototypes) > 1 {
		// Try to match parameter count to select overload
		for i, proto := range fn.Prototypes {
			if len(proto.Params) == len(params) {
				overload = i
				break
			}
		}
	}

	argc := len(params)
	if label != nil { argc++ }

	c.Output.EmitOpcode(loc, fn.OpType, fn.OpModule, fn.OpFunction, argc, overload)

	hasParams := len(params) > 0 || label != nil
	if hasParams {
		c.Output.AddCode(loc, []byte{'('})
		for i, p := range params {
			if i > 0 { c.Output.AddCode(loc, []byte{','}) }
			c.compileTypedParam(loc, fn, overload, i, p)
		}
		if label != nil {
			if len(params) > 0 { c.Output.AddCode(loc, []byte{','}) }
			c.Output.AddLabelRef(label.Ident, loc)
		}
		c.Output.AddCode(loc, []byte{')'})
	}
}

// compileTypedParam compiles a parameter with type checking.
func (c *Compiler) compileTypedParam(loc ast.Loc, fn kfn.FuncDef, overload, index int, p ast.Param) {
	// If we have a prototype, try to get the expected type
	if overload < len(fn.Prototypes) {
		proto := fn.Prototypes[overload]
		if index < len(proto.Params) {
			pdef := proto.Params[index]
			c.compileParamWithType(loc, p, pdef)
			return
		}
	}
	// Fallback: compile without type checking
	c.compileParam(loc, p)
}

// compileParamWithType compiles a parameter checking against the expected KFN type.
func (c *Compiler) compileParamWithType(loc ast.Loc, p ast.Param, pdef kfn.Parameter) {
	switch sp := p.(type) {
	case ast.SimpleParam:
		expr := c.resolveExpr(sp.Expr)

		// Type check
		switch pdef.Type {
		case kfn.PInt, kfn.PIntC, kfn.PIntV:
			// Ensure integer expression
			c.Output.EmitExpr(expr)
		case kfn.PStr, kfn.PStrC, kfn.PStrV:
			// Ensure string expression
			c.emitStrExpr(loc, expr)
		default:
			c.Output.EmitExpr(expr)
		}

	case ast.ComplexParam:
		// Complex/tuple parameter: {expr, expr, ...}
		c.Output.AddCode(loc, []byte{byte(0xa0)})
		for i, e := range sp.Exprs {
			if i > 0 { c.Output.AddCode(loc, []byte{','}) }
			resolved := c.resolveExpr(e)
			c.Output.EmitExpr(resolved)
		}

	case ast.SpecialParam:
		// Special function parameter
		c.Output.AddCode(loc, []byte{byte(0xa0 + sp.ID)})
		for _, e := range sp.Exprs {
			resolved := c.resolveExpr(e)
			c.Output.EmitExpr(resolved)
		}

	default:
		c.compileParam(loc, p)
	}
}

// emitStrExpr emits a string expression in the correct format.
func (c *Compiler) emitStrExpr(loc ast.Loc, expr ast.Expr) {
	switch e := expr.(type) {
	case ast.StrLit:
		// Emit string literal as quoted bytes
		for _, tok := range e.Tokens {
			if tt, ok := tok.(ast.TextToken); ok {
				c.Output.AddCode(loc, []byte{'"'})
				c.Output.AddCode(loc, []byte(tt.Text))
				c.Output.AddCode(loc, []byte{'"'})
			}
		}
	case ast.StrVar:
		// Emit string variable reference
		c.Output.EmitExpr(e)
	default:
		c.Output.EmitExpr(expr)
	}
}

// assembleOpcode builds the raw opcode bytes for a function call.
// Format: 0x23 type mod:2 func:2 argc:2 overload:2
func assembleOpcode(opType, opModule, opFunc, argc, overload int) []byte {
	buf := make([]byte, 8)
	buf[0] = '#'
	buf[1] = byte(opType)
	buf[2] = byte(opModule)
	buf[3] = byte(opModule >> 8)
	buf[4] = byte(opFunc)
	buf[5] = byte(opFunc >> 8)
	buf[6] = byte(argc)
	buf[7] = byte(overload)
	return buf
}

// compileFuncStr builds a function call as a string for insertion.
// Used for helper calls like zentohan in double-quote handling.
func compileFuncStr(loc ast.Loc, fn kfn.FuncDef, args []string) string {
	s := fmt.Sprintf("#%c%c%c%c%c%c%c",
		fn.OpType,
		fn.OpModule&0xff, fn.OpModule>>8,
		fn.OpFunction&0xff, fn.OpFunction>>8,
		len(args), 0)
	if len(args) > 0 {
		s += "("
		for i, a := range args {
			if i > 0 { s += "," }
			s += a
		}
		s += ")"
	}
	return s
}

// Ensure codegen import is used
var _ = codegen.EncodeInt32
