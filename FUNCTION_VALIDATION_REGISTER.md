# RLdev2026-Go bytecode function validation register

Last update: 2026-06-22

This register only tracks RealLive bytecode function signatures, AVG32
instruction shapes, overload selection rules, and function-shaped bytecode
behaviours that have been validated against known-good corpuses. Resource
handling, G00 work, archive keys, GUI behaviour, and general project notes
belong elsewhere.

## Method

- Compile extracted `.org` or `.avg` sources with the target game's
  interpreter and `Gameexe.ini` when available.
- Disassemble compiled files with opcode annotations.
- Compare function opcodes, instruction shapes, overload ids, and argument
  counts against the original extracted files.
- Keep rules tied to interpreter family/version first, and to a game only when
  the corpus proves the behaviour is game-specific.

## Validated Opcode Signatures

### CLANNAD 2007 FV

- Interpreter: RealLive 1.2.3.5.
- Corpus: 242 files.
- Status: validated for string-return overload selection, GAN helper roundtrip,
  and `ShakeLayers` overload selection; user gameplay validation after
  `seen0415` line 0273 on 2026-06-19 and after `seen3422` line 0100 on
  2026-06-22.

| Function | Opcode | Expected overload | Count FR | Count JP | Status |
| --- | --- | ---: | ---: | ---: | --- |
| `strsub` | `1:010:00005` | 0 | 2 | 2 | matched |
| `itoa_s` | `1:010:00015` | 1 | 10 | 10 | matched |
| `itoa` | `1:010:00017` | 1 | 36 | 36 | matched |
| `strcpy` 3-arg form | `1:010:00000` | 1 | 6 | 6 | matched |
| `objOfFileGan` 4-arg form | `1:071:01003` | 1 | 2 | 2 | in-game validated |
| `objOfFileGan` 7-arg form | `1:071:01003` | 3 | 1 | 1 | in-game validated |
| `ShakeLayers_04101` | `1:012:04101` | 1 | 1 | ? | in-game validated |
| `ShakeLayers_04202` | `1:012:04202` | 1 | 1 | ? | compiler validated |

The `ShakeLayers` JP counts are pending a fresh JP corpus comparison.

Validated rule: RealLive 1.2.3.5 uses overload 1 for `itoa*` calls with
three encoded args; `strsub` with three encoded args uses overload 0.
For RealLive 1.1+ CLANNAD FV bytecode, opcode family `1:071:01003` must
disassemble through the version-gated `objOfFileGan` name. Reusing the old
pre-1.1 `objOfFileAnm` source name can compile the call as overload 0 and break
the SEEN9077 animation helper reached after `seen0415` line 0273.
KFN entries with only overload 1 defined must include an explicit empty
prototype for overload 0 before the overload-1 prototype. Otherwise the source
prototype can be compacted to overload 0; this broke the `SEEN3422`
`ShakeLayers_04101` animation reached after line 0100.

### AIR 1.02

- Interpreter: RealLive 1.2.9.5.
- Corpus: 96 files.
- Status: route start validated by user; static overload audit matched.

| Function | Opcode | Expected overload | Count FR | Count JP | Status |
| --- | --- | ---: | ---: | ---: | --- |
| `itoa` | `1:010:00017` | 1 | 10552 | 10552 | matched |
| `strcpy` | `1:010:00000` | 0 | 22199 | 22199 | matched |
| `strcat` | `1:010:00002` | 0 | 10552 | 10552 | matched |

| Function/bytecode shape | Evidence | Status |
| --- | --- | --- |
| Static textout followed by `pause` | `pause` immediately after a static textout must share the previous text/kidoku line instead of receiving a fresh debug line marker. | route start validated |

Validated rule: RealLive 1.2.9.5 uses overload 1 for `itoa` calls with
three encoded args. KFN return parameters such as `>str` are encoded as real
parameters before string assignment rewriting, e.g. `strS[0] = itoa(n, 2)`.
When a static textout is followed immediately by `pause`, the `pause` opcode
does not receive a separate source-line marker; the preceding text/kidoku line
covers that display step.

### CLANNAD Side Stories Steam 2011

- Interpreter: RealLive 1.6.6.8.
- Corpus: 22 files
- Status: user gameplay validation on 2026-05-26 and 2026-05-27.

| Function shape | Evidence | Status |
| --- | --- | --- |
| `gosub_with(...) @label` store pointer | `SEEN2000` contains `intC[1] = gosub_with(...) @14` and `intL[0] = gosub_with(...) @14`. | pointer payload matched |
| KFN `(store goto)` functions | Same `SEEN2000` corpus. | trailing pointer preserved |

Validated rule: KFN functions marked `(store goto)` carry the same trailing
pointer payload as ordinary goto/gosub calls.

