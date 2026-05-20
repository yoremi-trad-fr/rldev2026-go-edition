# Rldev2026-Go Port Status
Update : 19/05/2026 : An initial version of the GUI has been created for Rldev2026



**Status update:** `20/05/2026`

VN full supported : 
## Clannad Full Voice (2007)
---

# Supported Titles

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

## Clannad (2004)

* Decompilation UTF8/SJIS: Passed
* Intermediate compilation (`.org / .ke`):  Passed
* Final compilation: Failed

---

## Clannad Full Voice (2007) : OK

* Decompilation UTF8/SJIS: Passed
* Intermediate compilation (`.org / .ke`):  Passed
* Final compilation: Passed

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
