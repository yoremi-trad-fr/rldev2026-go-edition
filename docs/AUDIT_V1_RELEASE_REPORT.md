# Audit V1 release report

Date: 2026-06-11

Scope: this document summarizes the compatibility work done during the Audit V1
sessions, including this session and the earlier `Corriger bugs audit v1`
thread. It is release-oriented: the goal was to stabilize roundtrip behaviour
before leaving the beta line for v1.0.0.

## Status

- Automated tests: `go test ./rlc/... ./kprl/...` passed.
- Planetarian R1/R2 GUI options freeze: fixed and user-validated.
- CLANNAD Steam R1/R2 / Dangopedia issues: confirmed resolved during the audit.
- Tomoyo After OCaml-source compatibility: improved for legacy special params,
  attached `\p`, and speaker-name/resource handling.
- Little Busters: final local user-side check remains the only stated pending
  manual validation item.

## RealLive compiler fixes

### Legacy OCaml source compatibility

- Added compatibility for old attached pause controls such as `phrase\pfin`.
  The compiler now treats the glued `\p` as a pause control opcode and keeps
  the following text as text, instead of compiling a visible Yen/backslash glyph.
- Parsed KFN `special(...)` case definitions instead of discarding their
  details. This enables old OCaml-style source to be recompiled safely:
  brace groups in `PSpecial` slots are coerced into `__special[N](...)`, and
  bare inline special args are coerced into `special<N>(...)` without parens.
- Added type/arity selection for those legacy special cases, covering Tomoyo
  patterns such as `index_series(..., {0, 10000, 0, 0})`,
  `GetSaveFlag(..., {intF[0], intL[10], 1})`, and `farcall_with(...)` /
  `gosub_with(...)` inline special parameters.

### Argument serialization

- Added `OmittedExpr` for intentionally empty argument slots. Empty tuple slots
  now emit only their separator commas, matching bytecode shapes such as
  `InitExFrames((0, , -880, ...))`.
- Fixed comma insertion before unquoted ASCII string expressions after another
  argument. Planetarian GUI scripts need this for calls like
  `objOfFile(0, SIROS)` and string comparisons against unquoted ASCII labels.
- Kept native non-ASCII string adjacency unchanged, avoiding regressions in
  legacy Japanese bytecode argument shapes.
- Classified `#res<>` references as string literals for KFN type selection.

### Overload selection

- Improved overload selection for functions whose overloads have the same
  arity but different argument types. The compiler now checks full parameter
  types when the source expression type is known.
- Added a targeted Planetarian compatibility mapping for
  `CCOM_LOCAL_FLAG_EXCOPY(str, str)`: opcode `0:004:02000` must encode overload
  byte `3`, matching the original `SEEN9040` bytecode.

### Resource textout

- Fixed standalone `#res<>` textout compilation for plain Japanese resources:
  text stays bare when the original bytecode is bare.
- Fixed simple speaker resources such as `\{ゆめみ}「...」` so the whole textout
  stays in the original bytecode shape instead of gaining extra quote runs.
- Kept resource string arguments quoted where RealLive expects a string
  argument, for example title/menu resource arguments.

## RealLive disassembler fixes

- Speaker-name tags are decoded more carefully under UTF-8/WESTERN output.
  Native Japanese names that contain CP932 bytes resembling Western accent
  prefixes no longer turn into mojibake.
- Accented speaker names remain compatible with WESTERN extraction, while
  clearly Japanese names prefer the native CP932 decode.

## Archive rebuild fixes

- RealLive archive rebuilds with a template now preserve header metadata such as
  `#val_0x2c` when the replacement file does not explicitly carry it.
- Template archive trailers continue to be preserved.

## Validation evidence

### Planetarian 2006

- Recompiled R1/R2 fully after the fixes.
- Compared the R1 GUI/system scripts that exist in the original archive against
  the original RealLive bytecode after Go compile and re-disassembly.
- Normalized opcode comparison showed zero diffs for:
  `SEEN1000`, `SEEN1003`, `SEEN9030`, `SEEN9031`, `SEEN9032`, `SEEN9033`,
  `SEEN9034`, `SEEN9035`, `SEEN9036`, `SEEN9040`, `SEEN9050`, `SEEN9051`,
  and `SEEN9900`.
- `SEEN0001` differs because it is translated/script content, not a GUI/system
  bytecode reference.
- User confirmed the R1/R2 GUI options freeze is fixed in game.

### Tomoyo After

