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

// parseParamProto pulls the first parameter prototype out of a KFN `fun`
// line. It is kept for older tests/helpers; ParseKFN uses parseParamProtos so
// overloaded functions retain every parenthesised prototype after the opcode.
//
// The KFN parameter mini-grammar we recognise:
//
//	param   ::= flag* type ('\'' name '\'')?
//	type    ::= 'int' | 'intC' | 'intV'
//	          | 'str' | 'strC' | 'strV'
//	          | 'res' | 'special' | 'complex'
//	          | 'any'
//	flag    ::= '<' | '>' | '+' | '~' | …
//
// Unknown tokens map to ParamAny. This isn't a full parser; just enough
// to know whether a position is a `res`-typed string so we can route it
// through the resource file like OCaml does, and whether a position is a
// return slot so return-param functions can be rendered as assignments.
func parseParamProto(line string) ([]ParamType, [][]ParamFlag) {
	protos, flags := parseParamProtos(line)
	if len(protos) == 0 {
		return nil, nil
	}
	return protos[0], flags[0]
}

func parseParamProtos(line string) ([][]ParamType, [][][]ParamFlag) {
	// Strip everything up to and including the opcode triple `<…,…>`.
	if idx := strings.Index(line, ">"); idx >= 0 {
		// In hardwired '/// fun_name <opcode>' lines there might be no
		// further parens; that's fine — we'll bail out below.
		line = line[idx+1:]
	}

	var protos [][]ParamType
	var allFlags [][][]ParamFlag
	for {
		open := strings.Index(line, "(")
		if open < 0 {
			break
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
		if close >= 0 {
			body := line[open+1 : close]
			proto, flags := parseParamBody(body)
			protos = append(protos, proto)
			allFlags = append(allFlags, flags)
			line = line[close+1:]
			continue
		}
		break
	}

	if len(protos) == 0 {
		return nil, nil
	}
	return protos, allFlags
}

func parseParamBody(body string) ([]ParamType, [][]ParamFlag) {
	// Split on top-level commas. The inner grammar can contain
	// quoted names but no nested parens at this stage.
	var params []ParamType
	var flags [][]ParamFlag
	cur := ""
	inQuote := false
	flushParam := func() {
		token := strings.TrimSpace(cur)
		cur = ""
		if token == "" {
			return
		}
		// Strip leading flag chars (`<`, `>`, `+`, `~`, `#`, etc.) and
		// retain only the ones that affect source rendering.
		var paramFlags []ParamFlag
		for len(token) > 0 {
			c := token[0]
			if c == '>' {
				paramFlags = append(paramFlags, ParamReturn)
				token = token[1:]
				continue
			}
			if c == '<' || c == '+' || c == '~' || c == '&' || c == '*' || c == '#' || c == '=' || c == ' ' || c == '\t' {
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
		flags = append(flags, paramFlags)
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
	return params, flags
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
	return LoadKFNForTarget(path, ModeRealLive, Version{1, 2, 7, 0})
}

func LoadKFNForTarget(path string, mode EngineMode, version Version) (*FuncRegistry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return ParseKFNForTarget(f, mode, version)
}

// ParseKFN parses a KFN file from a reader.
func ParseKFN(r io.Reader) (*FuncRegistry, error) {
	return ParseKFNForTarget(r, ModeRealLive, Version{1, 2, 7, 0})
}

func ParseKFNForTarget(r io.Reader, mode EngineMode, version Version) (*FuncRegistry, error) {
	if mode == ModeNone {
		mode = ModeRealLive
	}
	if version == (Version{}) {
		if mode == ModeAvg2000 {
			version = Version{1, 0, 0, 0}
		} else {
			version = Version{1, 2, 7, 0}
		}
	}
	reg := NewFuncRegistry()
	reg.Mode = mode
	reg.Version = version

	// Module name → number mapping (Jmp→1, Bgm→4, etc.)
	modNames := make(map[string]int)

	// Regex for opcode triple: <type:mod:code, overload>
	opcodeRe := regexp.MustCompile(`<\s*(\d+)\s*:\s*(\w+)\s*:\s*(\d+)\s*,\s*(\d+)\s*>`)

	// Regex for module declarations: module NNN = Name
	moduleRe := regexp.MustCompile(`^\s*module\s+(\d+)\s*=\s*(\w+)`)

	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // big buffer for long lines

	var logicalLines []string
	current := ""
	flush := func() {
		if strings.TrimSpace(current) != "" {
			logicalLines = append(logicalLines, current)
			current = ""
		}
	}

	for scanner.Scan() {
		trimmed := strings.TrimSpace(scanner.Text())
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "//") && !strings.HasPrefix(trimmed, "///") {
			continue
		}

		startsNew := strings.HasPrefix(trimmed, "fun ") ||
			strings.HasPrefix(trimmed, "/// ") ||
			moduleRe.MatchString(trimmed)
		if startsNew {
			flush()
			current = trimmed
			continue
		}

		if strings.HasPrefix(strings.TrimSpace(current), "fun ") && strings.HasPrefix(trimmed, "(") {
			current += " " + trimmed
			continue
		}

		flush()
		current = trimmed
	}
	flush()

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	var activeStack []bool
	for _, trimmed := range logicalLines {
		// Module declarations
		if m := moduleRe.FindStringSubmatch(trimmed); m != nil {
			num, _ := strconv.Atoi(m[1])
			name := m[2]
			modNames[name] = num
			reg.RegisterModule(num, name)
			continue
		}

		if strings.HasPrefix(trimmed, "ver ") {
			activeStack = append(activeStack, kfnConditionApplies(trimmed, mode, version))
			continue
		}
		if trimmed == "end" || strings.HasPrefix(trimmed, "end ") {
			if len(activeStack) > 0 {
				activeStack = activeStack[:len(activeStack)-1]
			}
			continue
		}
		if !kfnActive(activeStack) {
			continue
		}

		// Function definitions: fun <ident> ...
		if strings.HasPrefix(trimmed, "fun ") {
			name := extractDisplayFunName(trimmed)
			if name == "" {
				continue
			}
			// Parse the optional ccode annotation `{...}` that sits
			// between the function name and the opcode triple.
			// See kfnParser.mly L129-L135 for the OCaml grammar:
			//   ccode:
			//     | /* empty */         → no kepago form
			//     | Lbr Rbr             → {}      Unnamed, uses fn name
			//     | Lbr IDENT Rbr       → {xxx}   Named
			//     | Lbr St IDENT Rbr    → {*xxx}  Named + IsTextout
			//     | Lbr Eq IDENT Rbr    → {=xxx}  Named + NoBraces
			//     | Lbr St Eq IDENT Rbr → {*=xxx} Named + NoBraces + IsLbr
			ccode, ccodeFlags := extractCcode(trimmed, name)
			if m := opcodeRe.FindStringSubmatch(trimmed); m != nil {
				opType, _ := strconv.Atoi(m[1])
				modStr := m[2]
				opCode, _ := strconv.Atoi(m[3])
				overload, _ := strconv.Atoi(m[4])

				// Resolve module name → number
				modNum := resolveModule(modStr, modNames)

				opStr := fmt.Sprintf("%d:%03d:%05d,%d", opType, modNum, opCode, overload)
				def := FuncDef{Name: name, Ccode: ccode}
				def.Flags = append(def.Flags, ccodeFlags...)

				// Parse flags from the parenthesized hint.
				// Match exact hint tokens: `(store goto)` carries one
				// out-of-band pointer after the argument list, while
				// `(gotos)` opcodes carry a whole label table and are
				// handled by their dedicated readers.
				if hasHintToken(trimmed, "goto") {
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
				if protos, flags := parseParamProtos(trimmed); len(protos) > 0 {
					def.Prototypes = protos
					def.ParamFlags = flags
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

	return reg, nil
}

func kfnActive(stack []bool) bool {
	for _, active := range stack {
		if !active {
			return false
		}
	}
	return true
}

func kfnConditionApplies(line string, mode EngineMode, version Version) bool {
	cond := strings.TrimSpace(strings.TrimPrefix(line, "ver"))
	if idx := strings.Index(cond, "//"); idx >= 0 {
		cond = cond[:idx]
	}

	parts := strings.Split(cond, ",")
	sawTarget := false
	targetOK := false
	for _, part := range parts {
		fields := strings.Fields(strings.TrimSpace(part))
		if len(fields) == 0 {
			continue
		}

		switch strings.ToLower(fields[0]) {
		case "reallive":
			sawTarget = true
			targetOK = targetOK || mode == ModeRealLive
		case "avg2000", "avg2k":
			sawTarget = true
			targetOK = targetOK || mode == ModeAvg2000
		case "kinetic":
			sawTarget = true
			targetOK = targetOK || mode == ModeKinetic
		case "avg32", "avg":
			sawTarget = true
			targetOK = targetOK || mode == ModeAVG32
		case "<", "<=", ">", ">=", "=", "==":
			if len(fields) < 2 || !versionCompareApplies(version, fields[0], parseKFNVersion(fields[1])) {
				return false
			}
		default:
			return false
		}
	}
	return !sawTarget || targetOK
}

func versionCompareApplies(cur Version, op string, want Version) bool {
	cmp := compareKFNVersion(cur, want)
	switch op {
	case "<":
		return cmp < 0
	case "<=":
		return cmp <= 0
	case ">":
		return cmp > 0
	case ">=":
		return cmp >= 0
	case "=", "==":
		return cmp == 0
	default:
		return false
	}
}

func compareKFNVersion(a, b Version) int {
	for i := 0; i < 4; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

func parseKFNVersion(s string) Version {
	var v Version
	parts := strings.Split(strings.TrimSpace(s), ".")
	for i := 0; i < len(parts) && i < 4; i++ {
		n, _ := strconv.Atoi(parts[i])
		v[i] = n
	}
	return v
}

func extractDisplayFunName(line string) string {
	name := extractFunName(line)
	if name == "" {
		return ""
	}
	alias := extractFunAlias(line, name)
	if alias != "" && strings.HasPrefix(name, "__") && !hasFakeParamSyntax(line) {
		return alias
	}
	return name
}

func hasHintToken(line, token string) bool {
	opIdx := strings.Index(line, "<")
	if opIdx < 0 {
		return false
	}
	hints := line[:opIdx]
	start := strings.Index(hints, "(")
	end := strings.Index(hints, ")")
	if start < 0 || end < start {
		return false
	}
	for _, field := range strings.Fields(hints[start+1 : end]) {
		if field == token {
			return true
		}
	}
	return false
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

func extractFunAlias(line, funcName string) string {
	rest := strings.TrimPrefix(line, "fun ")
	rest = strings.TrimSpace(rest)
	if !strings.HasPrefix(rest, funcName) {
		return ""
	}
	rest = strings.TrimSpace(rest[len(funcName):])
	if rest == "" || rest[0] == '(' || rest[0] == '<' || rest[0] == '{' {
		return ""
	}
	end := 0
	for end < len(rest) && rest[end] != ' ' && rest[end] != '\t' && rest[end] != '(' && rest[end] != '<' && rest[end] != '{' {
		end++
	}
	if end == 0 {
		return ""
	}
	return rest[:end]
}

func hasFakeParamSyntax(line string) bool {
	opEnd := strings.Index(line, ">")
	if opEnd < 0 || opEnd+1 >= len(line) {
		return false
	}
	return strings.Contains(line[opEnd+1:], "=")
}

// extractCcode pulls the kepago control-code annotation `{...}` that
// may appear between the function name and the opcode triple in a KFN
// `fun` line. Returns ("", nil) if no annotation is present.
//
// Maps the OCaml kfnParser.mly grammar (L129-L135):
//
//	{}        → ("<name>", [])              — Unnamed, uses fn name as ccode
//	{xxx}     → ("xxx",    [])              — Named
//	{*xxx}    → ("xxx",    [IsTextout])
//	{=xxx}    → ("xxx",    [NoBraces])
//	{*=xxx}   → ("xxx",    [NoBraces, IsLbr])
//
// We only scan the substring between the function name and the first
// '<' (opcode triple start) so we don't pick up the parameter braces
// later in the line.
func extractCcode(line, funcName string) (string, []FuncFlag) {
	// Locate the substring between fn name and opcode '<'.
	nameIdx := strings.Index(line, funcName)
	if nameIdx < 0 {
		return "", nil
	}
	rest := line[nameIdx+len(funcName):]
	opStart := strings.Index(rest, "<")
	if opStart < 0 {
		return "", nil
	}
	zone := rest[:opStart]
	// Find a `{...}` group; skip the parenthesised flag hint if any.
	lb := strings.Index(zone, "{")
	if lb < 0 {
		return "", nil
	}
	rb := strings.Index(zone[lb:], "}")
	if rb < 0 {
		return "", nil
	}
	inner := strings.TrimSpace(zone[lb+1 : lb+rb])
	// Parse the leading flag markers.
	var flags []FuncFlag
	hasStar := false
	hasEq := false
	for len(inner) > 0 {
		switch inner[0] {
		case '*':
			hasStar = true
			inner = inner[1:]
		case '=':
			hasEq = true
			inner = inner[1:]
		default:
			goto doneFlags
		}
	}
doneFlags:
	// Set flags per grammar.
	switch {
	case hasStar && hasEq:
		flags = append(flags, FlagNoBraces, FlagIsLbr)
	case hasEq:
		flags = append(flags, FlagNoBraces)
	case hasStar:
		flags = append(flags, FlagIsTextout)
	}
	// The remaining content is the ccode name; if empty (Unnamed),
	// fall back to the function name.
	ccode := strings.TrimSpace(inner)
	if ccode == "" {
		ccode = funcName
	}
	return ccode, flags
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
