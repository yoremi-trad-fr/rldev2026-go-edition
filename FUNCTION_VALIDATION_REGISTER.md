# RLdev2026-Go bytecode function validation register

Last update: 2026-05-31

This register only tracks RealLive bytecode function signatures, overload
selection rules, and function-shaped bytecode behaviours that have been
validated against known-good corpuses. Resource handling, G00 work, archive
keys, GUI behaviour, and general project notes belong elsewhere.

## Method

- Compile extracted `.org` sources with the target game's interpreter and
  `Gameexe.ini` when available.
- Disassemble compiled files with opcode annotations.
- Compare function opcodes, overload ids, and argument counts against the
  original extracted files.
- Keep rules tied to RealLive/interpreter version first, and to a game only
  when the corpus proves the behaviour is game-specific.

## Validated Opcode Signatures

### CLANNAD 2007 FV

- Interpreter: RealLive 1.2.3.5.
- Corpus: 242 FR files and 242 JP SJIS files.
- Status: validated for string-return overload selection.

| Function | Opcode | Expected overload | Count FR | Count JP | Status |
| --- | --- | ---: | ---: | ---: | --- |
| `strsub` | `1:010:00005` | 1 | 2 | 2 | matched |
| `itoa_s` | `1:010:00015` | 0 | 10 | 10 | matched |
| `itoa` | `1:010:00017` | 0 | 36 | 36 | matched |
| `strcpy` 3-arg form | `1:010:00000` | 1 | 6 | 6 | matched |

Validated rule: RealLive 1.2.3.5 uses overload 0 for `itoa*` calls with
three encoded args; `strsub` with three encoded args uses overload 1.

### AIR 1.02

- Interpreter: RealLive 1.2.9.5.
- Corpus: 96 FR UTF-8 files and 96 JP SJIS files.
- Status: route start validated by user; static overload audit matched.

| Function | Opcode | Expected overload | Count FR | Count JP | Status |
| --- | --- | ---: | ---: | ---: | --- |
| `itoa` | `1:010:00017` | 1 | 10552 | 10552 | matched |
| `strcpy` | `1:010:00000` | 0 | 22199 | 22199 | matched |
| `strcat` | `1:010:00002` | 0 | 10552 | 10552 | matched |

Validated rule: RealLive 1.2.9.5 uses overload 1 for `itoa` calls with
three encoded args. KFN return parameters such as `>str` are encoded as real
parameters before string assignment rewriting, e.g. `strS[0] = itoa(n, 2)`.

### CLANNAD Side Stories Steam 2011

- Interpreter: `RealLiveEn.exe`, RealLive file version 1.6.6.8.
- Corpus: 22 Steam `.org` files extracted from the original `SEEN.TXT`.
- Status: user gameplay validation on 2026-05-26 and 2026-05-27.

| Function shape | Evidence | Status |
| --- | --- | --- |
| `gosub_with(...) @label` store pointer | `SEEN2000` contains `intC[1] = gosub_with(...) @14` and `intL[0] = gosub_with(...) @14`. | pointer payload matched |
| KFN `(store goto)` functions | Same `SEEN2000` corpus. | trailing pointer preserved |

Validated rule: KFN functions marked `(store goto)` carry the same trailing
pointer payload as ordinary goto/gosub calls.

### CLANNAD Steam 2015

- Interpreter: `SiglusEngine_Steam.exe`, RealLive file version 1.6.7.3.
- Corpus: 235 English SJIS `.org` files and 235 English UTF-8 `.org` files.
- Status: user gameplay validation on 2026-05-26; static compiler audit passed.

| Function/bytecode shape | Evidence | Status |
| --- | --- | --- |
| `Shl` KFN signatures | Previously emitted as raw fallback opcodes in Steam corpus. | fixed and compiled via KFN |
| Command-level `###PRINT` marker | CLANNAD Steam byte pattern `###PR 01 00 T(`. | decoded as textout marker |
| Raw textout stub | `SEEN9600` contains `raw #ff #01 endraw` stubs. | preserved as bytecode |
| Unary-minus argument split | Late Steam calls where bytecode `argc` proves a following negative argument was swallowed. | repaired to expected arg list |

Validated rule: late Steam bytecode can require argument-list repair when the
encoded `argc` contradicts a greedy subtraction parse.

### Tomoyo After 2010 / Steam 2011

- Interpreters: Tomoyo After Memorial Edition 2010 JP and Tomoyo After Steam
  2011 English.
- Corpus: JP 2010 native SJIS, Steam 2011 native UTF-8, FR 2010 patch, and
  FR 2011 patch.
- Status: user gameplay validation on 2026-05-30.

| Function/prototype shape | Evidence | Status |
| --- | --- | --- |
| KFN continuation prototypes | Multi-line prototypes such as `zentohan`. | parsed as one function definition |
| `DUMMYCHECK_DISC` argc mismatch | Old bytecode can report `argc = 1` while the KFN prototype has three args. | encoded argc is respected |
| Quoted arg followed by `$ff <int>` | Tomoyo/late KFN calls where `$` starts the next encoded argument. | argument boundary preserved |

Validated rule: old KFN/prototype mismatches must follow the encoded bytecode
argument count when it is more specific than the prototype.

## Compatibility Rules

| Rule | Scope | Source |
| --- | --- | --- |
| `itoa_ws`, `itoa_s`, `itoa_w`, and `itoa` with three encoded args use overload 0 before RealLive 1.2.9.0 and overload 1 from RealLive 1.2.9.0 onward. | Version-gated | CLANNAD 1.2.3.5, AIR 1.2.9.5 |
| `strsub` and `strrsub` with three encoded args use overload 1. | General until contradicted | CLANNAD 1.2.3.5 |
| KFN return parameters such as `>str` are emitted as real function parameters before assignment rewriting. | General KFN rule | AIR 1.02 |
| KFN `(store goto)` functions carry a trailing pointer payload. | General KFN rule | CLANNAD Side Stories `SEEN2000` |
| Encoded `argc` can override ambiguous source parsing when a greedy expression parse swallows a following argument. | Late RealLive/Steam | CLANNAD Steam |
| Old KFN calls may have fewer encoded args than the modern prototype; honour the original encoded `argc`. | Old bytecode/KFN mismatch | Tomoyo `DUMMYCHECK_DISC` |

## To Expand

- Add only validated bytecode function signatures or function-shaped bytecode
  behaviours.
- Include interpreter version, corpus size, opcode/function shape, expected
  overload or encoded argument rule, and validation status.
- Put resource, archive, GUI, and image-format notes in separate documents.
