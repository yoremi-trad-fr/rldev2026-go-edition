package disasm

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// parseParamProto pulls the parameter-type tokens out of a KFN `fun`
// line. It looks for the last parenthesised group on the line (after
// the opcode triple) and returns one ParamType per top-level position.
//
// The KFN parameter mini-grammar we recognise:
//
//	param   ::= flag* type ('\'' name '\'')?
//	type    ::= 'int' | 'intC' | 'intV'
//	          | 'str' | 'strC' | 'strV'
//	          | 'res' | 'special' | 'complex'
//	          | 'any'
//	flag    ::= '<' | '+' | '~' | …  (ignored — we only care about types)
//
// Unknown tokens map to ParamAny. This isn't a full parser; just enough
// to know whether a position is a `res`-typed string so we can route it
// through the resource file like OCaml does.
func parseParamProto(line string) []ParamType {
	// Strip everything up to and including the opcode triple `<…,…>`.
	if idx := strings.Index(line, ">"); idx >= 0 {
		// In hardwired '/// fun_name <opcode>' lines there might be no
		// further parens; that's fine — we'll bail out below.
		line = line[idx+1:]
	}
	// Find the outermost parenthesised group.
	open := strings.Index(line, "(")
	if open < 0 {
		return nil
	}
	depth := 0
	close := -1
	for i := open; i < len(line); i++ {
		switch line[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				close = i
			}
		}
		if close >= 0 {
			break
		}
	}
	if close < 0 {
		return nil
	}
	body := line[open+1 : close]
	// Split on top-level commas. The inner grammar can contain
	// quoted names but no nested parens at this stage.
	var params []ParamType
	cur := ""
	inQuote := false
	flushParam := func() {
		token := strings.TrimSpace(cur)
		cur = ""
		if token == "" {
			return
		}
		// Strip leading flag chars (`<`, `+`, `~`, `#`, etc.) and any
		// quoted name annotation. `#` denotes an output/return slot in
		// KFN syntax (e.g. `# res 'text'`); the type word follows it.
		for len(token) > 0 {
			c := token[0]
			if c == '<' || c == '+' || c == '~' || c == '&' || c == '*' || c == '#' || c == ' ' || c == '\t' {
				token = token[1:]
				continue
			}
			break
		}
		// First whitespace-separated word is the type.
		typeWord := token
		if sp := strings.IndexAny(token, " \t"); sp >= 0 {
			typeWord = token[:sp]
		}
		params = append(params, paramTypeFromWord(typeWord))
	}
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case c == '\'':
			inQuote = !inQuote
			cur += string(c)
		case c == ',' && !inQuote:
			flushParam()
		default:
			cur += string(c)
		}
	}
	flushParam()
	return params
}

// paramTypeFromWord maps a KFN type word to ParamType.
func paramTypeFromWord(w string) ParamType {
	switch w {
	case "int":
		return ParamInt
	case "intC":
		return ParamIntC
	case "intV":
		return ParamIntV
	case "str":
		return ParamStr
	case "strC":
		return ParamStrC
	case "strV":
		return ParamStrV
	case "res":
		return ParamResStr
	}
	return ParamAny
}

// LoadKFN reads a RealLive Function Definition file (.kfn) and returns
// a populated FuncRegistry mapping opcode strings to function names.
//
// The KFN format has entries like:
//
//	module 001 = Jmp
//	fun goto (skip goto) <0:Jmp:00000, 0> ()
//	fun goto_if (if goto) <0:Jmp:00001, 0> (<'condition')
//	/// goto_on <0:Jmp:00003, 0> (special case)
//
// The parser extracts:
//   - Module declarations: module NNN = Name
//   - Function definitions: fun <ident> ... <type:mod:code, overload>
//   - Hardwired functions: /// <ident> <type:mod:code, overload>
func LoadKFN(path string) (*FuncRegistry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseKFN(f)
}

