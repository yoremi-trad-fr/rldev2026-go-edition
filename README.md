### Status update: `11/06/2026`
<table>
  <tr>
    <td align="center" width="100%">
      <h2>Rldev2026-Go édition + GUI</h2>
    </td>
  </tr>
</table>

Update : 11/06/2026 : v1.0.0 closes the Audit V1 compatibility pass, fixes Planetarian GUI R1/R2, improves OCaml <-> Go source compatibility, and documents the release validation notes

Update : 04/06/2026 : Beta 3.1 adds bonus workflows: GAN XML roundtrip, NWA audio export, DAT CGM/TCC JSON editing, and experimental Babel GUI support

Update : 03/06/2026 : Beta 3.0 validates Planetarian (2006) and Kud Wafter (2010 18+)

Update : 02/06/2026 : Beta 2.9 adds Kanon 1999 all-age / 18+ AVG32 support and validates Little Busters! 2007

Update : 31/05/2026 : Beta 2.8 completes the Go `vaconv` G00 workflow with XML metadata, format 2 multi-region support, and GUI batch conversion

Update : 30/05/2026 : Beta 2.7 adds Tomoyo After 2010/2011 roundtrip support and merges optional game.cfg archive keys

Update : 28/05/2026 : Beta 2.6 adds RealLive debug-source extraction in the GUI and fixes the remaining CLANNAD Steam extraction opcode warnings

Update : 27/05/2026 : Clannad Side Stories Steam (2011) now supported

Update : 25/05/2026 : Oni uta + Royal Nekomimi Academy + Clannad Steam (2015) now supported

Update : 24/05/2026 : AIR is now supported - Creating a list of validated features

Update : 20/05/2026 : An initial version of the GUI has been created for Rldev2026
GUI update Best console log + full .log file
Fixes for the -x transform not included in the GUI
rldev2026-go now behaves in the same way as OCaml when it comes to handling encodings

<table>
  <tr>
    <td align="center" width="100%">
      <h2>Contribution en cours</h2>
    </td>
  </tr>
</table>

---



<table>
  <tr>
    <td align="center" width="100%">
      <h3>Full supported VNs & features</h3>
    </td>
  </tr>
</table>

## VN

### -Kanon (1999)

### -Kanon 18+ (1999)

### -Clannad (2004)

### -AIR 18+ (2005)

### -Tomoyo After 18+ (2005)

### -Clannad Full Voice (2007)

### -Little Busters! (2007)

### -Little Busters! EX (2008) - compile/rebuild ready, final in-game validation pending

### -Planetarian (2006)

### -Kud Wafter (2010 18+)

### -Tomoyo After Memorial Edition (2010)

### -Tomoyo After-Steam (2011)

### -Clannad Side Stories Steam (2011)

### -Clannad Steam (2015)

## NOT TESTED (contributions)

### -Oni Uta (Kotsuider contribution)

### -Royal Nekomimi Academy (CarouselAether contribution)


## Features

### -G00 workflow : Full format support (including XML), batch mode in GUI, G00 formats 1 and 2 (multi-layer)

### -List of files in SEEN.txt

### -Raw bytecode extraction

### -Game ID (-G) key support and autocomplete for protected RealLive titles

### -Compact RealLive debug-line / kidoku preservation for normal roundtrips

### -Packed RealLive interpreter version detection

### -Extraction mode in the GUI for Reallive debug mode (parameter -g)

### -AVG32 / Kanon workflow: PACL/PACK archive list, extract, rebuild, TPC32 `.avg` disassembly, `.utf` resources, and `.avg` compilation

### -Kanon UTF-8 / WESTERN text workflow with French accent support

### -GAN workflow: `.gan` to `.ganxml` export and `.ganxml` to `.gan` rebuild through `rlxml`, with GUI actions

### -NWA audio workflow: `.nwa` BGM export to `.mp3` or `.wav` through `vaconv`, with GUI batch mode

### -DAT asset workflow: `mode.cgm` and `tcdata.tcc` export to editable JSON and rebuild back to binary

### -Babel workflow: runtime setup helper, `global.kh` helper, RealLive version override, and experimental `#load 'rlBabel'` compiler path for old RealLive translation work

### -BABEL release folder support: bundled runtime files are expected under `BABEL/rtl`



<table>
  <tr>
    <td align="center" width="100%">
      <h3>Wanted Games</h3>
    </td>
  </tr>
</table>




### Missing original ISOs: I’m looking for these ISOs for my testing.(or .zip archive, as long as I have the original game to run my in-game tests
, tests that are essential because compilations often produce invisible errors and cause the game to crash during play)
### ( If you own the physical version of the game, you could also create an ISO from it – that would be a huge help!)

###  Clannad Side Stories (non steam)



<table>
  <tr>
    <td align="center" width="100%">
      <h3>To do list VN + features </h3>
    </td>
  </tr>
</table>



## VN

### AIR (2000 18+)

### Harmonia 2016


## Features

### Full compatibility with files extracted using Rldev OCaml

### UTF-8 support for dialogues contained in .org files


<table>
  <tr>
    <td align="center" width="100%">
      <h3>Project Overview</h3>
    </td>
  </tr>
</table>


This project is a full port of the **Rldev2026** toolchain to the **Go language**.

