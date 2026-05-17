# rldev2026-go — Changelog

Travail effectué durant les sessions de portage Phase 1 (mai 2026).

---

## v2026.2-go.7 — Désassembleur : grammaire d'arguments complète

### kprl/pkg/disasm

**Handler `select_s` (Module 2 / Sel)** — l'opcode `select_s` (et toute la
famille `select_w` / `select_s2` / `select_cancel` / `select_msgcancel`)
n'avait aucun handler côté Go ; le bloc `{...}` suivant le header opcode
n'était pas consommé. Conséquence : tout le bytecode qui suit était
mésinterprété, et en particulier les bytes `0x23 0x23 0x23 0x50 0x52 0x49
0x4e 0x54` (= `###PRINT(` en ASCII) apparaissaient comme un opcode header
fantôme `op<35:035:21072, 84>`. Implémenté `readSelect()` pragmatique
(suit OCaml `read_select` disassembler.ml L1870), gère le `(expr)`
optionnel pour les variantes `select_w`, le bloc `{...}` avec `argc`
items, et dispatch chaque item via `GetDataSep` qui reconnaît
`###PRINT(<expr>)` → `\s{<expr>}` ou `\i{<expr>}` inline. Le rendu produit
maintenant `select_s(#res<NNNN>, #res<NNNN>)` avec `<NNNN> \s{strS[1011]}`
dans le `.utf`, identique à OCaml.

Résultat : élimination des 24 warnings `op<35:035:21072>` Clannad +
restauration du round-trip sur SEEN1002-1009 et SEEN9999.

**Complex paren `((tuple)+)` — parser via `GetData`** — pour les opcodes
variadiques à tuples comme `InitExFrames <1:Sys:00620, 0>
(('counter', 'limit', 'limit2', 'time')+)`, le port Go consommait le
contenu via un compteur naïf `(`/`)` byte-par-byte. Or les bytes `0x28`
et `0x29` peuvent apparaître à l'intérieur d'expressions int32 immédiates
(`$ ff XX XX XX XX`), ce qui désynchronisait le compteur et lisait des
octets de trop. Remplacé par une boucle qui dispatche chaque datum via
`GetDataSep` (la même grammaire que le reste). Mirroir OCaml
`read_complex_param` L2244.

Résultat : SEEN9079 passe de 162 lignes tronquées à 411 lignes complètes,
SEEN9033 plus de désync.

**Disambiguation `(` arg-level via flag `complexForm`** — un `(` au
top-level d'un argument peut signifier soit un complex tuple (premier
byte du arg block est `(`, suivi immédiatement de `(`) soit une
expression parenthésée (`(800 - 180) / 2`). Sans info KFN
discriminante (le proto Go ne préserve pas `complex`/`special`), on
détecte à l'entrée de `readFuncArgsCtx` : si la byte qui suit le `(`
initial est elle-même `(`, on est en complex form et chaque `(` rencontré
est structurel ; sinon, un `(` rencontré dans la boucle est une
expression dispatchée via `GetDataSep` → `GetExpression`.

Résultat : SEEN9041 et autres expressions parenthésées avec opérateurs
arith trailing (`(a-b)/2`) parsées correctement.

**Suppression du faux warning `mismatch +N`** — les opcodes variadiques
(`+` dans le KFN, ex. `grpMulti`, `setarray`, `InitExFrames`) ont
nominalement plus d'args que l'argc bytecode. OCaml tolère silencieusement
ce dépassement. Changement de la condition `len(args) != argc` →
`len(args) < argc`.

Résultat : tous les warnings `Grp:00075 expects 2, got 4` éliminés.

**Mismatch `got < argc` déclassé en `diag.Phase`** — typique de proto
avec params `Fake (=)` non comptés dans l'argc bytecode (ex. `Sys:00401`
overload 1). Affiché seulement avec `-v`.

**Warnings enrichis** — `diag.SysWarning` pour les erreurs de désassemblage
inclut désormais : nom du fichier SEEN, offset hexadécimal, **opcode
formaté `op<T:M:F, O>`** avec nom de module KFN, et **dump hex 8 bytes
avant/après** l'offset fatal pour `disassembly aborted`.

**Marqueur `###PRINT(` dans `GetDataSep`** — défense en profondeur : un
`#` au début d'un arg qui matche `###PRINT(` est routé vers
`readStringUnquot` même hors contexte `select_s` (cas hypothétique non
observé en pratique mais valide selon OCaml).

---

## v2026.2-go.6 — Système diag : extracteur kprl

### kprl/pkg/disasm, kprl/cmd/kprl, kprl/pkg/kprl

**`disasm.Options.SourceFile`** — propagation du nom de fichier SEEN
dans le reader pour que les diagnostics internes puissent l'inclure.
Mis en place par `disassembleFile` (single file) et la boucle
`disassembleArchive` (un SEEN à la fois).

