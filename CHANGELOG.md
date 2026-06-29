# Changelog

This file tracks the RLdev2026 Go Edition beta history and the compatibility
fixes validated during the project sessions.

## v1.3.5 - 2026-06-29

Batch and GUI polish:

- Added `-jobs` to `rlsave map` and `rlsave doctor`, with automatic worker
  sizing for recursive save-folder scans.
- Added `-jobs` / `-j` to `vaconv` batch conversions so large image/audio/DAT
  folders can be processed in parallel.
- Made the Windows and Linux GUI console panel vertically resizable, which makes
  long wrapper output easier to inspect without leaving the app.
- Replaced project-specific save presets with generic `read.sav` progression,
  `save999.sav` global flag, and low-level dword profiles.
- Updated public save-editor examples so they no longer point at a private
  route/test value.

Validation:

- Automated checks passed for `rlsave`, `vaconv`, and the touched GUI frontends.
- Windows and Linux frontends rebuild successfully with the resizable console and
  generic save profiles.

## v1.3.4 - 2026-06-28

RealLive save tooling:

- Added the new `rlsave` command-line tool for RealLive `.sav` files.
- `rlsave info` decodes compressed save bodies and reports save kind, header
  size, compressed size, and uncompressed size.
- `rlsave get`, `rlsave set`, and `rlsave dump` support `AVG_GLOBAL_SAVE` /
  `save999.sav` global `intG` values, with timestamped backups before writes.
- `rlsave map` recursively inventories save folders and classifies compressed
  game/global saves, raw `read.sav` progression saves, and raw system saves.
- `rlsave export -lossless` writes editable text exports, including `seen[n]`
  progression entries for `read.sav`; `rlsave build` rebuilds `.sav` files from
  those lossless exports after edits.
- `rlsave get/set` now also support `seen[n]` / `seenNNNN` aliases for
  `read.sav` and low-level `dword[n]` body entries.
- Added `rlsave diff` to compare two saves and report changed `intG`, `seen`,
  or body dword entries.
- Added `rlsave doctor` to run a non-destructive structural diagnostic on one
  save or a save folder.
- `read.sav` text exports now annotate `seen[n]` entries with the matching
  `seenNNNN` / `seenNNNN.org` script hint.
- Integrated the save editor into the Windows and Linux Wails GUIs with Info,
  Map, Doctor, Diff, Get, Dump, Set, Export, and Build actions, still using
  `rlsave` as a wrapper backend; the panel also includes quick profiles for
  common `read.sav` / `save999.sav` edits.
- Regular game slots can be inspected and exported for now; their named
  variable banks remain unmapped until the per-slot layout is decoded safely.

Validation:

- Confirmed the supplied CLANNAD 2007 `save999.sav` global flags can be read
  through `rlsave`, including representative route/progression counters.
- Mapped the supplied multi-game save corpus across AIR, CLANNAD, Kud Wafter,
  Little Busters!, Planetarian, and Tomoyo After with no unsupported save kind.
- Confirmed CLANNAD `read.sav` exposes editable `seen[n]` progression entries
  and that setting one to zero on a temporary copy preserves the save structure.
- Confirmed `rlsave diff` reports a temporary-copy `seen[n]` edit correctly,
  and `rlsave doctor` reports the original `read.sav` without warnings or errors.
- Verified a CLANNAD `read.sav` lossless export/rebuild roundtrip is
  byte-for-byte identical before edits.
- Built the Windows and Linux GUI frontends after adding the save editor panel.
- Automated checks passed for `kprl`, `rlsave`, and the Windows/Linux GUI Go
  packages.

## v1.3.3 - 2026-06-27

Post-audit RealLive re-extraction regression fixes:

- Fixed select-block disassembly for adjacent empty choices, `###PRINT("")`
  empty choices, quoted choice text containing commas or inner quotes, and
  conditional blank choices. These cases could desync re-extraction of SEEN
  archives compiled by the Go fork after Audit V1.
