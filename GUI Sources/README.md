# RLdev 2026 - Go GUI

Interface Wails/Svelte dédiée à **RLdev 2026 - Go édition**.

La GUI expose uniquement les outils RLdev Go :

- KPRL : list, extract/disassemble, raw extract, rebuild SEEN.txt
- RLC : compile `.org` / `.ke`, simple ou batch
- Vaconv : G00 ↔ PNG
- RlXml : GAN ↔ XML

## Build

Prérequis : Go, Node.js, Wails CLI.

```bash
cd frontend
npm install
cd ..
wails build
```

Sortie attendue :

```text
build/bin/Rldev2026Go.exe
```

## Binaries attendus

Place les binaires CLI dans `bin/`, à côté de l'exe GUI :

```text
bin/kprl16.exe
bin/rlc2026.exe
bin/vaconv.exe
bin/rlxml.exe
bin/lib/reallive.kfn
```

`reallive.kfn` est auto-détecté dans :

```text
bin/lib/reallive.kfn
bin/reallive.kfn
```