### CLANNAD Steam 2015

- Interpreter: RealLive 1.6.7.3.
- Corpus: 235 files.
- Status: user gameplay validation on 2026-05-26; static compiler audit passed.

| Function/bytecode shape | Evidence | Status |
| --- | --- | --- |
| `Shl` KFN signatures | Previously emitted as raw fallback opcodes in Steam corpus. | fixed and compiled via KFN |
| Command-level `###PRINT` marker | CLANNAD Steam byte pattern `###PR 01 00 T(`. | decoded as textout marker |
| Raw textout stub | `SEEN9600` contains `raw #ff #01 endraw` stubs. | preserved as bytecode |
| Unary-minus argument split | Late Steam calls where bytecode `argc` proves a following negative argument was swallowed. | repaired to expected arg list |

Validated rule: late Steam bytecode can require argument-list repair when the
encoded `argc` contradicts a greedy subtraction parse.

### Tomoyo After 2005 18+ / 2010 / Steam 2011

- Interpreter-Tomoyo After 2005 18+ : RealLive 1.3.5.4
- Interpreter : RealLive 1.6.7.3
- Corpus: 2005 : 260 files
- Corpus: 2010 : 262 files
- Corpus: 2011 : 267 files

- Status: user gameplay validation on 2026-05-30; Tomoyo 2010 R5 OCaml-source
  roundtrip validated in game on 2026-06-06; legacy OCaml pause syntax
  validated for Go recompilation on 2026-06-11.

| Function/prototype shape | Evidence | Status |
| --- | --- | --- |
| KFN continuation prototypes | Multi-line prototypes such as `zentohan`. | parsed as one function definition |
| `DUMMYCHECK_DISC` argc mismatch | Old bytecode can report `argc = 1` while the KFN prototype has three args. | encoded argc is respected |
| Quoted arg followed by `$ff <int>` | Tomoyo/late KFN calls where `$` starts the next encoded argument. | argument boundary preserved |
| Legacy OCaml brace special params | Tomoyo 2010 R5 OCaml sources emit `index_series(..., {0, 10000, 0, 0})` and `GetSaveFlag(..., {intF[0], intL[10], 1})`; these must encode as KFN-selected `__special[2](...)` / `__special[0](...)`, not as ordinary tuples. | in-game R5 validated |
| Legacy OCaml inline special args | Tomoyo 2010 R5 `SEEN9032` emits bare `farcall_with(9042, 0, 0, 1)` / `gosub_with(0, 1000) @93` source args for KFN `special(0:#{intC}, 1:#{strC})+`; these must encode as inline `special<0>(...)` without parens. | menu/options/new-game path validated |
| Legacy OCaml attached pause control | Old sources can write `text\pnext` without a separating space or line split; this must emit the pause control opcode rather than a literal backslash/Yen glyph. | compiler compatibility validated |

Validated rule: old KFN/prototype mismatches must follow the encoded bytecode
argument count when it is more specific than the prototype. OCaml-origin
RealLive sources can also use legacy `special(...)` syntax: brace groups in a
KFN `PSpecial` slot select a parenthesised `__special[N](...)` case by arity and
type, while bare simple args in `#{...}` special slots select inline
`special<N>(...)` encoding. Attached legacy `\p` controls remain accepted so
old OCaml-origin source can be decompiled with Go and recompiled without manual
pause spacing fixes.

### Kanon 1999 / Kanon 1999 18+ AVG32

- Interpreter: AVG3217M / AVG3216M
- Corpus: Kanon 1999 all-age : 155 files
- Corpus: Kanon 1999 18+ : 157 files
- Status: user gameplay validation on 2026-06-02.

| Instruction shape | Evidence | Status |
| --- | --- | --- |
| `text_zenkaku` / `text_hankaku` top-level text | SJIS and UTF-8/WESTERN roundtrips, including French accent test text. | text preserved and rebuilds |
| `set_title([...])` formatted text | Kanon title/prologue resources extracted as editable `.utf` text. | editable and rebuilds |
| `choice(...)` / `choice2(...)` text lists | Kanon choice text extracted as editable `.utf` resources. | editable and rebuilds |
| Label and jump table targets | Full Kanon archive rebuild with recalculated offsets. | offsets rebuild cleanly |

Validated rule: AVG32 UTF-8/WESTERN output keeps Japanese text in SJIS,
maps validated Western accents through the configured font table, and emits
Latin-only dialogue as `text_hankaku` so spacing matches the AVG32 renderer.

### Little Busters! 2007

- Interpreter: RealLive 1.4.8.8
- Corpus: 438 files
- Status: user gameplay validation on 2026-06-02.

