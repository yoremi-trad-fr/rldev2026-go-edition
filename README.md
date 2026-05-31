### Status update: `31/05/2026`
<table>
  <tr>
    <td align="center" width="100%">
      <h2>Rldev2026-Go édition + GUI</h2>
    </td>
  </tr>
</table>

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
      <h3>Full supported VNs</h3>
    </td>
  </tr>
</table>


### -Clannad (2004)
### -Clannad Full Voice (2007)
### -Clannad Steam (2015) 
### -Clannad Side Stories Steam (2011)
### -AIR 18+ (2005)
### -Tomoyo After 18+ (2005)
### -Tomoyo After Memorial Edition (2010)
### -Tomoyo After-Steam (2011)
### -Oni Uta (not tested, Kotsuider contribution)
### -Royal Nekomimi Academy (not tested, CarouselAether contribution)

<table>
  <tr>
    <td align="center" width="100%">
      <h3>Wanted</h3>
    </td>
  </tr>
</table>


### Missing original ISOs: I’m looking for these ISOs for my testing.
#### ( If you own the physical version of the game, you could also create an ISO from it – that would be a huge help!)

1-Clannad Side Stories (non steam)

<table>
  <tr>
    <td align="center" width="100%">
      <h3>To do list VN + Others</h3>
    </td>
  </tr>
</table>

### Kanon (1999 AVG)

### Kanon (1999 18+ AVG)

### AIR (2000 18+)

### Little Busters! (2007)

### Little Busters EX ! (2008)

### Kud Wafter (2010 18+)

### Harmonia 2016

### Harmonia 2016-Steam

### Harmonia 2021-Steam HD édition

### Planetarian (2006)

- Beta 2.8 completed: improved `.g00` compatibility, XML metadata mode (`-m`), GUI batch processing, and G00 version 2 multi-region roundtrip support
- Support for GAN workflow
- Support for Babel module (for old version of ReaLlive)


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


cd "GUI Sources-Linux"

cd frontend
npm install
cd ..

wails build -clean -tags webkit2_41

<table>
  <tr>
    <td align="center" width="100%">
      <h3>Development Roadmap</h3>
    </td>
  </tr>
</table>


### Phase 1

* Port the `rldev2026` OCaml fork to Go
* Reach feature parity with the OCaml implementation
* Preserve compatibility with existing workflows

### Phase 2

* Add support for titles released after the original OCaml Rldev implementation
* Improve engine compatibility and tooling
* Expand modern platform support
*Tool for alignement utf Ocaml / GO

---

### Future Direction
