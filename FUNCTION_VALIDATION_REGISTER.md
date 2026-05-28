# RLdev2026-Go function validation register

Last update: 2026-05-28

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

## CLANNAD Steam 2015

- Interpreter: `SiglusEngine_Steam.exe`, RealLive file version 1.6.7.3.
- Corpus: 235 English SJIS `.org` files and 235 English UTF-8 `.org` files
  compiled with the Steam `Gameexe.ini` and `reallive.kfn`; the 235 French
  `.org` files were scanned for the same function/opcode shapes.
- Status: user gameplay validation on 2026-05-26; static compiler audit passed
  for both English corpuses with no compile failures. Extraction audit updated
  on 2026-05-28: the previously preserved Steam raw opcode warnings are fixed.

Steam-specific audit notes:

| Area | Evidence | Status |
| --- | --- | --- |
| Entry/kidoku marker | `Gameexe.ini` provides `KIDOKU_TYPE`; interpreter version is 1.6.7.3. | fixed and validated in game |
| Steam interpreter detection | `SiglusEngine_Steam.exe` accepted by CLI and GUI interpreter lookup. | fixed |
| Raw control textout | `SEEN9600` contains three preserved `raw #ff #01 endraw` stubs. | expected, must be retained |
| Raw op fallback | The previous seven targeted `op<...>` fallbacks are eliminated: two `Shl` calls now have KFN signatures and five encoded `###PRINT` markers now become text resources. | fixed |
| Resource byte escapes | Resource strings containing raw byte escapes no longer create empty quote runs. | fixed |
| String argument separators | Unquoted string arguments after another argument/operator receive a separator; quoted ASCII strings are unchanged. | fixed |
| Internal quoted strings | English select strings such as `Say "Hello."` keep their internal quotes instead of splitting the argument too early. | fixed |
| Unary-minus argument split | Late Steam calls whose bytecode `argc` proves the previous expression swallowed a following negative argument are split back into the expected argument list. | fixed |

Findings fixed from CLANNAD Steam debugging:

- Steam builds must read `KIDOKU_TYPE` from `Gameexe.ini` and feed it into
  bytecode generation; relying only on the default marker can desynchronise the
  entry table and crash the engine.
- `SiglusEngine_Steam.exe` is a valid RealLive-compatible interpreter source for
  PE version extraction.
- Default RealLive generation version is 1.2.7.0 when no interpreter or explicit
  target version is provided.
- `raw #ff #01 endraw` textout stubs in `SEEN9600` are intentional bytecode and
  must not be dropped.
- String parameters that compile to unquoted bytes need comma separators when
  they are not the first emitted parameter.
- Encoded command-level `###PRINT` markers can appear as ASCII `###PRINT(` or
  as the CLANNAD Steam byte pattern `###PR 01 00 T(`; both are textout markers,
  not unknown opcodes.
- Some late Steam scripts encode adjacent negative arguments in a shape that can
  be parsed greedily as subtraction. When the bytecode argument count proves
  this happened, split the unary-minus expression back out as the next argument.
- Native RealLive debug sources need `kprl -g` / `#line`; `flag.ini` only labels
  variables for the F5 flag window and is not a scene/source index.

## CLANNAD Side Stories Steam 2011

- Interpreter: `RealLiveEn.exe`, RealLive file version 1.6.6.8.
- Corpus: 22 Steam `.org` files extracted from the original `SEEN.TXT` with
  `Gameexe.ini`, `RealLiveEn.exe`, and `reallive.kfn`.
- Status: user gameplay validation on 2026-05-26 and 2026-05-27; both metadata
  and no-metadata rebuilt archives launch and run past the first story start.

Side Stories audit notes:

| Area | Evidence | Status |
| --- | --- | --- |
| Story bootstrap | `SEEN0001` calls `SEEN2000` before the first voice line. | validated in game |
| `gosub_with` pointer | `SEEN2000` contains `intC[1] = gosub_with(...) @14` and `intL[0] = gosub_with(...) @14`. | fixed; payload byte-identical |
| EOF trailer | Side Stories files keep `eof` followed by raw `halt`. | fixed and preserved |
| Entrypoint table | Unassigned entrypoints mirror the default entrypoint instead of staying zero. | fixed |
| Resource indexing | Go extraction keeps `*Bo\shake{2}` and `nk*` as separate resources in `SEEN0001`; `.org` and `.utf` files must stay from the same extraction/indexing pass. | documented |
| Round-trip audit | 17 of 22 files are payload-identical to the original; the remaining 5 redump stably and differ only in known string/resource serialization shapes. | accepted |

Findings fixed from CLANNAD Side Stories debugging:

- KFN hints containing `(store goto)` must set the generic goto-pointer flag.
  Without this, `gosub_with` leaves its trailing 4-byte pointer in the stream;
  the disassembler then emits bogus `halt` commands and a detached `= store`,
  corrupting the early story bootstrap.
- The compiler parser must allow trailing labels on `gosub_with`/`GOSUBP`, so
  `intC[1] = gosub_with(...) @14` recompiles to the original pointer form.
- Source-level `eof` must be preserved instead of stopping parsing, and a final
  raw `halt` after the `SeenEnd` trailer must remain present.
- Unassigned RealLive entrypoint slots should be filled from entrypoint 0, or
  the first defined entrypoint when slot 0 is absent.
- Do not mix OCaml-fused `.utf` resources with Go-extracted `.org` files unless
  the resource indices have been checked. `SEEN0001` has a known split around
  `*Bo\shake{2}` / `nk*`; fusing it shifts every following text resource by one.

## Current compatibility rules

| Rule | Scope | Source |
| --- | --- | --- |
| `itoa_ws`, `itoa_s`, `itoa_w`, `itoa` with 3 encoded args use overload 0 before RealLive 1.2.9.0 and overload 1 from 1.2.9.0 onward. | Version-gated | CLANNAD 1.2.3.5, AIR 1.2.9.5 |
| `strsub` and `strrsub` with 3 encoded args use overload 1. | General until contradicted by a later corpus | CLANNAD 1.2.3.5 |
| Steam/late RealLive builds should use `KIDOKU_TYPE` from `Gameexe.ini` when present. | Version and game config gated | CLANNAD Steam 1.6.7.3 |
| `raw #ff #01 endraw` textout stubs are bytecode-preserving, not display text. | Known Steam case | CLANNAD Steam `SEEN9600` |
| Command-level `###PRINT` markers are textout markers, including CLANNAD Steam's encoded `###PR 01 00 T(` byte shape. | Steam/late RealLive files | CLANNAD Steam |
| Greedy subtraction parses may need repair when `argc` proves a unary-minus-started argument was swallowed by the prior expression. | Steam/late RealLive files | CLANNAD Steam |
| Unquoted string argument bytes need a separator after a prior argument/operator. | General string emission rule | CLANNAD Steam |
| KFN `(store goto)` functions carry a trailing pointer like ordinary goto/gosub calls. | General KFN rule | CLANNAD Side Stories `SEEN2000` |
| `eof` plus a following raw `halt` is a meaningful trailer shape and must be preserved. | Steam/late RealLive files | CLANNAD Side Stories |
| Unknown interpreter version keeps the normal KFN/prototype selection, with default generation version 1.2.7.0. | Safety fallback | Compiler policy |
| `kprl -g` emits source debug line mapping for the native RealLive debugger; `kprl -G` remains only the game-ID option. | Debug tooling | CLANNAD Steam debug audit |

## To expand

- Add each new title with interpreter version, corpus size, and any newly seen
  opcode signatures.
- When a later title contradicts a rule, split it by RealLive version range
  instead of making a game-specific exception first.
