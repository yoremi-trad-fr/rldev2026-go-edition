# RealLive native debug mode and Flag.ini

A concise guide to using the RealLive native debugger with scripts extracted
by RLdev2026-Go. This guide focuses primarily on CLANNAD Steam, but the logic is
generic for RealLive games.

## What the debug mode does

The engine’s debug mode is not an RLdev tool. It is a hidden interface of the
RealLive/Siglus Steam engine that can display in-game debug windows.

- `F3`: source/scene window, when the bytecode contains debug information.
- `F5`: monitoring of variables/flags stored in `flag.ini`.
- `F6`: monitoring of read/kidoku flags.
- `O`: opens the current source in the editor configured by the debugger.

In CLANNAD Steam, the screenshots show that debugging is indeed active: the engine
displays references of the type `Seen0414(Line)`. If the Japanese text in
the interface is unreadable, it is likely a CP932 locale issue,
not an extraction problem.

## Game settings

In `Gameexe.ini`, enable at least the following:

```ini
#MEMORY=1
```

Useful option to make the debug state more visible:

```ini
#DEBUG_WINDOW_CAPTION=001
```

The native debugger is old and uses ANSI/CP932. To avoid garbled text in the
menus/windows, run the game under a Japanese locale, for example using Locale
Emulator, or with the Japanese system locale setting.

## Extracting debug sources

For the debugger to locate the current source, you need to extract a set of
`.org` files containing RealLive debug information:

```bat
kprl -d -g -e CP932 -kfn KFN\reallive.kfn -o seen-debug SEEN.TXT
```

In the beta 2.6 GUI, tick:

```text
RealLive debug sources (-g / #line)
```

Important points:

- `-g` is the debug flag for the `kprl` disassembler.
- `-G` is the Game ID; it is not debug mode.
- Generate these `.org` files from the same `SEEN.TXT` file that is loaded in-game.
- Do not mix these debug `.org` files with the standard translation `.org` files.
- For the native debugger, use `CP932 / Shift-JIS`; `UTF-8` remains practical
  for tools, but may be misread by the native interface.

Next, in the game’s debug settings (`デバッグ設定`), point the source folder
to `seen-debug`, then configure the desired editor. If `O` opens an
empty file, first check the source folder, the `-g` option, and that
the `.org` files do indeed correspond to the `SEEN.TXT` currently being tested.

## Flag.ini

`flag.ini` is optional. It does not create a scene index and is not used to
map `.org` files. It is used solely to assign human-readable labels to the variables
displayed by the `F5` window.

The minimum format, based on the behaviour of the old RLdev, is:

```ini
variable[index]:group:label
```

Examples:

```ini
intA[0]:0:example_intA_flag
intB[0]:0:example_intB_flag
intC[1000]:0:example_route_flag
intL[0]:0:example_local_flag
strS[0]:0:example_string_slot
```

Group `0` is the default group. The old RLdev options
`--flag-labels` and the Kepago directive `labelled` were used to generate this type
of line automatically, but for a translation project it is often
easier to maintain a manual `flag.ini` containing only the relevant flags.

For CLANNAD, a truly useful `flag.ini` must list the route,
affection and status variables identified in the scripts. A generic template is sufficient to
bring up the `F5` window, but it will not replace the audit of the game’s
flags.

## No need for .idx or .inf files

For RealLive source debugging, no additional `.idx` or `.inf` files
are required in the `seen` folder. The mapping comes from two sources:

- the debug information present in the bytecode of `SEEN.TXT`;
- the `#line` directives present in the `.org` files extracted with `kprl -g`.

The file verification button in the debug panel is not an
`.org` index generator; if it does not populate the list of scenes, check instead
the source folder, the CP932 locale, or the exact correspondence between
debug sources and the tested archive.

There are probably no missing `.idx` or `.inf` files. For the native
RealLive debugger, you need to regenerate a special debug set of `.org` files using `kprl -g` or the
GUI option `RealLive debug sources (-g / #line)`, using the same `SEEN.TXT` file as
the one tested in-game, then point `デバッグ設定` to this folder. `flag.ini` is
separate: it is only used to name the variables visible in `F5`.

Useful references:

- [Original RLdev manual, `usage.tex`](https://github.com/eglaysher/rldev/blob/master/src/docsrc/usage.tex):
  `kprl -g / --debug-info` option.
- [Original RLdev manual, `usage.tex`](https://github.com/eglaysher/rldev/blob/master/src/docsrc/usage.tex):
  `rlc --flag-labels` option and `flag.ini` format.


# RealLive native debug mode and Flag.ini

Guide concis pour utiliser le debugger natif RealLive avec des scripts extraits
par RLdev2026-Go. Le cas vise surtout CLANNAD Steam, mais la logique est
generique pour les jeux RealLive.

## Ce que fait le mode debug

Le mode debug du moteur n'est pas un outil RLdev. C'est une interface cachee du
moteur RealLive/Siglus Steam qui peut afficher des fenetres de debug en jeu.

- `F3` : fenetre source/scene, quand le bytecode contient des infos de debug.
- `F5` : suivi des variables/flags enregistres dans `flag.ini`.
- `F6` : suivi des flags de lecture/kidoku.
- `O` : ouvre la source courante dans l'editeur configure par le debugger.

Sur CLANNAD Steam, les captures montrent que le debug est bien actif: le moteur
affiche des references du type `Seen0414(Line)`. Si les textes japonais de
l'interface sont illisibles, c'est probablement un probleme de locale CP932,
pas un probleme d'extraction.

## Activation cote jeu

Dans `Gameexe.ini`, activer au minimum:

```ini
#MEMORY=1
```

Option utile pour rendre l'etat debug plus visible:

```ini
#DEBUG_WINDOW_CAPTION=001
```

Le debugger natif est ancien et ANSI/CP932. Pour eviter le mojibake dans les
menus/fenetres, lancer le jeu sous locale japonaise, par exemple avec Locale
Emulator, ou avec le parametre regional systeme japonais.

## Extraction des sources debug

Pour que le debugger retrouve la source courante, il faut extraire un jeu de
`.org` avec les infos de debug RealLive:

```bat
kprl -d -g -e CP932 -kfn KFN\reallive.kfn -o seen-debug SEEN.TXT
```

Dans la GUI beta 2.6, cocher:

```text
Sources debug RealLive (-g / #line)
```

Points importants:

- `-g` est le flag de debug du disassembleur `kprl`.
- `-G` est le Game ID, ce n'est pas le mode debug.
- Generer ces `.org` depuis le meme `SEEN.TXT` que celui lance en jeu.
- Ne pas melanger ces `.org` debug avec les `.org` de traduction courants.
- Pour le debugger natif, preferer `CP932 / Shift-JIS`; `UTF-8` reste pratique
  pour les outils, mais peut etre mal lu par l'interface native.

Ensuite, dans les parametres debug du jeu (`デバッグ設定`), pointer le dossier
source vers `seen-debug`, puis configurer l'editeur voulu. Si `O` ouvre un
fichier vide, verifier d'abord le dossier source, l'option `-g`, et le fait que
les `.org` correspondent bien au `SEEN.TXT` en cours de test.

## Flag.ini

`flag.ini` est optionnel. Il ne cree pas d'index de scenes et ne sert pas a
mapper les `.org`. Il sert seulement a donner des labels lisibles aux variables
affichees par la fenetre `F5`.

Le format minimal, d'apres le comportement de l'ancien RLdev, est:

```ini
variable[index]:group:label
```

Exemples:

```ini
intA[0]:0:example_intA_flag
intB[0]:0:example_intB_flag
intC[1000]:0:example_route_flag
intL[0]:0:example_local_flag
strS[0]:0:example_string_slot
```

Le groupe `0` est le groupe par defaut. Les anciennes options RLdev
`--flag-labels` et la directive Kepago `labelled` servaient a produire ce type
de lignes automatiquement, mais pour un projet de traduction il est souvent
plus simple de maintenir un `flag.ini` manuel avec seulement les flags utiles.

Pour CLANNAD, un `flag.ini` vraiment utile doit lister les variables de route,
d'affection et d'etat reperees dans les scripts. Un modele generique suffit a
faire apparaitre la fenetre `F5`, mais il ne remplacera pas l'audit des flags
du jeu.

## Pas besoin de fichier .idx ou .inf

Pour le debug source RealLive, aucun fichier `.idx` ou `.inf` supplementaire
n'est attendu dans le dossier `seen`. Le mapping vient de deux choses:

- les informations de debug presentes dans le bytecode du `SEEN.TXT`;
- les directives `#line` presentes dans les `.org` extraits avec `kprl -g`.

Le bouton de verification des fichiers du panneau debug n'est pas un generateur
d'index `.org`; s'il ne remplit pas la liste de scenes, chercher plutot du cote
du dossier source, de la locale CP932, ou de la correspondance exacte entre
sources debug et archive testee.

Il ne manque probablement pas de `.idx` ou `.inf`. Pour le debugger natif
RealLive, il faut regenerer un jeu de `.org` special debug avec `kprl -g` ou la
case GUI `Sources debug RealLive (-g / #line)`, depuis le meme `SEEN.TXT` que
celui teste en jeu, puis pointer `デバッグ設定` vers ce dossier. `flag.ini` est
separe: il ne sert qu'a nommer les variables visibles dans `F5`.

References utiles:

- [Manuel RLdev original, `usage.tex`](https://github.com/eglaysher/rldev/blob/master/src/docsrc/usage.tex):
  option `kprl -g / --debug-info`.
- [Manuel RLdev original, `usage.tex`](https://github.com/eglaysher/rldev/blob/master/src/docsrc/usage.tex):
  option `rlc --flag-labels` et format `flag.ini`.