- Added a hard-coded `gosub_with` reader for opcode `<0:Jmp:00016, 0>`, so the
  trailing pointer is consumed correctly even when `kprl` is launched with an
  older/default KFN version that does not register the newer function name.
- Kept the compiler-side select fixes from the warning audit: missing select
  resources now compile as empty choices and quoted select labels escape inner
  quotes before bytecode emission.

Validation:

- Re-extracted the warning-producing files from Little Busters!, Little
  Busters! EX, Kud Wafter, Tomoyo After 2005, Tomoyo After Memorial Edition
  2010, and Tomoyo After Steam with no warnings on the targeted scenes.
- Automated checks passed for the full `kprl` and `rlc` Go packages.

## v1.3.2 - 2026-06-23

Audit v2 RealLive extraction compatibility:

- `kprl` now auto-detects the RealLive interpreter version next to `Seen.txt`
  before loading KFN definitions for disassembly, matching the compile-side
  behaviour already used by `rlc` and the GUI.
- The disassembler `-f` option can now be used as documented with an
  interpreter executable, game directory, or adjacent file path, not only a raw
  version string.
- Kept `gosub_with` / `farcall_with` / `ret_with` / `rtl_with` available for
  newer RealLive 1.6.x interpreters; Tomoyo After Steam reports `1.6.7.3` but
  still uses these opcodes.

Validation:

- Re-extracted the Audit v2 warning files for Little Busters, Little Busters
  EX, and Tomoyo After 2005/2010/Steam with no warnings and no truncated
  `.org` tails.
- Automated checks passed for `kprl` command version parsing and disassembler
  KFN selection.

## v1.3.1 - 2026-06-22

CLANNAD Full Voice 2007 compatibility:

- Fixed the CLANNAD FV 2007 `seen3422` animation after line 0100 by preserving
  overload 1 for `ShakeLayers_04101` / opcode `1:Shl:04101`.
- Corrected the KFN declarations for `ShakeLayers_04101` and
  `ShakeLayers_04202` so their single defined prototype remains at overload 1
  instead of being compacted to overload 0.
- `rlc` now rejects KFN function declarations whose prototype count does not
  match the declared overload range, preventing this class of silent overload
  skew.

Validation:

- User confirmed the `seen3422` after-line-0100 animation no longer freezes in
  game on 2026-06-22.
- Recompiled the supplied 242-file CLANNAD FV 2007 `seen` corpus after source
  encoding cleanup with 0 warnings and 0 errors.
- Automated checks passed for `rlc` KFN parsing and compiler-frame overload
  emission.

## v1.3.0 - 2026-06-19

CLANNAD Full Voice 2007 compatibility:

- Fixed the freeze after CLANNAD FV 2007 `seen0415` line 0273 by preserving
  the RealLive 1.2.3.5 GAN helper overload during full extract/rebuild
  roundtrips.
- `kprl` now honours KFN `ver ... end` blocks while disassembling. For
  RealLive 1.1+ targets, opcode family `1:071:01003` is emitted as
  `objOfFileGan` / `objBgOfFileGan` instead of the pre-1.1 `objOfFileAnm`
  aliases.
- The `kprl -f` RealLive version is now used for KFN version selection during
  disassembly, so `-f 1.2.3.5` extracts CLANNAD FV 2007 with the expected
  modern function names.
- `rlc` now refuses the pre-1.1 object animation aliases on modern RealLive
  versions instead of silently compiling them with the wrong overload id.

Source and GUI fixes:

- Fixed UTF-8/WESTERN `#character` name encoding so accented character-table
  entries survive a decompile/recompile roundtrip.
- Normal `.org` extraction hides compact line markers by default again, while
  `-g` / explicit line-info workflows can still preserve the markers needed
  for exact debug-line roundtrips.