**Main loop disassembly** — l'erreur `disassembly error at offset` était
émise via `fmt.Printf` (stdout) uniquement avec `-v`. Désormais routée
via `diag.SysWarning` (toujours visible, stderr) au format OCaml. Un
context-dump hex automatique vient en complément.

**Argc mismatch** — anciennement marqué « Soft warning only » dans le
code mais émettant rien. Désormais émet un warning structuré.

**archiver `ReadFullHeader` fail** — était `fmt.Printf` (stdout, polluant
le listing d'archive). Routé via `diag.SysWarning`.

**kprl `main()`** — wire `diag.SetVerbose(*verbose > 0)`. Tous les
`fmt.Fprintf(os.Stderr, ...)` orphelins (KFN load fail, decompress fail,
disassemble fail, write fail) migrés vers `diag.SysWarning` ou
`diag.Phase`. KFN absent maintenant **toujours signalé** (n'était visible
qu'avec `-v` avant).

---

## v2026.2-go.5 — Système diag : codegen / compilerframe

### rlc/pkg/compilerframe, rlc/pkg/directive, rlc/cmd/rlc

**`compilerframe.error/warning`** (37 callers) — anciennement append-only
dans les slices `c.Errors`/`c.Warnings`, flushées par `main.go` au format
`warning: %s` minuscule. Désormais émet aussi en temps réel via
`diag.Warning`/`diag.Errorf` au format OCaml `(file line N): msg`. Les
slices restent pour `HasErrors()` et tests.

**`directive.error/warning`** (idem) — même traitement. `#print` passe
aussi par `diag.Info`.

**`main.go` flush des slices** — supprimé (duplicate avec diag).

**`loadGameexe` / `loadKfn` absents** — anciennement masqués sans `-v`.
Désormais signalés via `diag.SysWarning` avec un message explicite sur
l'impact :
- `KFN file "X" not found — opcodes will use raw op<…> form, no overload filtering`
- `unable to locate gameexe.ini, using defaults`

**Phase tracing** — les `if Verbose>0 { Fprintf }` éparpillés (Compiling,
GAMEEXE entries, KFN functions, Detected interpreter, Output, Reading
INI/KFN) unifiés en `diag.Phase`.

---

## v2026.2-go.4 — Système diag : encodage sortant texttransforms

### kprl/pkg/texttransforms, rlc/pkg/codegen

**`BadRunes() []rune` exposé** — liste distincte ordonnée des runes que
le batch encoder n'a pas pu représenter, depuis le dernier
`ResetBadChars()`. Anciennement `badChars` était une `sync.Map` privée
inutilisable par les callers.

**`noteBadRune(r)` câblé** dans toutes les branches de perte de
caractère : `toSJSBytecode`, `encodeWestern`, `encodeChinese` (2 sites),
`encodeKorean` (2 sites). Reset+notation systématiques.

**`toSJSBytecode` rune-par-rune** sur erreur du batch encoder — au lieu de
l'erreur opaque qui s'arrêtait au premier byte invalide, on identifie
**chaque** rune unmappable en un seul passage. Sous `ForceEncode`,
substitution par espace inchangée.

**`Output.encodeText(loc, s)`** — nouveau wrapper dans codegen : reset →
encode → émet 1 `diag.Warning` par bad rune unique avec le `Loc` source
exact. Câblé aux 3 callsites `ToBytecode` (top-level resource, TextToken,
inline ResRef). Message identique à OCaml : `cannot represent U+XXXX "x"
in RealLive bytecode`.

Résultat : un caractère traduit qui n'existe pas en CP932 (apostrophe
typographique U+2019, em-dash U+2014, etc.) est désormais signalé avec
sa position exacte dans le .org au lieu d'être silencieusement substitué.

---

## v2026.2-go.3 — Système diag : décodage source et lexer

### rlc/cmd/rlc, rlc/pkg/lexer

**`decodeSource` scan up-front** — pour les sources UTF-8, un balayage
byte-par-byte signale chaque byte invalide avec sa ligne. Typique : un
stray Shift-JIS dans un .org UTF-8.

**BOM UTF-8** — détecté et stripé en début de fichier, avec un warning
explicite (`UTF-8 BOM stripped at start of file — some interpreters
reject it`).

**U+FFFD résiduel après décodage** — scan post-décodage qui pointe
chaque substitute character à sa ligne.

**Lexer `Skip unknown`** — l'ex-branche silencieuse devient
`diag.WarnAt(file, line, "skipping U+FFFD replacement character…")` ou
`"skipping unrecognised character U+XXXX 'x'"`. Le translator voit
exactement le caractère perdu.

**Phase tracing** — `lexing %s (%d bytes, encoding %s)`, `parsed %d
statement(s)`, `generated %d bytes of bytecode (version X.Y.Z.W)`.

---

## v2026.2-go.2 — Système diag : câblage CLI rlc

### rlc/cmd/rlc

**Flag `-Wfatal` / `--warnings-fatal`** ajouté à `Options`.

**`main()`** — configuration process-wide `diag.SetQuiet/SetVerbose/
SetWarningsFatal` après parse des flags.

**`compileFile`** — `diag.Reset()` au début (per-fichier), `diag.Summary`
+ abort sur `diag.Errors() > 0` à la fin.

**Warning interpreter ad-hoc** — `fmt.Fprintf(os.Stderr, "Warning: ...")`
migré vers `diag.SysWarning`.

---

## v2026.2-go.1 — Socle pkg/diag

### kprl/pkg/diag (nouveau)

Package partagé importable depuis tous les outils via
`github.com/yoremi/rldev-go/pkg/diag`.

**Localisé** (OCaml `keTypes.ml`) :
- `Warning(loc, format, ...)` → `Warning (file line N): msg.`
- `Errorf(loc, format, ...) error` → `Error (file line N): msg`
- `Info(loc, format, ...)` → `file line N: msg`
- Variantes `WarnAt(file, line, ...)`, `ErrorAt(...)`, `InfoAt(...)` pour
  callers qui n'ont pas un `Loc` sous la main.

**Global** (OCaml `optpp.ml`) :
- `SysWarning(format, ...)` → `Warning: msg.`
- `SysError(format, ...) error` → `Error: msg`
- `SysInfo(format, ...)` → `msg`

**Tracing** : `Phase(format, ...)` — affiché uniquement avec
`SetVerbose(true)`, préfixé de 2 espaces (= les sub-phases d'OCaml).

**Contrôle** : `SetOutput(w)`, `SetQuiet(bool)`, `SetVerbose(bool)`,
`SetWarningsFatal(bool)`, `Reset()`.

**Compteurs** : `Warnings()`, `Errors()`. Erreurs jamais silenciées
(`-q` ne masque pas les errors, comportement OCaml `cliError`).

**Bilan** : `Summary(file)` — `SEEN0001.org: 2 warning(s), 0 error(s)`.

8 tests dédiés, tous verts. Le package est placé dans `kprl/pkg/diag/`
plutôt que `common/pkg/diag/` pour des raisons de collision de module
go.work — voir le doc-comment en tête du fichier.

---

## v2026.2-go.0 — GUI : exe finder & SEEN.txt save

### GUI-Sources

**Champ « RealLive.exe »** ajouté à l'étape 3 (Compile) : permet à
l'utilisateur de désigner explicitement un `RealLive.exe` qui sera
utilisé pour extraire la version PE de l'interpréteur (impact sur le
marker kidoku `@`/`!` et le filtrage des overloads KFN). Priorité de
résolution de version : `--target-version` > `-I` > auto-détection à
côté du `.org`.

**Bouton « Output SEEN.txt » étape 4** — utilise maintenant
`SelectSaveFile` au lieu de `SelectFile`, permettant de taper un nouveau
nom au lieu d'écraser un SEEN existant.

Backend `rlc/cmd/rlc/main.go` : nouveau flag `-I` / `--interpreter`.
GUI `app.go` : signatures `RldevCompile`/`RldevCompileBatch` étendues
avec `interpreter`. Bindings wails régénérés.

---

# Récapitulatif par jeu RealLive

État avant cette session vs. maintenant, sur les jeux test
(`yoremi-trad-fr/Tests-works`) :

| Jeu | Avant | Maintenant | Notes |
|---|---|---|---|
| Clannad (japonais) | 60 warnings dont 1 abort fatal (SEEN9079 tronqué) | **0** | 242/242 SEEN extraits intégralement |
| AIR | non testé | **0** | |
| CSD | non testé | **0** | Clannad Side Stories |
| Tomoyo After | 7 warnings | **1** | reste : `gosub_with` avec `special(0:#{intC})+` variadique |
| Tomoyo After Steam | 5 warnings | **0** | |
| **Kanon** (AVG32) | format non géré | **non géré** | rldev OCaml ne le supporte pas non plus — Phase 2 (parser PACL + bytecode AVG32 à écrire) |

Le résidu Tomoyo (`SEEN7300` 1 warning) est un cas exotique nécessitant
`read_soft_function` complet (priorité 1 de notes Phase 2). Le mini-RPG
encodé dans les .org reste indisponible — comportement déjà présent dans
RLdev OCaml original selon les notes du translator.

---

# Tests Phase 1 restants

- [ ] **Compilation round-trip** : Clannad SEEN.txt extrait → modifié
  marginalement → recompilé → bytecode identique octet-pour-octet (ou
  validation runtime via boot du jeu)
- [ ] **vaconv** : conversions G00 ↔ PNG sur quelques fonds Clannad
- [ ] **rlxml** : conversions GAN ↔ XML sur quelques animations
- [ ] Système de log Babel (plugin) si pertinent