- Legacy OCaml-origin source forms are now accepted on the Go compiler path.
- Previously fixed R5 behaviour remains covered by tests for legacy special
  params and inline special args.
- Attached `\p` compatibility is covered by compiler tests and is intended for
  old OCaml-extracted scripts that used glued pause tags.

### CLANNAD Steam

- R1/R2 / Dangopedia issues were confirmed resolved during the audit after the
  shared compiler fixes.
- No additional game-specific CLANNAD Steam workaround was needed in this pass.

## Potential retest impact

These changes are intentionally compatibility-oriented, but they touch shared
RealLive paths. The following areas are worth spot-testing if a title uses the
shape described.

Static scans over the Audit V1 round folders refined the likely impact:

- `CCOM_LOCAL_FLAG_EXCOPY(str, str)` was found only in Planetarian among the
  audited corpus. Little Busters and Little Busters EX use the int-only forms
  (`intF[1261]` / `intF[1262]`), so the Planetarian string-pair overload byte
  mapping is not expected to affect them.
- `\p` pause controls are widespread across CLANNAD, Tomoyo After, Planetarian,
  Little Busters, Little Busters EX, AIR, and Kud Wafter. The new behaviour is
  permissive legacy support for glued OCaml-style `\p`; spaced and split modern
  forms are intended to remain unchanged.
- Accented speaker tags were found mainly in Tomoyo After and CLANNAD French
  sources. The disassembler change is expected to improve re-extraction of
  French names with accents while keeping native Japanese names readable.
- Planetarian remains the confirmed corpus for omitted nested tuple slots such
  as `InitExFrames((0, , -880, ...))`; the broad scan did not expose another
  audited title with that exact empty-slot shape.

| Area | Potentially affected titles | Risk | Suggested check |
| --- | --- | --- | --- |
| Same-arity overload selection by type | Any RealLive title with one opcode and int/string overloads | Medium, intended improvement | Menus/options and save/load paths that call system CCOM or mixed int/string functions |
| `CCOM_LOCAL_FLAG_EXCOPY(str, str)` overload byte `3` | Probably Planetarian-specific, possibly other RealLive 1.3-era titles | Low to medium | Any title that uses `CCOM_LOCAL_FLAG_EXCOPY` with string variables |
| Bare Japanese `#res<>` textout | Planetarian confirmed; possible in other JP RealLive scripts | Medium | GUI/menu text that is emitted as standalone `#res<>` |
| Speaker-name decode under WESTERN output | French patches with `\{Name}` tags and Japanese names under UTF-8/WESTERN extraction | Low, intended fix | Decompile/recompile a few files with accented and Japanese speaker tags |
| Empty tuple slots | Planetarian confirmed; any script with `(..., , ...)` | Low, intended fix | Scripts using `InitExFrames`, `ReadExFrames`, or similar nested tuple calls |
| Archive template header metadata | RealLive archive rebuilds using `-template` | Low, intended preservation | Rebuild archives for titles with non-zero `#val_0x2c` and verify boot/menu |
| Legacy `\p` glued to text | Old OCaml-origin scripts | Low, intended compatibility | Recompile an old OCaml-extracted script without manually spacing every `\p` |
| Little Busters pending check | Little Busters / Little Busters EX | Low to medium | Since these corpuses contain many resource references, include GUI/options/save-load and resource-heavy scenes in the final manual pass |

AVG32/Kanon paths were not directly changed by the compiler fixes above. The
only shared code touched in this pass is general archive/disassembly plumbing,
so Kanon risk is considered low.

## Files most relevant to the audit

- `rlc/pkg/compilerframe/compilerframe.go`: legacy `\p`, special-param
  coercion, omitted tuple slots, overload selection, Planetarian overload byte,
  resource textout handling.
- `rlc/pkg/codegen/codegen.go`: resource string argument quoting and ASCII
  string separator detection.
- `rlc/pkg/parser/parser.go` and `rlc/pkg/ast/ast.go`: omitted expression slots.
- `rlc/pkg/kfn/kfn.go`: KFN special-definition parsing.
- `rlc/pkg/function/function.go`: `#res<>` string classification.
- `kprl/pkg/disasm/writer.go`: speaker-name decoding under WESTERN output.
- `kprl/pkg/kprl/archiver.go`: template header metadata preservation.
- `FUNCTION_VALIDATION_REGISTER.md`: bytecode-level validation rules.
- `CHANGELOG.md`: v1.0.0 release summary.
