# Audit du MANUEL_RLDEV2026_GO.md vs. état réel du code

État du repo audité : `yoremi-trad-fr/rldev2026-go-edition` à la fin
de la session diag/désassembleur (mai 2026).

Légende :
- ✅ Manuel correct, rien à changer
- ⚠️ Manuel à corriger (détail/imprécision)
- ❌ Manuel faux (la fonction n'existe pas / fonctionne autrement)
- ➕ Fonctionnalité existe dans le code mais absente du manuel

---

## §1-2 Introduction, différences avec OCaml

✅ Globalement correct.

⚠️ « Les commandes CLI sont compatibles (mêmes flags) » — partiellement
vrai. Quelques flags ont une casse ou un nom différent (voir §5).
Reformuler en « les commandes CLI sont **largement** compatibles ».

---

## §3 Installation

✅ Structure recommandée OK.

⚠️ La commande `go build -o kprl16.exe ./kprl/cmd/kprl/` est correcte
mais l'exécutable réel s'appelle juste `kprl` (pas `kprl16`) — la doc
suppose qu'on renomme à l'install. Soit ajouter une note explicite,
soit harmoniser le nom de build par défaut.

➕ Aucune mention de la **GUI AIO** (`AIO Key game tools`) qui est
maintenant l'interface principale recommandée pour les translators
non-CLI. À ajouter à terme dans le manuel.

---

## §4 kprl16 — Archiveur et désassembleur

### §4.1 / §4.2 (-l, -x, -b, ranges)
✅ OK.

### §4.3 Désassemblage

✅ La forme `-d -e UTF-8 -o DIR SEEN.txt` fonctionne.

⚠️ **Flag `--kfn=` vs `-kfn`** — le manuel utilise `--kfn=reallive.kfn`
(double tiret + `=`), mais l'aide actuelle de kprl montre `-kfn FILE`
(simple tiret). Avec `flag.StringVar` du stdlib Go, les deux syntaxes
fonctionnent (`-kfn=val`, `--kfn=val`, `-kfn val`, `--kfn val`). Donc
la commande du manuel marche, mais l'aide générée ne le montre pas.
Pas critique mais à uniformiser dans la doc en cas de revue.

⚠️ Tableau des options de désassemblage :
- `-S` (« Séparer tout le texte japonais ») : présent ✅
- `-s` : présent ✅ (mais inversé : `don't separate text into resource file`)
- `-r` : nouveau, **non documenté dans le manuel** : `don't generate
  control codes`
- `-raw-strings` : nouveau, **non documenté** : `no special markup in strings`
- `-opcodes` : nouveau, **non documenté** : `show opcode annotations`
- `-hexdump` : nouveau, **non documenté** : `generate hex dump`
- `-y string` : nouveau, **non documenté** : `decoder key for compiler version 110002`
- `-cast string` : nouveau, **non documenté** : `cast of characters translation file`

➕ Ajouter au tableau les flags listés ci-dessus.

### §4.4 / §4.5 Archives, suppression, infos
✅ Conforme au code.

---

## §5 rlc2026 — Compilateur Kepago

### §5.1 Compilation basique
✅ Les commandes fonctionnent.

### §5.2 Transformation d'encodage

❌ **CRITIQUE — `-x CP1252` et `--force-transform` n'existent PAS dans
rlc Go.** Vérification :
```
$ rlc -h | grep -i transform
(rien)
$ rlc -h | grep -- " -x"
(rien)
```

Le rlc OCaml avait `-x ENC` (transform output) pour produire du
bytecode en CP1252/CP936/CP949 à partir d'un .org UTF-8 — c'est la
mécanique de base des patchs occidentaux. **Cette fonctionnalité n'est
pas portée**. Les translators qui veulent un patch français/anglais
ne peuvent actuellement pas produire un bytecode CP1252 via rlc Go.

Workaround actuel : passer l'encodage de sortie à `-e CP932` ou
`-e UTF-8` directement (lecture = écriture). Pour CP1252 il faut soit
l'implémenter (priorité haute pour les patches occidentaux), soit
encoder à la main avant compilation.

À ajouter au backlog de la phase 1.

### §5.3 Tableau des options

Vérifications flag par flag :

| Manuel | Réel | Statut |
|---|---|---|
| `-v` / `--verbose` | `-v` (int, repeat for more) | ⚠️ pas de `--verbose` |
| `-o FILE` / `--output` | `-o string` | ⚠️ pas de `--output` |
| `-d DIR` / `--outdir` | `-d string` | ⚠️ pas de `--outdir` |
| `-i FILE` / `--ini` | `-i string` + `-ini` + `-g` (alias) | ✅ |
| `-e ENC` / `--encoding` | `-e string` | ⚠️ pas de `--encoding` |
| `-x ENC` / `--transform-output` | **ABSENT** | ❌ |
| `--force-transform` | **ABSENT** | ❌ |
| `-K FILE` / `--kfn` | `-K string` | ⚠️ pas de `--kfn` long |
| `-t TGT` / `--target` | `-target string` | ⚠️ long form est `-target`, pas `--target` |
| `-f VER` / `--target-version` | `-target-version string` | ⚠️ pas de `-f` ! |
| `-G GID` / `--game` | `-id string` (default LB) | ❌ `-G` n'existe pas, c'est `-id` |
| `-u` / `--uncompressed` | `-compress` (default true) | ⚠️ inversé : `-compress=false` au lieu de `-u` |
| `-g` / `--no-debug` | `-debug-info` (default true) | ⚠️ inversé : `-debug-info=false`. Et `-g` est utilisé comme alias pour `-i` (GAMEEXE) ! |

➕ Flags présents dans rlc Go **non documentés dans le manuel** :
- `-I` / `--interpreter` : path to RealLive.exe pour extraire la
  version PE (ajouté cette session, voir CHANGELOG v2026.2-go.0)
- `-Wfatal` / `--warnings-fatal` : treat warnings as errors (cette
  session, v2026.2-go.2)
- `-q` : quiet mode (cette session)
- `-O int` : niveau d'optimisation (0|1|2, défaut 1)
- `-array-bounds` : runtime array bounds checking
- `-assertions` : enable runtime assertions
- `-cast` : cast file
- `-game` : game.cfg path (≠ `-id` qui est l'identifier)
- `-old-vars` : use old variable layout
- `-with-rtl` : include runtime library
- `-flag-labels` : flag labels in output
- `-metadata` : include metadata
- `-resdir` : resource directory
- `-runtime-trace` : runtime trace level
- `-src-ext` : source extension (default "org")
- `-start-line` / `-end-line` : compilation partielle

➕ **À ajouter au manuel** : section « Diagnostics » expliquant le
nouveau système de log :
- format `Warning (file line N): msg.`
- résumé par fichier `SEEN0001.org: N warning(s), M error(s)`
- comportement `-q` / `-v` / `-Wfatal`

---

## §6 vaconv

✅ Conforme au code pour les options présentes.

⚠️ Le manuel mentionne `-i png` pour PNG→G00 et `-f 0` pour spécifier
le format. Vérification :
- `-i string` (input format) : ✅
- `-f string` (G00 format, défaut "auto") : ✅
- `-o string` (output filename) : ✅
- `-d string` (output directory) : ✅
- `-v` : ✅

Tableau formats G00 (0, 1, 2) : non vérifié dans cette session (code
read-only). À tester en Phase 1 finale.

---

## §7 rlxml

✅ Très synthétique mais correct. Notez que rlxml Go n'a que `-o` et
`-v` comme options, ce qui correspond au manuel.

---

## §8 Workflow complet de traduction

⚠️ Étape 3 « Compilation » du workflow :
```bash
for %f in (scripts\SEEN*.org) do (
    rlc2026 -v -K reallive.kfn -e UTF-8 -d new_seens %f
)
```
Cette commande compile **chaque .org séparément**. Conforme au code,
mais à noter qu'il n'y a pas de mode batch interne à rlc (chaque appel
recharge le KFN). Pour la GUI AIO, le batch est géré par boucle externe.

⚠️ Étape 5 « Images optionnel » suppose que vaconv prend un PNG en
input direct : OK mais le mode bidirectionnel `-i png file.png -o
file.g00` doit être testé en pratique sur des fonds Clannad réels
(Phase 1 finale).

---

## §9 Migration depuis RLdev OCaml

Tableau de correspondance :

| OCaml | Manuel Go | Réel Go | Statut |
|---|---|---|---|
| `./kprl15.exe -v -f 1.5 -d -o ...` | `kprl16 -v -f 1.5 -d -o ...` | `kprl -v -f 1.5 -d -o ...` | ⚠️ binaire = `kprl`, pas `kprl16` |
| `-e cp932` | `-e CP932` | `-e CP932` | ✅ (sensible à la casse) |
| `-G FIVE` (rlc) | `-G FIVE` | **`-id FIVE`** | ❌ — c'est `-id`, pas `-G` |
| `-c 110002` (rlc) | `-c 110002` | **n'existe pas** | ❌ — fonctionnalité absente |
| `-x CP932 --force-transform` (rlc) | idem | **absent** | ❌ |
| `vaconv *.g00 -d g00` | `vaconv -d g00 *.g00` | OK | ✅ |
| `vaconv -i png -o xxx.g00 xxx.png` | OK | OK | ✅ |

### Flags renommés (manuel)

Le manuel dit « `-i FILE` (rlc) → `-i FILE` ou `-g FILE` ». Réel :
- `-i string` : GAMEEXE.INI path ✅
- `-g string` : GAMEEXE.INI path (alias for `-g`) ✅
- `-ini string` : alias ✅

Donc **3 manières** d'écrire la même chose. OK.

---

## §10 Jeux supportés

⚠️ Le tableau liste `LB, LBEX, LBME, CLAN, FIVE, CFV, SNOW, LBd`. Le
flag dans rlc est `-id` (pas `-G`), et l'aide n'affiche que la valeur
par défaut « LB ». Lister explicitement les valeurs supportées dans
l'aide du flag serait utile.

⚠️ kprl, lui, affiche bien `-G string game ID (LB, LBEX, CFV, FIVE,
SNOW)` dans son aide. Donc kprl utilise `-G` et rlc utilise `-id` —
**incohérence entre les deux outils**. Soit harmoniser (recommandé :
ajouter `-G` comme alias à rlc), soit documenter explicitement.

Encryptions `110002` / `1110002` : **mécanisme absent de rlc Go** (pas
de flag `-c 110002`). À vérifier si géré automatiquement via `-id`
ou s'il s'agit d'un autre manquement Phase 1.

---

## §11 Dépannage

✅ « reallive.kfn not found » : le mécanisme `findKFN` existe bien
dans `GUI-Sources/app.go` mais **côté CLI direct, c'est le flag
`-kfn`/`-K` qui prime**, pas de variable `RLDEV`. À clarifier dans
le manuel.

✅ « op<0:001:00003> » : explication correcte, KFN absent →
warning explicite désormais grâce au nouveau système diag.

✅ « caractères corrompus » : avec le nouveau système diag, le warning
pointe désormais **précisément la ligne et le code point**. Ajouter
un exemple dans le manuel :
```
Warning (SEEN0001.org line 42): invalid UTF-8 byte 0x83 — likely a
stray Shift-JIS character in a UTF-8 file
```

❌ « Les accents ne s'affichent pas en jeu » → recommande `-x CP1252
--force-transform`. **Cette option n'existe pas dans rlc Go.** À
réécrire en attendant le portage, ou prioriser le portage.

---

# Récapitulatif des actions sur le manuel

## Priorité haute (cassures factuelles)

1. **§5.2 & §11** : retirer ou marquer comme « non encore portée »
   la transformation `-x ENC` / `--force-transform`. Ouvrir une
   issue Phase 1.
2. **§5.3** : corriger le tableau (flags inexistants `-x`, `-G` rlc,
   `-c 110002`, `-u`, `-g --no-debug`).
3. **§9** : corriger les correspondances OCaml→Go pour `-G` (devient
   `-id` pour rlc), `-c 110002` (absent), `-x` (absent).
4. **§10** : préciser que `-G` ne marche que pour kprl, `-id` pour rlc.

## Priorité moyenne (compléments)

5. Ajouter une section **§ Diagnostics** documentant le nouveau
   système de log (format OCaml, compteurs, `-q`/`-v`/`-Wfatal`).
6. §4.3 : compléter le tableau kprl avec `-r`, `-raw-strings`,
   `-opcodes`, `-hexdump`, `-y`, `-cast`.
7. §5.3 : compléter le tableau rlc avec `-I/--interpreter`, `-Wfatal`,
   `-q`, `-O`, `-id`, `-cast`, etc.

## Priorité basse (cosmétique)

8. Uniformiser le nom de binaire (kprl ou kprl16 ?). Idem rlc.
9. Mentionner la GUI AIO comme interface principale.
10. Ajouter exemples de sortie diag dans §11.

---

# Manuel toujours valide sur

- §1, §2 globalement (philosophie du portage)
- §3 (structure dossier)
- §4.1, §4.2, §4.4, §4.5 (kprl basique)
- §5.1 (rlc compilation basique)
- §6 (vaconv)
- §7 (rlxml)
- §8 (workflow général, sauf points soulevés)