The goal is to provide a modern and portable implementation capable of running natively on current operating systems without relying on outdated environments such as Cygwin or virtual machines.
A GUI is available; the aim is to make it easy for anyone to work with ReaLlive engine files for fan translations

Beta 2.6 adds an explicit extraction option for the native RealLive debugger:
`Sources debug RealLive (-g / #line)`. Keep it disabled for normal translation
sources, and enable it only when generating `.org` files for the in-game
F3/F5/O debug workflow. See `docs/debug-rl/README.md` for the concise debug
mode and `flag.ini` guide.

Beta 2.8 completes the Go `vaconv` path for the current G00 workflow:
format 0/1/2 extraction, format 2 XML metadata, PNG+XML import, and GUI batch
conversion in both directions. Format 2 files now preserve multi-region layout
metadata, while format 0 keeps the expected BGR channel order.

Beta 2.9 adds the first AVG32 target to the Go toolchain. Kanon 1999 all-age
and Kanon 1999 18+ now roundtrip through PACL/PACK extraction, TPC32 `.avg`
sources, editable `.utf` resources, and rebuild. SJIS and UTF-8/WESTERN
roundtrips were validated in game, including French accent output. The same
release also validates Little Busters! 2007 on the RealLive path with the
original disc/ISO workflow, without executable patching.

Beta 3.0 validates Planetarian 2006 and Kud Wafter 2010 18+ support.
Planetarian's normal, non-debug roundtrip now preserves compact RealLive line
and kidoku bytecode markers, so the rebuilt `SEEN.TXT` matches the validated
`-g` roundtrip without requiring debug-source extraction for translation work.
Kud Wafter 2010 18+ extracts with `Game ID (-G) = KUDO`, compiles against
RealLive `1.6.3.4`, rebuilds, and runs in-game with the same normal translation
workflow.

Beta 3.1 adds the optional asset and advanced translation workflows. `rlxml`
handles `GAN <-> GANXML` roundtrips. `vaconv` can export NWA BGM files to MP3
or WAV, and can export/import selected DAT-side assets (`mode.cgm` CG tables
and `tcdata.tcc` tone curves) through editable JSON. The GUI also includes an
experimental Babel tab for older RealLive translation projects: it prepares the
runtime DLL/map files, can update `GAMEEXE.INI`, writes a minimal `global.kh`
helper, and the compiler recognises `#load 'rlBabel'` without affecting the
normal text workflow.

### BABEL runtime folder

For releases, the Babel runtime pack should keep only the runtime files needed
by the GUI:

```text
BABEL/
  rtl/
    rlBabel.dll
    rlBabelF.dll
    1.2.3.5.map
    ...
    1.4.0.5.map
```

Historical helper folders such as `genmap` or standalone `rlbabel` tests are
not required by the integrated GUI workflow. Keep them in a separate advanced
tools archive if needed.

### Game ID (-G) for protected titles

`Game ID (-G)` selects the per-game XOR key used by `kprl` when protected
RealLive `SEEN.TXT` entries are extracted or disassembled. If a title is listed
below, use the exact ID in the GUI field named `Game ID (-G, optionnel)` or on
the command line, for example:

```bat
kprl16.exe -d -G KUDO -kfn KFN\reallive.kfn -e CP932 -o Ext-sjis SEEN.TXT
```

Without the matching `Game ID (-G)`, extraction may still produce files, but the
bytecode is decrypted with the wrong key and the resulting scripts can fail to
disassemble or recompile cleanly.

| Game ID | Title / edition |
| --- | --- |
| `CFV` | Clannad Full Voice |
| `LB` | Little Busters! |
| `LBEX` | Little Busters! EX |
| `LBME` | Little Busters! Memorial Edition |
| `LBPE` | Little Busters! PE |
| `FIVE` | 5 -Faibu- |
| `SNOW` | Snow Standard Edition |
| `KUDO` | Kud Wafter 18+ |
| `KUDA` | Kud Wafter all-ages |
| `PLHD` | Planetarian HD |
| `TMPE` | Tomoyo After PE / Memorial Edition |
| `ONIU` / `ONIUTA` | Oni Uta |
| `PING` | 3P LOVERS |
| `KOYO` | Nizuma Koyomi |
| `SHINO` | Nizuma Shino |
| `TAMA` | Nizuma Tamaki |
| `PRIP` | Princess Heart Link package edition |
| `PRID` | Princess Heart Link DL edition |
| `HINA` | Hinasawa Tomoka no Zettai Joousei |
| `LUV` | Lovedori Halation |


<table>
  <tr>
    <td align="center" width="100%">
      <h3>Building</h3>
    </td>
  </tr>
</table>


The command line tools build natively on Windows and Linux with Go 1.22 or newer.

Windows:

```bat
build-rldev.bat
```

Linux / Mint:

```bash
bash build-rldev.sh
```

The older script names are still kept as wrappers:

```bash
bash "build Binaires Rldev.sh"
```

For a Linux release folder from another OS:

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 OUTDIR=bin/linux-amd64 bash build-rldev.sh
```

Windows builds embed version metadata and an application manifest into the four CLI executables.


Build GUI Linux


```bash
cd "GUI Sources-Linux"
```
```bash
cd frontend
```

```bash
npm install
```

```bash
cd ..
```

```bash
wails build -clean -tags webkit2_41
```