| Function/prototype shape | Evidence | Status |
| --- | --- | --- |
| `objBgOfFileAnm` overload id 2 | Little Busters! bytecode uses the pre-1.1 filename + animation-name + visible/x/y form. | KFN updated and roundtrip validated |
| `objBgOfFileAnm` shorter overloads | Same function also keeps the older one-arg and five-arg animation-name forms. | overload range preserved |

Validated rule: pre-1.1 RealLive `objBgOfFileAnm` accepts overload ids 0, 1,
and 2, including the filename + animation-name + visible/x/y form used by
Little Busters! 2007.

### Little Busters! EX 2008

- Interpreter: RealLive 1.5.2.4.
- Corpus: 350 `.org` files.
- Status: Audit V2 function pass on 2026-06-23.

| Function/opcode shape | Evidence | Status |
| --- | --- | --- |
| `__shk_00010` zero-arg shake opcode | `SEEN2814` contains opcode `1:013:00010,0` between `__shkzm` and `__shkud`. Historical Audit V1 compiles warned on unresolved `op<1:Shk:00010,0>`. | KFN updated and fresh extraction emits `__shk_00010` |

Validated rule: RealLive 1.5.2.4 can emit the no-argument shake opcode
`1:013:00010,0`; keep it as a named KFN entry so extracted `.org` files do not
fall back to raw `op<...>` syntax.

### Planetarian 2006

- Interpreter: RealLive 1.3.9.5.
- Corpus: 20 files
- Status: user gameplay validation on 2026-06-03; R1/R2 GUI options freeze
  fixed and user-validated on 2026-06-11.

| Function/bytecode shape | Evidence | Status |
| --- | --- | --- |
| `objBgOfFileGan` short form | Planetarian uses the same encoded opcode family as `objBgOfFileAnm`, but the three-argument bytecode shape matches the GAN prototype. | selected by encoded argc and validated |
| Compact RealLive line markers | Non-`-g` Planetarian extraction emits `{- line N -}` for bytecode line markers. | recompiles byte-identical to validated `-g` corpus |
| Compact kidoku line table markers | Non-`-g` extraction emits `{- kidoku N line L -}` so the original read-flag line table values survive recompilation. | table matched value-for-value |
| `select_w` item separators | Planetarian route select blocks require the original logical line number on each item separator, not the physical `.org` line. | preserved by compact line comments |
| Omitted nested argument slots | `SEEN9034` contains tuple shapes such as `InitExFrames((0, , -880, ...))`; the empty slot must remain an omitted slot, not literal `0`. | opcode redump matched |
| Unquoted ASCII string parameters | GUI scripts contain calls such as `objOfFile(0, SIROS)` / `strcmp(strS[n], NONE)` where the bytecode requires a comma before an unquoted ASCII string after a previous argument. | GUI R1/R2 validated |
| `CCOM_LOCAL_FLAG_EXCOPY(str, str)` | `SEEN9040` original bytecode uses opcode `0:004:02000,3` for the string-pair form, although the KFN internal prototype index is the third defined prototype. | GUI R1/R2 validated |

Validated rule: RealLive roundtrips that hide full debug sources must still
preserve bytecode line markers and kidoku line-table values when the source is
used for recompilation. Compact line comments update the compiler's logical
line while suppressing physical source-line injection.
Planetarian also proves that empty tuple slots are bytecode separators, not
zero literals; same-arity overloads must be type-checked before falling back to
argument count; and unquoted ASCII string arguments need an explicit separator
when they follow another argument.

### Kud Wafter 2010 18+

- Interpreter: RealLive 1.6.3.4.
- Corpus: 62 files.
- Status: user in-game validation on 2026-06-03. Full SJIS roundtrip compiled
  with 0 errors and 0 warnings.

| Function/bytecode shape | Evidence | Status |
| --- | --- | --- |
| `objOfFileGan` overload id 2 | Kud Wafter uses opcode `1:071:01003, 2` with `filename`, `ganname`, `visible`, `x`, and `y`, e.g. `objOfFileGan (153, '_NYED_CT00_02', 'NYED_CT00_02', 1, 0, 0)`. | KFN updated and roundtrip validated |
| Adjacent KFN `strC` parameters | `objOfFileGan` contains consecutive `strC 'filename'` and `strC 'ganname'` arguments that must remain two quoted source parameters. | source recompiles |
| Nested special parameter groups | `TIMETABLE2` / `TIMETABLELEN2` use nested forms such as `special<48>(__special[1](...))` inside variadic special-parameter lists. | nested argument counts preserved |

Validated rule: RealLive KFN functions can contain consecutive string
parameters that must be split by prototype position, not by byte adjacency.
Variadic `special<N>` parameters can also wrap explicit nested `__special[M]`
calls; the compiler must emit the inner special call with its own argument
count before the outer special parameter is emitted.

