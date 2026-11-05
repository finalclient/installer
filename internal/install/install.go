package install

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
)

// Options steuert eine Installation. Leere Felder bekommen sinnvolle Standardwerte.
type Options struct {
	MinecraftDir string       // Ordner des offiziellen Launchers
	Icon         []byte       // PNG für das Profil
	HTTP         *http.Client // für Tests austauschbar
	FabricMeta   string       // Basis-Adresse von meta.fabricmc.net
	Parallel     int          // gleichzeitige Downloads
}

// Progress meldet Fortschritt: done von total Schritten, dazu ein kurzer Text.
type Progress func(done, total int, msg string)

const stateFile = ".finalclient-installer.json"

// state merkt sich, welche Dateien der Installer angelegt hat. Nur die räumt
// ein Update wieder weg, selbst hinzugefügte Mods bleiben liegen.
type state struct {
	Version string   `json:"version"`
	Files   []string `json:"files"`
}

var safeVersionID = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)

// Install richtet das Paket ein oder bringt eine vorhandene Installation auf dessen Stand.
func Install(ctx context.Context, pack *Pack, o Options, progress Progress) error {
	if o.HTTP == nil {
		o.HTTP = http.DefaultClient
	}
	if o.FabricMeta == "" {
		o.FabricMeta = "https://meta.fabricmc.net"
	}
	if o.Parallel <= 0 {
		o.Parallel = 4
	}
	if progress == nil {
		progress = func(int, int, string) {}
	}
	profiles := profileFiles(o.MinecraftDir)
	if len(profiles) == 0 {
		return ErrNoLauncher
	}
	gameDir := filepath.Join(o.MinecraftDir, GameDirName)
	files := pack.Index.ClientFiles()
	total := len(files) + len(pack.Overrides) + 2
	done := 0
	step := func(msg string) { done++; progress(done, total, msg) }

	// 1. Fabric als Minecraft-Version eintragen
	progress(done, total, "Fabric wird eingerichtet …")
	versionID, err := installFabric(ctx, o, pack.Index.Dependencies["minecraft"], pack.Index.Dependencies["fabric-loader"])
	if err != nil {
		return fmt.Errorf("Fabric: %w", err)
	}
	step("Fabric eingerichtet")

	// 2. Mods und Texturpakete laden (vorhandene mit passender Prüfsumme bleiben)
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		firstEr error
		sem     = make(chan struct{}, o.Parallel)
	)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for _, f := range files {
		wg.Add(1)
		go func(f File) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			err := fetchFile(ctx, o.HTTP, f, filepath.Join(gameDir, filepath.FromSlash(f.Path)))
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstEr == nil {
					firstEr = fmt.Errorf("%s: %w", f.Path, err)
					cancel()
				}
				return
			}
			step(filepath.Base(f.Path))
		}(f)
	}
	wg.Wait()
	if firstEr != nil {
		return firstEr
	}

	// 3. Dateien aus dem Paket selbst (z. B. der FinalClient-Mod)
	for _, rel := range sortedKeys(pack.Overrides) {
		if err := writeAtomic(filepath.Join(gameDir, filepath.FromSlash(rel)), pack.Overrides[rel]); err != nil {
			return err
		}
		step(filepath.Base(rel))
	}

	// 4. Alte Dateien früherer Versionen wegräumen
	now := map[string]bool{}
	for _, f := range files {
		now[f.Path] = true
	}
	for rel := range pack.Overrides {
		now[rel] = true
	}
	old := readState(gameDir)
	for _, rel := range old.Files {
		if !now[rel] && checkPath(rel) == nil {
			_ = os.Remove(filepath.Join(gameDir, filepath.FromSlash(rel)))
		}
	}
	newState := state{Version: pack.Index.VersionID, Files: sortedKeys(now)}
	b, _ := json.MarshalIndent(newState, "", "  ")
	if err := writeAtomic(filepath.Join(gameDir, stateFile), b); err != nil {
		return err
	}

	// 5. Profil im Launcher
	for _, p := range profiles {
		if err := writeProfile(p, versionID, gameDir, o.Icon); err != nil {
			return err
		}
	}
	step("Profil im Launcher eingetragen")
	return nil
}

// InstalledVersion ist die Paketversion der letzten Installation, leer wenn keine.
func InstalledVersion(mcDir string) string {
	return readState(filepath.Join(mcDir, GameDirName)).Version
}

func readState(gameDir string) state {
	var s state
	b, err := os.ReadFile(filepath.Join(gameDir, stateFile))
	if err == nil {
		_ = json.Unmarshal(b, &s)
	}
	return s
}

// installFabric holt das Launcher-Profil von Fabric und legt es unter versions/ ab,
// so wie es der offizielle Fabric-Installer auch tut.
func installFabric(ctx context.Context, o Options, mc, loader string) (string, error) {
	u := fmt.Sprintf("%s/v2/versions/loader/%s/%s/profile/json", o.FabricMeta, mc, loader)
	body, err := get(ctx, o.HTTP, u, 1<<20)
	if err != nil {
		return "", err
	}
	var profile struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &profile); err != nil {
		return "", err
	}
	if !safeVersionID.MatchString(profile.ID) {
		return "", fmt.Errorf("unerwartete Versions-ID %q", profile.ID)
	}
	dir := filepath.Join(o.MinecraftDir, "versions", profile.ID)
	if err := writeAtomic(filepath.Join(dir, profile.ID+".json"), body); err != nil {
		return "", err
	}
	jar := filepath.Join(dir, profile.ID+".jar") // der Launcher erwartet eine (leere) Jar
	if _, err := os.Stat(jar); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(jar, nil, 0o644); err != nil {
			return "", err
		}
	}
	return profile.ID, nil
}

func fetchFile(ctx context.Context, client *http.Client, f File, dest string) error {
	want := f.Hashes["sha512"]
	if sum, err := hashFile(dest); err == nil && sum == want {
		return nil // schon da
	}
	var lastErr error
	for _, u := range f.Downloads {
		data, err := get(ctx, client, u, 512<<20)
		if err != nil {
			lastErr = err
			continue
		}
		sum := sha512.Sum512(data)
		if hex.EncodeToString(sum[:]) != want {
			lastErr = errors.New("Prüfsumme stimmt nicht, Datei wurde nicht gespeichert")
			continue
		}
		return writeAtomic(dest, data)
	}
	return lastErr
}

func get(ctx context.Context, client *http.Client, u string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "finalclient-installer (github.com/steedalot/finalclient-installer)")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", u, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha512.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
