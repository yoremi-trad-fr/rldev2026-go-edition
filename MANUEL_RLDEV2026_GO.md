# RLdev2026-Go — Manuel d'utilisation

## Toolchain de modding pour RealLive (Visual Art's / Key)

**Version** : 2026.2 (portage Go)
**Auteur du fork** : Yoremi (Jérémy)
**Base** : RLdev 1.45 (Haeleth) → Richard 23 → fork Yoremi 2026.2 → portage Go

---

## 1. Qu'est-ce que RLdev2026-Go ?

RLdev est le seul toolkit capable de décompiler, modifier et recompiler les
visual novels utilisant le moteur **RealLive** de Visual Art's / Key :

- **AIR**, **Kanon**, **CLANNAD**, **CLANNAD Side Stories**
- **Tomoyo After**, **Little Busters!**, **Little Busters! EX**
- **Planetarian**, **Kud Wafter**
- Et tous les autres jeux RealLive

**RLdev2026-Go** est le portage complet en Go du fork OCaml. L'objectif :
rendre RLdev utilisable sur les OS modernes **sans Cygwin, sans VM, sans
configuration complexe**. Un simple dossier avec des `.exe` qui marchent.

---

## 2. Ce qui change par rapport à RLdev OCaml

### ✅ Plus besoin de Cygwin
L'ancienne version nécessitait :
- Une VM Windows 32-bit (ou Wine)
- Cygwin installé et configuré
- Des chemins `/cygdrive/c/...` partout
- Des DLL OCaml spécifiques

**La version Go** : un dossier, des `.exe` natifs, c'est tout.

### ✅ Cross-platform
Compilation native pour :
- **Windows** x64 (7, 8, 10, 11)
- **Linux** x64
- **macOS** (Intel et Apple Silicon)

### ✅ Chemins Windows natifs
```
# Avant (Cygwin)
./kprl15.exe -d -o /cygdrive/c/cygwin/usr/local/share/rldev/out c:/SIDE/SEEN.txt

# Maintenant
kprl16.exe -d -o C:\output SEEN.txt
```

