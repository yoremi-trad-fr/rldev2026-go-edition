# Changelog

This file tracks the RLdev2026 Go Edition beta history and the compatibility
fixes validated during the project sessions.

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
