# FinalClient-Installer

Richtet [FinalClient](https://client.finalsmp.net) im **offiziellen Minecraft-Launcher** ein. Prism oder ein anderer Launcher ist dafür nicht nötig.

Der Code ist offen, damit jeder nachlesen kann, was das Programm auf dem Rechner macht. Die Downloads unter *Releases* baut GitHub direkt aus diesem Code.

## Benutzen

1. Den Minecraft-Launcher einmal starten, anmelden und wieder schließen.
2. Den Installer starten und auf **Installieren** klicken.
3. Im Launcher das Profil **FinalClient** wählen und auf **Spielen** klicken.

Für ein Update startest du den Installer noch mal. Eigene Mods im Ordner `finalclient/mods` bleiben dabei erhalten.

**Windows-Warnung „Der Computer wurde durch Windows geschützt“:** Der Installer ist nicht signiert, denn eine Signatur kostet jedes Jahr Geld. Klicke auf *Weitere Informationen* und dann auf *Trotzdem ausführen*.
**Mac „kann nicht geöffnet werden“:** *Fertig* klicken, dann *Systemeinstellungen → Datenschutz & Sicherheit* → ganz unten *Dennoch öffnen*. Das ist nur beim ersten Mal nötig.

## Was das Programm macht

| | |
|---|---|
| Lädt | `latest.json` und das Paket von der FinalClient-Website. Das Paket wird gegen die SHA-512-Prüfsumme aus `latest.json` geprüft. |
| Lädt | Fabric von `meta.fabricmc.net` |
| Lädt | Die Mods und Texturpakete aus dem Paket, nur von `cdn.modrinth.com`, `github.com` oder `gitlab.com`, jede Datei mit SHA-512-Prüfsumme. Stimmt eine Prüfsumme nicht, wird nichts gespeichert. |
| Schreibt | `<Minecraft-Ordner>/finalclient/`: eigener Spielordner, die normalen Welten und Einstellungen bleiben unberührt |
| Schreibt | `<Minecraft-Ordner>/versions/fabric-loader-…/`: genau wie der offizielle Fabric-Installer |
| Ändert | `launcher_profiles.json`: fügt nur das Profil „FinalClient“ hinzu. Vorher wird eine Sicherung `launcher_profiles.json.vor-finalclient` angelegt. |
| Nicht | Kein Login, keine Passwörter, kein Microsoft-Konto, keine Daten an uns |

Minecraft-Ordner: Windows `%APPDATA%\.minecraft`, Mac `~/Library/Application Support/minecraft`, Linux `~/.minecraft`.

Der Installer kennt keine FinalClient-Funktionen. Was installiert wird, steht im Paket (`modrinth.index.json`, Modrinth-Format). Neue Funktionen brauchen deshalb keinen neuen Installer.

## Selbst bauen

Du brauchst Go (siehe `go.mod`) und einen C-Compiler (für die Fenster-Bibliothek [Fyne](https://fyne.io)).

```sh
go test ./...
go build .                                         # Programm
./finalclient-installer -cli -minecraft /tmp/mc    # ohne Fenster, in einen Test-Ordner
go install fyne.io/tools/cmd/fyne@v1.7.4
fyne package --release                             # .exe / .app / .tar.xz für das eigene System
```

`-mrpack Datei.mrpack` installiert ein lokales Paket statt das von der Website.

Releases entstehen mit einem Tag `v*` (siehe `.github/workflows/build.yml`), dazu kommt `SHA256SUMS` zum Nachprüfen.

## Lizenz

CC0. Mach damit, was du willst.