- The Windows GUI now detects the RealLive interpreter version from the
  selected `GAMEEXE.INI` / executable path and auto-fills the compile version
  when the field is left in auto mode.

Validation:

- CLANNAD FV 2007 was validated in game after line 0273 by rebuilding from the
  prepared compiled file set and again after a fresh Japanese extraction.
- Automated checks passed for `kprl`, `rlc`, and the Windows GUI packages.

## v1.2.0 - 2026-06-18

GUI and workflow updates:

- Added `Extract text ORG` to the Windows GUI, with export/import modes and
  single-file or batch-folder processing for `.org` / `.ke` scripts.
- Added `rlc --text-export`, `rlc --text-import`, and `--text-file` for the
  same ORG/KE dialogue workflow from the command line.
- Exported text is written as editable `.utf` files and can be imported back
  into patched `.org` / `.ke` sources after translation.
- Files with no real dialogue text are skipped, so batch export does not create
  empty `.utf` files for purely structural scripts.

ORG/KE text handling:

- Added extraction of dialogue carried by `#res<...>` resource references,
  including Planetarian-style direct string mirrors.
- Added extraction/import of direct single-quoted dialogue literals such as the
  Tomoyo After `strS[...] = '...'` script text.
- Import can update direct script literals and, when resource-backed entries
  are present, writes the companion resource `.utf` file alongside the patched
  source.
- Resource references without an available resource file or direct text mirror
  are ignored instead of producing blank translation entries.

`vaconv` and GUI batch fixes:

- Added directory input expansion to `vaconv` through explicit input formats
  such as `-i g00 <folder>` and `-i png <folder>`.
- Updated Windows and Linux GUI G00 batch conversion to pass the source folder
  to `vaconv` instead of expanding every file path in the GUI process.
- Updated the Windows GUI asset batch calls that use `vaconv` to use the same
  safer folder-input mode where applicable.
- This avoids the Windows command-line/path-length failure seen when converting
  folders containing long G00 file names, even when the fork and assets are
  already placed near `C:\`.

Validation:

- Tested ORG text export/import on the provided Planetarian, Tomoyo After, and
  CLANNAD sample scripts.
- Planetarian `SEEN0001.org` exported 1042 real text entries and imported a
  modified entry back into both the `.org` mirror and companion `.utf`.
- Tomoyo After `SEEN7820.org` exported 814 direct script text entries and
  imported modified text back into the `.org`.
- CLANNAD `SEEN1002.org` only exported the one real available script string
  from the provided sample set; missing companion resource files no longer
  generate blank entries.
- Rebuilt `bin/rlc2026.exe`, `bin/vaconv.exe`, and the Windows Wails GUI.
- Automated checks passed for `rlc`, `vaconv`, the Windows GUI, and the Linux
  GUI packages.

## v1.0.0 - 2026-06-11

Release status:

- Closed the Audit V1 compatibility pass that prepares the project to leave
  beta. The tested table is green on the audited titles; the remaining manual
  user-side check is the Little Busters entry still marked for final local
  testing.
- Added `docs/AUDIT_V1_RELEASE_REPORT.md` with the cross-session audit record:
  fixes, validation evidence, and retest risk notes.

RealLive compatibility fixes:

- Fixed UTF-8/WESTERN extraction of speaker-name tags when a CP932 Japanese
  name contains bytes that can be mistaken for Western accent prefixes.
- Preserved accented speaker names in UTF-8/WESTERN roundtrips while still
  keeping native Japanese speaker names readable.
- Added compatibility for legacy OCaml `\p` pause controls glued to following
  text, so old sources such as `phrase\pfin` recompile as a pause opcode
  instead of displaying a Yen/backslash glyph.
- Added `OmittedExpr` support so empty tuple slots such as
  `InitExFrames((0, , -880, ...))` remain omitted bytecode separators instead
  of becoming literal zeroes.
- Fixed function argument serialization before unquoted ASCII string
  parameters, covering Planetarian GUI calls such as `objOfFile(0, SIROS)` and
  string comparisons against unquoted ASCII labels.
- Improved same-arity overload selection by checking source argument types
  before falling back to argument count.
- Fixed Planetarian `CCOM_LOCAL_FLAG_EXCOPY(str, str)` to emit opcode
  `0:004:02000,3`, matching the original `SEEN9040` bytecode.
- Parsed KFN `special(...)` case definitions and used them to coerce legacy
  OCaml brace/simple special parameters into the correct RealLive
  `__special[N](...)` / `special<N>(...)` encodings.

Resource and textout fixes:

- Fixed Planetarian resource textout compilation for plain Japanese resources
  and simple `\{name}body` speaker resources so the bytecode stays bare where
  the original RealLive scripts are bare.
- Kept `#res<>` string arguments quoted where RealLive expects string
  arguments, including Japanese title/menu resource arguments.
