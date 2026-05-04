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
				if strings.Contains(trimmed, "(store ") {
					def.Flags = append(def.Flags, FlagPushStore)
				}
				if strings.Contains(trimmed, "(call)") || strings.Contains(trimmed, "(store call)") {
					def.Flags = append(def.Flags, FlagIsCall)
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