// ParseKFN parses a KFN file from a reader.
func ParseKFN(r io.Reader) (*FuncRegistry, error) {
	reg := NewFuncRegistry()

	// Module name → number mapping (Jmp→1, Bgm→4, etc.)
	modNames := make(map[string]int)

	// Regex for opcode triple: <type:mod:code, overload>
	opcodeRe := regexp.MustCompile(`<\s*(\d+)\s*:\s*(\w+)\s*:\s*(\d+)\s*,\s*(\d+)\s*>`)

	// Regex for module declarations: module NNN = Name
	moduleRe := regexp.MustCompile(`^\s*module\s+(\d+)\s*=\s*(\w+)`)

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // big buffer for long lines

	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines and pure comments
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "//") && !strings.HasPrefix(trimmed, "///") {
			continue
		}

		// Module declarations
		if m := moduleRe.FindStringSubmatch(trimmed); m != nil {
			num, _ := strconv.Atoi(m[1])
			name := m[2]
			modNames[name] = num
			reg.RegisterModule(num, name)
			continue
		}

		// Function definitions: fun <ident> ...
		if strings.HasPrefix(trimmed, "fun ") {
			name := extractFunName(trimmed)
			if name == "" {
				continue
			}
			if m := opcodeRe.FindStringSubmatch(trimmed); m != nil {
				opType, _ := strconv.Atoi(m[1])
				modStr := m[2]
				opCode, _ := strconv.Atoi(m[3])
				overload, _ := strconv.Atoi(m[4])

				// Resolve module name → number
				modNum := resolveModule(modStr, modNames)

				opStr := fmt.Sprintf("%d:%03d:%05d,%d", opType, modNum, opCode, overload)
				def := FuncDef{Name: name}

				// Parse flags from the parenthesized hint
				if strings.Contains(trimmed, "(skip ") {
					def.Flags = append(def.Flags, FlagIsGoto)
				}
				// PushStore: function leaves its return value on the
				// implicit `store` register. The disassembler folds a
				// subsequent `dst = store` assignment into the previous
				// command (see resolveStoreFold in writer.go).
				if strings.Contains(trimmed, "(store)") ||
					strings.Contains(trimmed, "(store ") ||
					strings.Contains(trimmed, " store)") ||
					strings.Contains(trimmed, "(if store") {
					def.Flags = append(def.Flags, FlagPushStore)
				}
				if strings.Contains(trimmed, "(call)") || strings.Contains(trimmed, "(store call)") {
					def.Flags = append(def.Flags, FlagIsCall)
				}

				// Parse the parameter list. The KFN format puts it as the
				// last parenthesised group on the line, e.g.
				//   fun title <1:Sys:00000, 0> (res 'sub-title')
				//   fun goto_if (if goto) <0:Jmp:00001, 0> (<'condition')
				// We extract just the param-type tokens (`int`, `str`,
				// `res`, …) so the disassembler can pass sep_str=true to
				// the string reader for `res`-typed params, matching
				// OCaml read_soft_function (disassembler.ml L2314).
				if proto := parseParamProto(trimmed); proto != nil {
					def.Prototypes = [][]ParamType{proto}
				}

				reg.Register(opStr, def)
			}
			continue
		}

		// Hardwired functions: /// <ident> <type:mod:code, overload>
		if strings.HasPrefix(trimmed, "/// ") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				name := parts[1]
				if m := opcodeRe.FindStringSubmatch(trimmed); m != nil {
					opType, _ := strconv.Atoi(m[1])
					modStr := m[2]
					opCode, _ := strconv.Atoi(m[3])
					overload, _ := strconv.Atoi(m[4])
					modNum := resolveModule(modStr, modNames)
					opStr := fmt.Sprintf("%d:%03d:%05d,%d", opType, modNum, opCode, overload)
					reg.Register(opStr, FuncDef{Name: name})
				}
			}
			continue
		}
	}

	return reg, scanner.Err()
}

// extractFunName extracts the function identifier from a 'fun' line.
// Format: fun <ident> (hints) <opcode> (params)
func extractFunName(line string) string {
	// Remove 'fun ' prefix
	rest := strings.TrimPrefix(line, "fun ")
	rest = strings.TrimSpace(rest)

	// The identifier is the first word
	end := 0
	for end < len(rest) && rest[end] != ' ' && rest[end] != '\t' && rest[end] != '(' {
		end++
	}
	if end == 0 {
		return ""
	}
	return rest[:end]
}

// resolveModule converts a module name (like "Jmp") or number string to a number.
func resolveModule(modStr string, modNames map[string]int) int {
	// Try as number first
	if n, err := strconv.Atoi(modStr); err == nil {
		return n
	}
	// Try as name
	if n, ok := modNames[modStr]; ok {
		return n
	}
	return 0
}