## Compatibility Rules

| Rule | Scope | Source |
| --- | --- | --- |
| `itoa_ws`, `itoa_s`, `itoa_w`, and `itoa` with three encoded args use overload 1 in the validated RealLive 1.2.3.5 and 1.2.9.5 corpuses. | Validated RealLive corpuses | CLANNAD 1.2.3.5, AIR 1.2.9.5 |
| `strsub` and `strrsub` with three encoded args use overload 0; four encoded args use overload 1. | General until contradicted | CLANNAD 1.2.3.5 |
| KFN return parameters such as `>str` are emitted as real function parameters before assignment rewriting. | General KFN rule | AIR 1.02 |
| KFN `(store goto)` functions carry a trailing pointer payload. | General KFN rule | CLANNAD Side Stories `SEEN2000` |
| Encoded `argc` can override ambiguous source parsing when a greedy expression parse swallows a following argument. | Late RealLive/Steam | CLANNAD Steam |
| Old KFN calls may have fewer encoded args than the modern prototype; honour the original encoded `argc`. | Old bytecode/KFN mismatch | Tomoyo `DUMMYCHECK_DISC` |
| AVG32 Latin-only WESTERN dialogue is emitted as `text_hankaku`; Japanese dialogue remains in the native text form. | AVG32/Kanon | Kanon 1999 all-age / 18+ |
| pre-1.1 `objBgOfFileAnm` accepts overload id 2 for filename + animation-name + visible/x/y. | RealLive pre-1.1 | Little Busters! 2007 |
| Same-opcode RealLive function families can require prototype selection by encoded argument count before falling back to the exact overload name. | RealLive KFN overload aliases | Planetarian 2006, Little Busters! 2007 |
| RealLive `1:013:00010,0` is a zero-argument shake opcode and must remain named in KFN. | RealLive Shk module | Little Busters! EX 2008 |
| Non-`-g` RealLive extraction must preserve bytecode line markers and kidoku line-table values when sources are intended for recompilation. | RealLive debug/kidoku bytecode | Planetarian 2006 |
| `objOfFileGan` accepts overload id 2 for filename + GAN-name + visible/x/y. | RealLive KFN GAN functions | Kud Wafter 2010 18+ |
| Consecutive KFN string parameters must remain distinct source arguments even when their encoded bytes are adjacent. | RealLive KFN string arguments | Kud Wafter 2010 18+ |
| Variadic `special<N>` parameters may wrap nested `__special[M]` calls; each special group keeps its own encoded argument count. | RealLive special parameters | Kud Wafter 2010 18+ |
| A `pause` immediately following static textout does not receive a fresh debug line marker. | RealLive debug-line bytecode | AIR 1.02 |
| Legacy OCaml brace groups in KFN `special(...)` slots select parenthesised `__special[N](...)` by special-case arity/type, not ordinary tuple encoding. | RealLive special parameters | Tomoyo After 2010 R5 |
| Legacy OCaml bare args in `special(0:#{intC}, 1:#{strC})+` slots select inline `special<N>(...)` encoding without parens. | RealLive inline special parameters | Tomoyo After 2010 R5 |
| Legacy attached `\p` pause controls remain accepted and emit the pause opcode even when glued to following text. | RealLive text control bytecode | Tomoyo After / OCaml-source compatibility |
| KFN `ver ... end` blocks must be applied during disassembly; RealLive 1.1+ uses `objOfFileGan` / `objBgOfFileGan` names where pre-1.1 sources used `objOfFileAnm` / `objBgOfFileAnm`. | Version-gated RealLive KFN aliases | CLANNAD FV 2007 |
| Empty tuple/parameter slots are preserved as omitted bytecode separators instead of being compiled as literal zero. | RealLive complex parameters | Planetarian 2006 |
| Same-arity overloads with different argument types are selected by full parameter type when the source expression type is known. | RealLive overload selection | Planetarian 2006 |
| `CCOM_LOCAL_FLAG_EXCOPY` string-pair form encodes as overload byte `3` for opcode `0:004:02000`. | RealLive system CCOM | Planetarian 2006 `SEEN9040` |
| Unquoted ASCII string expressions following another argument require an argument separator; native non-ASCII string bytecode keeps historical adjacency. | RealLive argument serialization | Planetarian 2006 GUI scripts |

## To Expand

- Add only validated bytecode function signatures or function-shaped bytecode
  behaviours.
- Include interpreter version, corpus size, opcode/function shape, expected
  overload or encoded argument rule, and validation status.
- Put resource, archive, GUI, and image-format notes in separate documents.
