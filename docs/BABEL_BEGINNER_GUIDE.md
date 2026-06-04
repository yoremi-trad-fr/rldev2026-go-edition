# Guide debutant Babel pour RLdev2026-Go

Ce guide explique le workflow Babel dans la GUI RLdev2026-Go. Il est ecrit pour un premier test sur CLANNAD 2004, mais le principe vaut pour les vieux titres RealLive.

## A quoi sert Babel

Babel ajoute un moteur de rendu texte a RealLive. Il n'est pas seulement la pour les accents. Il sert surtout a afficher du texte traduit avec une largeur variable, une lineation dynamique, et un meilleur controle des retours a la ligne que le rendu CP932 classique.

Le transform `WESTERN` reste utile : il convertit le texte latin dans l'espace bytecode RealLive. Babel, lui, gere l'affichage runtime de ce texte.

## Avant de commencer

Travaille toujours sur une copie du jeu.

Il faut avoir :
- le dossier du jeu avec `GAMEEXE.INI`, `RealLive.exe` et `SEEN.TXT`;
- le dossier `BABEL` contenant `rtl\rlBabel.dll`, `rtl\rlBabelF.dll` et les `.map`;
- les scripts extraits avec RLdev/KPRL;
- une sortie de compilation separee, par exemple `work\compiled`.

Pour CLANNAD 2004, le premier essai recommande est `RealLive 1.2.3.5`.

## Etape 1 - Preparer le runtime Babel

Dans la GUI :

1. Ouvre `BABEL > Runtime setup`.
2. `BABEL folder` : selectionne le dossier `BABEL`.
3. `Game folder` : selectionne le dossier du jeu, celui qui contient `GAMEEXE.INI`.
4. `RealLive version` : mets `1.2.3.5` pour CLANNAD 2004.
5. `DLL` : laisse `Auto by version`.
6. `#NAME_ENC` : choisis `Western`.
7. Laisse `Update GAMEEXE.INI` coche.
8. Clique `Prepare Runtime`.

Pour une version inferieure a `1.2.5`, la GUI copie `rlBabelF.dll`. Pour `1.2.5+`, elle copie `rlBabel.dll` et ajoute une entree `#DLL.xxx = "rlBabel"` dans `GAMEEXE.INI`.

Une sauvegarde `GAMEEXE.INI.babel-....bak` est creee avant modification.

## Etape 2 - Creer le header Babel

Dans la GUI :

1. Ouvre `BABEL > global.kh helper`.
2. Choisis un dossier de sortie, par exemple le dossier de tes scripts extraits.
3. Laisse `Enable glosses` decoche pour le premier test.
4. Clique `Create global.kh`.

Pour le premier test, copie ces lignes au debut du script `.org` que tu veux compiler :

```kepago
#define __DynamicLineation__ = 1
#load 'rlBabel'
```

Ne te contente pas de poser `global.kh` dans le dossier : pour l'instant, copie les lignes dans le script a tester ou dans un header commun deja inclus par ton workflow.

## Etape 3 - Compiler un seul script test

Commence par une seule scene, pas tout le jeu.

Dans `KPRL / RLC > Compile .org / .ke / .avg` :

1. Selectionne un seul `.org`.
2. Selectionne `KFN\reallive.kfn`.
3. Selectionne le `GAMEEXE.INI` du jeu.
4. Selectionne `RealLive.exe` si possible.
5. `Version RealLive` : mets `1.2.3.5` si la detection de l'exe est incertaine.
6. `Encodage source` : `UTF-8` si tes scripts sont en UTF-8.
7. `Transformation sortie` : `WESTERN` pour un script deja traduit en alphabet latin.
8. `Force transform` : laisse decoche au premier essai.
9. Choisis le dossier de sortie.
10. Clique `Compile`.

Important : si tu recompiles un script encore entierement japonais juste pour verifier que Babel compile, n'utilise pas `WESTERN` sur ce test brut. Le mode `WESTERN` est fait pour le texte traduit en francais/anglais ; sur du japonais original, il signale des caracteres impossibles a representer et les remplace si le forcage est actif. Pour un test Babel utile, traduis d'abord quelques lignes, ou fais un test separe sans transformation de sortie.

Si la compilation signale des caracteres impossibles a representer, corrige-les ou coche `Force transform` seulement pour un test rapide.

## Etape 4 - Rebuild et test en jeu

1. Va dans `KPRL / RLC > Rebuild SEEN.txt`.
2. `Input folder` : dossier contenant les `.TXT` recompiles.
3. `Original/template SEEN.txt` : le `SEEN.TXT` original du jeu.
4. `Output SEEN.txt` : un nouveau `SEEN.TXT` dans une copie du jeu.
5. Lance le jeu depuis cette copie.

Teste d'abord :
- demarrage du jeu;
- affichage d'une ligne modifiee;
- retour ligne automatique;
- sauvegarde puis chargement;
- passage a la ligne suivante apres clic.

## Depannage rapide

Crash au lancement :
- verifie que la bonne DLL est dans le dossier du jeu;
- pour CLANNAD 2004, commence avec `rlBabelF.dll` et `1.2.3.5.map`;
- verifie que la version de compilation est bien `1.2.3.5`.

Texte affiche comme avant :
- verifie que les deux lignes Babel ont bien ete copiees dans le `.org`;
- verifie que la compilation utilise le script modifie, pas un ancien fichier.

Accents incorrects :
- utilise `Transformation sortie = WESTERN`;
- dans `Runtime setup`, garde `#NAME_ENC = Western`.

Beaucoup d'avertissements sur des kanji :
- c'est normal si le script est encore japonais et que la sortie est en `WESTERN`;
- traduis les lignes concernees, ou recompile ce script brut sans transformation pour un test technique.

Erreur `undefined function` :
- verifie que `KFN\reallive.kfn` est selectionne;
- force `Version RealLive = 1.2.3.5` pour CLANNAD 2004.

Glosses :
- laisse `Enable glosses` desactive pour le moment. Le port Go actuel compile le texte de base, mais les gloss interactifs ne sont pas encore le bon premier test.

## Commande CLI equivalente

Exemple pour compiler un script CLANNAD 2004 en Babel :

```powershell
bin\rlc2026.exe -v -e UTF-8 -x WESTERN -K KFN\reallive.kfn -i "C:\Jeux\CLANNAD\GAMEEXE.INI" -I "C:\Jeux\CLANNAD\RealLive.exe" --target-version 1.2.3.5 -d "C:\work\compiled" "C:\work\seen0001.org"
```

Si `RealLive.exe` donne une version correcte, `--target-version` peut etre omis. Pour un vieux titre, le garder pendant le test evite les ambiguities.
