# Changelog

This file tracks the RLdev2026 Go Edition beta history and the compatibility
fixes validated during the project sessions.

## Beta 3.0 - 2026-06-03

Support added:

- Added Planetarian 2006 support, validated in game from the normal SJIS
  roundtrip path.
- Added Kud Wafter 2010 18+ support, validated in game from the normal SJIS
  roundtrip path with `Game ID (-G) = KUDO`.

Planetarian / RealLive updates:

- Preserved RealLive debug-line markers in normal extraction through compact
  `{- line N -}` comments, without enabling the full `-g` / `#line` source
  dump.
- Preserved read-flag markers and their original line table values through
  compact `{- kidoku N line L -}` comments.
- Taught `rlc` to consume the compact line/kidoku annotations, suppress
  physical `.org` line markers when compact bytecode lines are present, and
  rebuild the same bytecode as the validated `-g` roundtrip.
- Fixed overloaded KFN lookup for same-opcode RealLive function families by
  preferring prototypes that match the encoded argument count. This keeps
  Planetarian's short `objBgOfFileGan` form and Little Busters!' longer
  `objBgOfFileAnm` form on the same opcode family from being confused.

Kud Wafter / RealLive updates:

- Added and validated the `objOfFileGan` overload id 2 form used by Kud
  Wafter: filename + GAN-name + visible/x/y.
- Fixed disassembly of adjacent KFN `strC` parameters so consecutive strings
  such as `filename` and `ganname` remain separate source arguments.
- Fixed compilation of nested `special<N>(__special[M](...))` parameter groups
  used by Kud's `TIMETABLE2` / `TIMETABLELEN2` bytecode.
- Improved RealLive interpreter version detection for packed executables whose
  `VS_FIXEDFILEINFO` block sits outside the normal `.rsrc` raw section. Kud
  Wafter's `RealLive.exe` now resolves as version `1.6.3.4` instead of emitting
  the previous version-read warning.
- Added `Game ID (-G)` documentation and GUI autocomplete for protected
  RealLive titles.

## Beta 2.9 - 2026-06-02

Support added:

- Added Kanon 1999 all-age support for the AVG32/TPC32 engine, validated in
  game.
- Added Kanon 1999 18+ support for the AVG32/TPC32 engine, validated in game.
- Added Little Busters! 2007 support, validated in game with the original
  disc/ISO workflow and no executable patching.

AVG32 / Kanon updates:

- Added the AVG32 target to `kprl` with `-t AVG32` and the new
  `KFN/avg32.kfn` opcode table.
- Added PACL/PACK archive detection, listing, extraction, and rebuild support
  for Kanon archives.
- Added TPC32 `.avg` disassembly with paired `.utf` resources and lossless
  `#rawhex` preservation for unknown bytecode blocks.
- Added `.avg -> .TXT` assembly in the CLI and GUI, including single-file and
  batch compilation.
- Added editable AVG32 text coverage for top-level dialogue, `set_title`, and
  choice text.
- Fixed AVG32 UTF-8/WESTERN output so Japanese SJIS text is preserved, French
  accents use the validated Western mapping, Latin-only dialogue is emitted as
  `text_hankaku`, and generated `.utf` files use CRLF line endings.
- Validated SJIS and UTF-8 roundtrips for both Kanon 1999 versions.

Little Busters! 2007 updates:

- Added and validated the pre-1.1 `objBgOfFileAnm` overload shape used by the
  Little Busters! 2007 bytecode corpus.
- Validated the Little Busters! 2007 roundtrip and in-game launch path with an
  original disc/ISO setup.

GUI updates:

- The compile panel now accepts `.avg` sources in addition to `.org` and `.ke`.
- Batch compilation can include `.avg` files.
- Archive rebuild workflows accept `.TXT` and `.avg` inputs where appropriate.
- KFN selection remains required for RealLive `.org` / `.ke` compilation, but
  no longer blocks AVG32 `.avg` compilation.

Notes:

- AVG32 PACK rebuild currently uses valid literal recompression; rebuilt files
  can be larger than the originals.

## Beta 2.8 - 2026-05-31

G00 / `vaconv` updates:

- Completed the Go `vaconv` workflow for G00 format 0, 1, and 2 files.
- Added XML metadata support with `-m`, including PNG+XML import and automatic
  XML export for format 2 files.
- Added format 2 multi-region encode/decode support, compatible with modern
  G00 files that carry region layout metadata.
- Fixed format 0 BGR channel order so roundtripped background images keep their
  original colours.