### ✅ Recherche automatique du KFN
Le fichier `reallive.kfn` est cherché automatiquement dans :
1. Le dossier de l'exécutable
2. `<exe>/lib/`
3. `%RLDEV%/lib/` (variable d'environnement)
4. `%USERPROFILE%/rldev/lib/`

Plus besoin de `/usr/local/share/rldev/lib/`.

### ⚠️ Ce qui ne change PAS
- Le format des fichiers `.org` / `.ke` est identique
- Le format SEEN.txt est identique
- Les fichiers `.kfn`, `.kh`, `gameexe.ini` sont les mêmes
- Les commandes CLI sont compatibles (mêmes flags)
- Le format G00 et GAN est le même

---

## 3. Installation

### Structure du dossier

```
rldev2026-go/
├── kprl16.exe          ← archiveur / désassembleur
├── rlc2026.exe         ← compilateur Kepago
├── rlxml.exe           ← convertisseur GAN ↔ XML
├── vaconv.exe          ← convertisseur G00 ↔ PNG
├── lib/
│   ├── reallive.kfn    ← définitions de fonctions (auto-détecté)
│   ├── reallive141.kfn ← version alternative
│   ├── compat.kh       ← bibliothèque de compatibilité
│   ├── rlBabel.kh      ← bibliothèque multi-langue
│   ├── rlapi.kh        ← API RealLive
│   ├── vas_g00.dtd     ← DTD pour métadonnées G00
│   └── game.cfg        ← configuration des jeux
└── README.txt
```

### Compilation depuis les sources

```bash
# Compiler les 4 outils (depuis n'importe quel OS)
cd rldev2026-go

# Pour Windows
set GOOS=windows
set GOARCH=amd64
go build -o kprl16.exe    ./kprl/cmd/kprl/
go build -o rlc2026.exe   ./rlc/cmd/rlc/
go build -o rlxml.exe     ./rlxml/cmd/rlxml/
go build -o vaconv.exe    ./vaconv/cmd/vaconv/

# Pour Linux
GOOS=linux GOARCH=amd64 go build -o kprl16 ./kprl/cmd/kprl/
# etc.
```

### Variable d'environnement (optionnel)

```
set RLDEV=C:\rldev2026-go
set PATH=%PATH%;%RLDEV%
```

---

## 4. kprl16 — Archiveur et désassembleur

### 4.1 Lister le contenu d'une archive

```
kprl16 -l SEEN.txt
```

Affiche la liste des fichiers (SEEN0001.TXT, SEEN0002.TXT, etc.)
avec leurs tailles compressée et décompressée.

### 4.2 Extraire les fichiers bruts

```
kprl16 -x SEEN.txt
kprl16 -x -o C:\output SEEN.txt
kprl16 -x SEEN.txt 1-10 50 100-150    # plages spécifiques
```

Extrait les fichiers `.TXT` bruts (bytecode compressé).

### 4.3 Désassembler en scripts Kepago (.org)

```
# Désassembler en UTF-8 (recommandé pour la traduction)
kprl16 -d -e UTF-8 -o C:\projet\scripts SEEN.txt

# Désassembler en CP932 / Shift-JIS (format japonais natif)
kprl16 -d -e CP932 -o C:\projet\scripts SEEN.txt

# Avec le KFN pour résoudre les noms de fonctions
kprl16 --kfn=reallive.kfn -d -e UTF-8 SEEN.txt

# Désassembler des fichiers individuels
kprl16 -d SEEN0001.TXT SEEN0002.TXT
```

**Options de désassemblage :**

| Flag | Description |
|------|-------------|
| `-d` | Désassembler |
| `-e ENC` | Encodage de sortie : `UTF-8` (défaut), `CP932` |
| `-o DIR` | Dossier de sortie |
| `--kfn=FILE` | Fichier de définition des fonctions |
| `-f VER` | Version de l'interpréteur (ex: `1.2.7.0`) |
| `-G GID` | ID du jeu : `LB`, `CLAN`, `FIVE`, `LBEX`, etc. |
| `-s` | Fichier unique (pas de .utf/.sjs séparé) |
| `-S` | Séparer tout le texte japonais |
| `-n` | Annoter avec les offsets |
| `-t TGT` | Cible : `RealLive`, `AVG2000`, `Kinetic` |
| `-v N` | Verbosité (1 = normal, 2 = très détaillé) |
| `--bom` | Inclure le BOM UTF-8 |
| `--ext=EXT` | Extension des scripts (défaut : `org`) |

### 4.4 Créer / mettre à jour une archive

```
# Créer une nouvelle archive avec tous les .TXT compilés
kprl16 -a SEEN.txt new_seens\*.TXT

# Ajouter/remplacer des fichiers spécifiques
kprl16 -a SEEN.txt SEEN0001.TXT SEEN0628.TXT
```

### 4.5 Autres actions

```
kprl16 -k SEEN.txt 50 100-150   # Supprimer des fichiers
kprl16 -i SEEN.txt               # Afficher les infos d'en-tête
kprl16 -b SEEN.txt               # Extraire sans décompresser
kprl16 -c fichier.TXT            # Compresser un fichier
```

---

## 5. rlc2026 — Compilateur Kepago

### 5.1 Compilation basique

```
# Compiler un script .org en .TXT (bytecode RealLive)
rlc2026 -v -K reallive.kfn -e UTF-8 fichier.org

# Spécifier le dossier de sortie
rlc2026 -v -o SEEN0001 -d C:\output -K reallive.kfn fichier.org

# Avec le GAMEEXE.INI pour la configuration du jeu
rlc2026 -v -i C:\jeu\gameexe.ini -K reallive.kfn fichier.org
```

### 5.2 Compilation avec transformation d'encodage

```
# Pour les patchs occidentaux (Latin/accents)
rlc2026 -x CP1252 --force-transform -K reallive.kfn fichier.org

# Pour les patchs chinois
rlc2026 -x CP936 -K reallive.kfn fichier.org

# Pour les patchs coréens
rlc2026 -x CP949 -K reallive.kfn fichier.org
```

### 5.3 Options complètes

| Flag | Long | Description |
|------|------|-------------|
| `-v` | `--verbose` | Mode verbeux |
| `-o FILE` | `--output` | Nom du fichier de sortie |
| `-d DIR` | `--outdir` | Dossier de sortie |
| `-i FILE` | `--ini` | Chemin vers GAMEEXE.INI |
| `-e ENC` | `--encoding` | Encodage d'entrée (défaut : `UTF-8`) |
| `-x ENC` | `--transform-output` | Transformation de sortie |
| | `--force-transform` | Ne pas échouer sur les caractères non-mappables |
| `-K FILE` | `--kfn` | Fichier de définition des fonctions |
| `-t TGT` | `--target` | Cible : `RealLive`, `AVG2000`, `Kinetic` |
| `-f VER` | `--target-version` | Version de l'interpréteur |
| `-G GID` | `--game` | ID du jeu |
| `-u` | `--uncompressed` | Ne pas compresser la sortie |
| `-g` | `--no-debug` | Supprimer les infos de débogage |

---

## 6. vaconv — Convertisseur d'images G00

### 6.1 G00 → PNG (extraction)

```
# Convertir un fichier
vaconv BG011.g00

# Convertir plusieurs fichiers
vaconv -d C:\output *.g00

# Mode verbeux
vaconv -v BG011.g00
```

### 6.2 PNG → G00 (compilation)

```
# Convertir un PNG en G00
vaconv -o BG011.g00 -i png BG011.png

# Spécifier le format G00
vaconv -f 0 -o BG011.g00 -i png BG011.png    # format 0 (RGB)
```

### 6.3 Formats G00 supportés

| Format | Description | Utilisation |
|--------|-------------|-------------|
| 0 | Bitmap RGB 24-bit | Fonds d'écran (BG) |
| 1 | Bitmap paletté 8-bit + alpha | Sprites simples |
| 2 | Composite RGBA avec régions | Sprites complexes, personnages |

---

## 7. rlxml — Convertisseur d'animations GAN

```
# GAN → XML
rlxml animation.gan

# XML → GAN
rlxml animation.ganxml

# Avec dossier de sortie
rlxml -d C:\output -v animation.gan
```

---

## 8. Workflow complet de traduction

### Étape 1 : Extraction

```bash
# Créer un dossier de travail
mkdir projet_trad
cd projet_trad

# Copier les fichiers nécessaires
copy C:\jeu\SEEN.txt .
copy C:\rldev2026-go\lib\reallive.kfn .

# Désassembler tous les scripts
kprl16 --kfn=reallive.kfn -d -e UTF-8 -o scripts SEEN.txt
```

### Étape 2 : Traduction

Éditez les fichiers `.org` dans `scripts/` avec un éditeur de texte
(VS Code, Notepad++, etc.). Le texte est en UTF-8.

Les chaînes de texte sont dans les fichiers `.utf` (ou `.sjs`) associés.

### Étape 3 : Compilation

```bash
# Compiler chaque script modifié
mkdir new_seens
for %f in (scripts\SEEN*.org) do (
    rlc2026 -v -K reallive.kfn -e UTF-8 -d new_seens %f
)
```

### Étape 4 : Assemblage

```bash
# Créer la nouvelle archive SEEN.txt
kprl16 -a SEEN_patched.txt new_seens\*.TXT

# Copier dans le dossier du jeu
copy SEEN_patched.txt C:\jeu\SEEN.txt
```

### Étape 5 : Images (optionnel)

```bash
# Extraire les images pour modification
vaconv -d images_png C:\jeu\g00\*.g00

# Modifier les PNG (Photoshop, GIMP, etc.)

# Recompiler
for %f in (images_modifiees\*.png) do (
    vaconv -o C:\jeu\g00\%~nf.g00 -i png %f
)
```

---

## 9. Migration depuis RLdev OCaml

### Correspondance des commandes

| Commande OCaml (Cygwin) | Commande Go (Windows natif) |
|---|---|
| `./kprl15.exe -v -f 1.5 -d -o /cygdrive/c/.../out c:/SIDE/SEEN.txt` | `kprl16 -v -f 1.5 -d -o C:\out SEEN.txt` |
| `./kprl15.exe -e cp932 -d -o /cygdrive/c/.../out c:/CLANNAD/SEEN.txt` | `kprl16 -e CP932 -d -o C:\out SEEN.txt` |
| `./rlc2026 -v -o SEEN03000 -d out -e cp932 -i C:/SIDE/gameexe.ini SEEN3000.org` | `rlc2026 -v -o SEEN03000 -d out -e CP932 -i gameexe.ini SEEN3000.org` |
| `./rlc2026 -o SEEN0628 -d /cygdrive/.../new_seens -x CP932 --force-transform -i file.org` | `rlc2026 -o SEEN0628 -d new_seens -x CP932 --force-transform file.org` |
| `./kprl16 -a SEEN.txt new_seens/*.TXT` | `kprl16 -a SEEN.txt new_seens\*.TXT` |
| `vaconv *.g00 -d g00` | `vaconv -d g00 *.g00` |
| `vaconv -i png xxx.png -m xxx.xml -o xxx.g00` | `vaconv -i png -o xxx.g00 xxx.png` |

### Flags renommés

| OCaml | Go | Notes |
|-------|-----|-------|
| `-i FILE` (rlc) | `-i FILE` ou `-g FILE` | Les deux marchent |
| `--ini=FILE` | `--ini=FILE` ou `-g` | Alias ajouté |
| Chemins Cygwin | Chemins Windows natifs | `C:\dossier\fichier` |

### Fichiers de configuration

Les fichiers `.kfn`, `.kh`, `.cfg`, `.dtd` sont identiques.
Placez-les dans un sous-dossier `lib/` à côté des exécutables.

---

## 10. Jeux supportés

| ID | Jeu | Encryption |
|----|-----|-----------|
| `LB` | Little Busters! (défaut) | Standard |
| `LBEX` | Little Busters! EX | 110002 |
| `LBME` | Little Busters! ME | 110002 |
| `CLAN` | CLANNAD | Standard |
| `FIVE` | CLANNAD Full Voice | 110002 |
| `CFV` | CLANNAD Full Voice (alt) | 110002 |
| `SNOW` | Snow (Moefont) | Standard |
| `LBd` | Little Busters! (debug) | Standard |

Pour les jeux avec encryption `110002`, ajoutez `-G ID` :

```
kprl16 -G FIVE -d SEEN.txt
rlc2026 -G FIVE -c 110002 fichier.org
```

---

## 11. Dépannage

### "reallive.kfn not found"
→ Placez `reallive.kfn` dans le même dossier que l'exécutable,
ou dans un sous-dossier `lib/`, ou définissez `RLDEV=C:\rldev2026-go`.

### Les fonctions apparaissent comme `op<0:001:00003>`
→ Le KFN n'est pas chargé. Utilisez `--kfn=reallive.kfn`.

### Caractères corrompus dans les scripts
→ Vérifiez l'encodage : `-e UTF-8` pour Unicode, `-e CP932` pour Shift-JIS.
Les fichiers `.org` sont en UTF-8 par défaut dans la version Go.

### Les accents ne s'affichent pas en jeu
→ Utilisez la transformation Western : `-x CP1252 --force-transform`
et un patch de police (DLL proxy ou font hack).

---

*RLdev2026-Go — Portage complet d'OCaml vers Go*
*Zéro dépendance. Zéro configuration. Ça marche.*
