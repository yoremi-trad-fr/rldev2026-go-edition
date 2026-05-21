# Rldev2026-Go Port Status
Update : 20/05/2026 : An initial version of the GUI has been created for Rldev2026

GUI update Best console log + full .log file
Fixes for the -x transform not included in the GUI
rldev2026-go now behaves in the same way as OCaml when it comes to handling encodings

**Status update:** `20/05/2026`

Supported VNs
## -Clannad Full Voice (2007)
## -Clannad (2004)
---

Planned updates to the tools: Improved .g00 compatibility, support for version 2 

# To do list VN

## Kanon (1999 AVG)

* Decompilation UTF8/SJIS: Not implemented
* Intermediate compilation (`.org / .ke`): Not implemented
* Final compilation: Not implemented

---

## Kanon (1999 18+ AVG)

* Decompilation UTF8/SJIS: Not implemented
* Intermediate compilation (`.org / .ke`): Not implemented
* Final compilation: Not implemented

---

## AIR First Press Edition (2000 18+)

* Status: Not tested

* Decompilation UTF8/SJIS:
* Intermediate compilation (`.org / .ke`):
* Final compilation:

---

## AIR 1.02 (2005 18+)

* Decompilation UTF8/SJIS: Passed
* Intermediate compilation (`.org / .ke`): Passed
* Final compilation: Not tested

---

## Little Busters! (2007)

* Status: Not tested

* Decompilation UTF8/SJIS:
* Intermediate compilation (`.org / .ke`):
* Final compilation:

---

## Tomoyo After Memorial Edition (2010)

* Decompilation UTF8/SJIS: Passed
* Intermediate compilation (`.org / .ke`):  Passed
* Final compilation:  Failed

---

## Tomoyo After Steam Edition (2011)

* Decompilation UTF8/SJIS:  Passed
* Intermediate compilation (`.org / .ke`):Passed
* Final compilation:  Failed

---

## Clannad Side Stories Steam Edition (2011)

* Decompilation UTF8/SJIS: Passed
* Intermediate compilation (`.org / .ke`):  Passed
* Final compilation:  Failed

---

## Kud Wafter (2010 18+)

* Status: Not tested

* Decompilation UTF8/SJIS:
* Intermediate compilation (`.org / .ke`):
* Final compilation:

---
## Harmonia 2016

* Status: Not tested

* Decompilation UTF8/SJIS:
* Intermediate compilation (`.org / .ke`):
* Final compilation:
---

## Planetarian (2006)

* Status: Not tested

* Decompilation UTF8/SJIS:
* Intermediate compilation (`.org / .ke`):
* Final compilation:

---

# Project Overview

This project is a full port of the **Rldev2026** toolchain to the **Go language**.

The goal is to provide a modern and portable implementation capable of running natively on current operating systems without relying on outdated environments such as Cygwin or virtual machines.

---

# Building

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

---

# Development Roadmap

## Phase 1

* Port the `rldev2026` OCaml fork to Go
* Reach feature parity with the OCaml implementation
* Preserve compatibility with existing workflows

## Phase 2

* Add support for titles released after the original OCaml Rldev implementation
* Improve engine compatibility and tooling
* Expand modern platform support

---

# Future Direction

The Go implementation is intended to become the main actively maintained version of the project.

The legacy OCaml version will eventually be discontinued, and future updates will focus exclusively on the Go codebase.