- Classified resource references as string literals for KFN type selection.

Archive/rebuild fixes:

- Preserved template SEEN header metadata such as `#val_0x2c` during archive
  rebuilds, preventing known-good original metadata from being zeroed when the
  compiled replacement file does not carry it.
- Continued preserving RealLive archive trailers from the template archive.

Validation:

- Planetarian R1/R2 GUI options freeze fixed and user-validated.
- Planetarian GUI/system scripts present in the original archive redump with
  zero normalized opcode diffs after Go compile/re-extract, excluding the
  translated main script where text differs by design.
- Tomoyo After legacy OCaml-source compatibility improved for special params
  and attached pause controls.
- CLANNAD Steam R1/R2 and Dangopedia regressions were confirmed resolved during
  the wider audit.
- Full automated suite passed with `go test ./rlc/... ./kprl/...`.

## Beta 3.1 - 2026-06-04

Support and workflow updates:

- Added Little Busters! EX 2008 compile/rebuild readiness checks. Final in-game
  validation remains pending, but it is no longer part of the main remaining
  blocker list.
- Added the GAN workflow to the GUI through `rlxml`: `.gan -> .ganxml` export
  and `.ganxml -> .gan` rebuild.
- Added NWA BGM export to `vaconv` and the GUI, with MP3 and WAV output modes
  plus batch conversion for `.nwa` folders.
- Added selected DAT asset editing through JSON: `mode.cgm` CG tables and
  `tcdata.tcc` tone curves can be exported, edited, and rebuilt.
- Added an experimental Babel workflow for old RealLive translation projects:
  runtime setup, DLL/map copy, optional `GAMEEXE.INI` update, and a minimal
  `global.kh` helper from the GUI.

Babel compiler/runtime updates:

- Added Go compiler support for `#load 'rlBabel'`, gated behind the explicit
  Babel load so the normal text workflow remains unchanged.
- Added runtime text output through `CallDLL`, including `rlBabelF.dll` support
  for pre-1.2.5 RealLive versions.
- Added support for text resources referenced by `#res<>`, including CLANNAD
  style name markers such as `\m{A}` and `\m{B}`.
- Added a RealLive target-version override in the GUI compile panel, useful for
  old executables such as CLANNAD 2004 where version detection can be
  ambiguous.
- Added `docs/BABEL_BEGINNER_GUIDE.md` with a beginner workflow and CLANNAD
  2004 notes.

Known Babel limits:

- Gloss/ruby interactive behaviour is not fully ported yet; base text is
  compiled and warnings are emitted.
- Babel is opt-in. Scripts without `#load 'rlBabel'` continue to use the normal
  RealLive text compiler path.

Packaging notes:

- The release Babel pack only needs `BABEL/rtl` with `rlBabel.dll`,
  `rlBabelF.dll`, and the bundled `.map` files. Historical `genmap` and
  standalone `rlbabel` helper folders can stay out of the main release archive.

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