- Fixed format 2 BGRA pixel handling and PNG alpha preservation for
  semi-transparent assets.
- Added focused G00 tests for format 0 colour order, PNG alpha handling,
  format 1 roundtrip, and format 2 BGRA/region roundtrip.

GUI updates:

- Added G00 batch conversion to the Windows and Linux GUI for both
  `G00 -> PNG` and `PNG -> G00`.
- Added XML metadata file/folder selection for G00 conversion workflows.
- Added a G00 format selector for PNG import: auto, v0, v1, and v2.
- Updated generated Wails bindings and frontend builds for the new GUI
  conversion signatures.

Compiler/disassembler maintenance:

- Improved KFN alias and overload handling for function families whose public
  name differs from the internal opcode name.
- Improved argument emission around complex/special parameters and unary
  negative values.
- Added parser/compiler support for `select(...)` as an expression where older
  extracted sources require it.

Documentation:

- Cleaned `FUNCTION_VALIDATION_REGISTER.md` so it only tracks validated
  bytecode function signatures and function-shaped bytecode behaviours.

## Beta 2.7 - 2026-05-30

Support and tooling updates:

- Added Tomoyo After roundtrip coverage for Memorial Edition 2010 JP, Steam
  2011 English, FR 2010, and FR 2011 scripts.
- Merged additional `game.cfg` archive keys into the Go game-key registry and
  `kprl -G` help text. These keys remain optional and are only used when a
  title actually needs encrypted SEEN decompression/recompression.
- Kept the old `rlc/compiler` scratch package out of normal Go builds; the
  active compiler remains `rlc/pkg/compilerframe` plus `rlc/pkg/function`, and
  `go test ./...` now passes for the `rlc` module.

Tomoyo After fixes:

- Fixed KFN prototype continuation parsing, covering multi-line overloads such
  as `zentohan`.
- Fixed extraction of legacy menu/select strings that begin with `-"..."`.
- Fixed old bytecode calls such as `DUMMYCHECK_DISC`, where the KFN declares
  three arguments but the opcode header reports one; the disassembler now uses
  the prototype and treats `$` after a quoted string as an argument boundary.

## Beta 2.6 - 2026-05-28

Support and tooling updates:

- Added an explicit GUI checkbox for RealLive debugger source extraction:
  `Sources debug RealLive (-g / #line)`.
- Kept normal extraction clean by default; debug `.org` generation is now an
  intentional secondary output for native F3/F5/O workflows.
- Added `docs/debug-rl/` with a concise RealLive debug-mode guide and a minimal
  `Flag.ini.example`.

CLANNAD Steam extraction fixes:

- Added missing CLANNAD Steam `Shl` signatures for `1:Shl:04101` and
  `1:Shl:04202`.
- Fixed command-level `###PRINT` markers, including the CLANNAD Steam encoded
  `###PR 01 00 T(` form, so they disassemble as text resources instead of raw
  `op<35:035:21072,...>` calls.
- Fixed quoted English `select`/string arguments that contain internal double
  quotes.
- Repaired late Steam argument parsing where bytecode argument counts show that
  a unary-minus expression starts the next argument, covering cases such as
  `objBgMove` and `InitFrame`.
- Validated a full verbose extraction of the 235-file CLANNAD Steam corpus with
  no remaining targeted raw opcode warnings.

## Beta 2.4 - 2026-05-27

Support added for:

- CLANNAD Side Stories Steam (2011), validated in game.

CLANNAD Side Stories fixes:

- Fixed `gosub_with` / `GOSUBP` disassembly for KFN `(store goto)` functions:
  their trailing 4-byte pointer is now read and emitted as a source label.
- Allowed `gosub_with (...) @label` to recompile, preserving the original
  pointer form in `SEEN2000`.
- Preserved source-level `eof` plus the final raw `halt` after the `SeenEnd`
  trailer.
- Filled unassigned RealLive entrypoint table slots from the default
  entrypoint, matching the original Side Stories headers.
- Fixed unary negative emission and parameter separation in calls that include
  `-1` style arguments.
- Audited the Side Stories corpus: 22 Steam files compile; `SEEN2000` is
  payload-identical after the `gosub_with` fix; 17/22 files are payload-identical
  overall and the remaining five redump stably.
- Documented the Side Stories `SEEN0001` resource-indexing pitfall: Go extraction
  keeps `*Bo\shake{2}` and `nk*` split across two resources, so translated `.utf`
  files must preserve the same indices as their paired `.org` extraction.

## Beta 2.3 - 2026-05-26

Support added for:

- CLANNAD Steam (2015), validated in game.
- Oni Uta, added from the Kotsuider contribution. Not locally validated yet.
- Royal Nekomimi Academy, added from the CarouselAether contribution. Not
  locally validated yet.

CLANNAD Steam fixes:

- Accepted `SiglusEngine_Steam.exe` as a RealLive-compatible interpreter in the
  compiler and GUI auto-detection paths.
- Extracted interpreter file version 1.6.7.3 from the Steam executable and used
  it for version-gated bytecode generation.
- Read `KIDOKU_TYPE` from `Gameexe.ini` and passed it to code generation so the
  entry/kidoku marker matches the Steam build.
- Changed the default RealLive generation version to 1.2.7.0 when no explicit
  interpreter or target version is available.
- Preserved CLANNAD Steam `SEEN9600` raw `FF 01` textout bytes as
  `raw #ff #01 endraw` instead of treating them as discardable display text.
- Fixed resource text compilation for raw byte escapes such as `\x{84}\x{02}`,
  avoiding empty quote runs that produced corrupt text streams.
- Fixed separator insertion for string parameters that compile to unquoted
  bytes, covering calls such as `SetLocalName`, `strcpy`, and `strcmp`.
- Kept quoted ASCII string arguments stable so the new separator logic does not
  alter already-safe strings.
- Audited the Steam `.org` corpus: 235 English SJIS files and 235 English UTF-8
  files compile cleanly with the Steam `Gameexe.ini`, `SiglusEngine_Steam.exe`,
  and `reallive.kfn`.

GUI and packaging fixes:

- Added the Linux GUI module to `go.work`.
- Renamed the Linux GUI module so Go workspace builds resolve it cleanly.
- Fixed Linux GUI tool lookup to use Linux binary names (`kprl16`, `rlc2026`,
  `vaconv`, `rlxml`) while still allowing Windows names on Windows.
- Kept GUI KFN auto-detection for `./KFN/reallive.kfn`, bundled `bin/lib`, and
  nearby project folders.
- Updated GUI interpreter lookup to include Steam/Siglus executables.
- Added the archive-template field to the Linux GUI rebuild workflow, matching
  the Windows GUI.

## Beta 2.2 - 2026-05-24

Support added for AIR 2005 18+ / AIR 1.02.

AIR fixes:

- Added the function validation register to track known-good opcode signatures.
- Validated AIR route start in game.
- Fixed KFN return-parameter injection before string assignment rewrites, so
  calls such as `strS[0] = itoa(n, 2)` encode the destination string parameter.
- Applied version-gated `itoa` overload selection: RealLive 1.2.9.5 uses
  overload 1 for the three-argument form.
- Avoided inserting an extra debug line marker on the `pause` immediately after
  static text output.
- Confirmed AIR `strcpy`, `strcat`, and `itoa` signature counts against the
  original corpus.

## Beta 2.1 - 2026-05-24

Added automatic KFN loading.

- CLI and GUI workflows can find `reallive.kfn` in common project and bundled
  locations instead of requiring the user to select it every time.
- Missing KFN files now produce explicit warnings because they reduce overload
  filtering and argument validation.
- GUI startup pre-fills the KFN path when a known copy is available.

## Beta 2 - 2026-05-23

GUI optimisation, bug fixes, and CLANNAD 2004 support.

- Added and refined the Wails GUI workflow for listing, extracting,
  disassembling, compiling, rebuilding archives, and image/animation helpers.
- Added batch `.org` compilation from the GUI.
- Added encoding and output transform controls to the GUI, including the
  `WESTERN` transform and force-transform option needed for translated text.
- Improved GUI logging and command visibility, including full batch logs.
- Added CLANNAD 2004 compatibility coverage.
- Fixed RealLive text encoding and transform handling so Go output follows the
  OCaml toolchain more closely.
- Improved archive rebuild/extract behaviour and SEEN file handling.

## Beta 1 - 2026-05

Initial supported baseline.

- CLANNAD Full Voice 2007 is the fully supported baseline title.
- Ported the RLdev toolchain to Go with command-line tools for RealLive script,
  archive, image, and animation workflows.
- Established CLANNAD 2007 function rules for `strsub`, `itoa_s`, `itoa`, and
  three-argument `strcpy`.
- Fixed legacy CLANNAD 2007 overload handling: RealLive 1.2.3.5 uses overload 0
  for the three-argument `itoa*` family.
- Fixed `strsub`/`strrsub` three-argument overload handling.
- Fixed inner ASCII double quote emission in string literals to match observed
  Shift-JIS output.
