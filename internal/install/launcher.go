package install

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const (
	ProfileID   = "finalclient"
	ProfileName = "FinalClient"
	GameDirName = "finalclient" // eigener Spielordner neben .minecraft-Inhalten, Vanilla bleibt unberührt
	javaArgs    = "-Xmx4G -XX:+UnlockExperimentalVMOptions -XX:+UseG1GC"
)

// DefaultMinecraftDir ist der Ordner des offiziellen Launchers.
func DefaultMinecraftDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "windows":
		if appdata := os.Getenv("APPDATA"); appdata != "" {
			return filepath.Join(appdata, ".minecraft"), nil
		}
		return filepath.Join(home, "AppData", "Roaming", ".minecraft"), nil
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "minecraft"), nil
	default:
		return filepath.Join(home, ".minecraft"), nil
	}
}

// ErrNoLauncher: Der Launcher wurde auf diesem Rechner noch nie gestartet.
var ErrNoLauncher = errors.New("kein Minecraft-Launcher gefunden. Starte ihn einmal, melde dich an, schließe ihn wieder und versuche es dann noch mal")

// profileFiles sind die Profil-Dateien, die es gibt (die Microsoft-Store-Version hat eine eigene).
func profileFiles(mcDir string) []string {
	var out []string
	for _, name := range []string{"launcher_profiles.json", "launcher_profiles_microsoft_store.json"} {
		p := filepath.Join(mcDir, name)
		if _, err := os.Stat(p); err == nil {
			out = append(out, p)
		}
	}
	return out
}

// writeProfile trägt das FinalClient-Profil ein und lässt alles andere in der Datei, wie es ist.
func writeProfile(file, versionID, gameDir string, icon []byte) error {
	raw, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("%s ist kaputt: %w", filepath.Base(file), err)
	}
	backup := file + ".vor-finalclient"
	if _, err := os.Stat(backup); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(backup, raw, 0o644); err != nil {
			return err
		}
	}
	profiles, _ := doc["profiles"].(map[string]any)
	if profiles == nil {
		profiles = map[string]any{}
		doc["profiles"] = profiles
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	profile, _ := profiles[ProfileID].(map[string]any)
	if profile == nil {
		profile = map[string]any{"created": now}
	}
	profile["name"] = ProfileName
	profile["type"] = "custom"
	profile["lastVersionId"] = versionID
	profile["gameDir"] = gameDir
	profile["lastUsed"] = now
	if _, ok := profile["javaArgs"]; !ok { // eigene Einstellung im Launcher behalten
		profile["javaArgs"] = javaArgs
	}
	if len(icon) > 0 {
		profile["icon"] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(icon)
	}
	profiles[ProfileID] = profile
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(file, out)
}

func writeAtomic(file string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return err
	}
	tmp := file + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, file)
}
