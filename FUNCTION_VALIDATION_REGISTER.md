# RLdev2026-Go function validation register

Last update: 2026-05-24

This register tracks opcode/function signatures that have been checked against
known-good game corpuses. It is meant to avoid re-auditing the same RealLive
function families for every new title.

## Method

- Compile the extracted `.org` sources with the target game's `RealLive.exe`
  and `Gameexe.ini`.
- Disassemble compiled files with opcode annotations.
- Compare the disassembled opcode signatures against original extracted files.
- For sensitive overload cases, confirm the byte header manually when needed.
- Keep version-specific rules tied to the interpreter version, not to one game
  name, unless the corpus proves otherwise.

## CLANNAD 2007 FV

- Interpreter: RealLive 1.2.3.5.
- Corpus: 242 FR files and 242 JP SJIS files.
- Status: validated for the string-return overload cases below.
- Note: compiling the FR UTF-8 corpus without a Western output transform emits
  accent conversion warnings. This is separate from opcode signature parity.

Validated signatures:

| Function | Opcode | Expected overload | Count FR | Count JP | Status |
| --- | --- | ---: | ---: | ---: | --- |
| `strsub` | `1:010:00005` | 1 | 2 | 2 | fixed and matched |
| `itoa_s` | `1:010:00015` | 0 | 10 | 10 | fixed and matched |
| `itoa` | `1:010:00017` | 0 | 36 | 36 | fixed and matched |
| `itoa` | `1:010:00017` | 1 | 0 | 0 | absent, as expected |
| `strcpy` 3-arg form | `1:010:00000` | 1 | 6 | 6 | matched |

Findings fixed from this audit:

- RealLive 1.2.3.5 uses overload 0 for `itoa*` calls with 3 encoded args.
- `strsub` with 3 encoded args uses overload 1 in the original bytecode.
- Inner ASCII double quotes inside string literals must not be emitted as raw
  `0x22`; the compiler maps them to the observed SJIS quote pair.

## AIR 1.02

- Interpreter: RealLive 1.2.9.5.
- Corpus: 96 FR UTF-8 files and 96 JP SJIS files.
- Status: gameplay route start validated by user; static overload audit matched.

Validated signatures:

| Function | Opcode | Expected overload | Count FR | Count JP | Status |
| --- | --- | ---: | ---: | ---: | --- |
| `itoa` | `1:010:00017` | 1 | 10552 | 10552 | matched |
| `itoa` | `1:010:00017` | 0 | 0 | 0 | absent, as expected |
| `strcpy` | `1:010:00000` | 0 | 22199 | 22199 | matched |
| `strcat` | `1:010:00002` | 0 | 10552 | 10552 | matched |

Findings fixed from AIR debugging:

- KFN return parameters (`>str`, etc.) must be injected before string assignment
  rewrites. Example: `strS[0] = itoa(n, 2)` encodes as an `itoa` opcode with
  the destination string as a real parameter.
- RealLive 1.2.9.5 uses overload 1 for `itoa` calls with 3 encoded args.
- The `pause` immediately following static text output should not receive an
  extra debug line marker.

## Current compatibility rules

| Rule | Scope | Source |
| --- | --- | --- |
| `itoa_ws`, `itoa_s`, `itoa_w`, `itoa` with 3 encoded args use overload 0 before RealLive 1.2.9.0 and overload 1 from 1.2.9.0 onward. | Version-gated | CLANNAD 1.2.3.5, AIR 1.2.9.5 |
| `strsub` and `strrsub` with 3 encoded args use overload 1. | General until contradicted by a later corpus | CLANNAD 1.2.3.5 |
| Unknown interpreter version keeps the normal KFN/prototype selection. | Safety fallback | Compiler policy |

## To expand

- Add each new title with interpreter version, corpus size, and any newly seen
  opcode signatures.
- When a later title contradicts a rule, split it by RealLive version range
  instead of making a game-specific exception first.
