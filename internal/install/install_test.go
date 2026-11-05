package install

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sha(b []byte) string { s := sha512.Sum512(b); return hex.EncodeToString(s[:]) }

// fakeWeb spielt cdn.modrinth.com und meta.fabricmc.net.
type fakeWeb struct {
	files    map[string][]byte
	requests int
}

func (w *fakeWeb) RoundTrip(r *http.Request) (*http.Response, error) {
	w.requests++
	rec := httptest.NewRecorder()
	switch {
	case r.URL.Host == "meta.test" && strings.HasSuffix(r.URL.Path, "/profile/json"):
		rec.WriteString(`{"id":"fabric-loader-0.19.5-1.21.11","inheritsFrom":"1.21.11"}`)
	case w.files[r.URL.String()] != nil:
		rec.Write(w.files[r.URL.String()])
	default:
		rec.WriteHeader(404)
	}
	return rec.Result(), nil
}

func buildPack(t *testing.T, version string, files map[string][]byte, overrides map[string][]byte) ([]byte, *fakeWeb) {
	t.Helper()
	web := &fakeWeb{files: map[string][]byte{}}
	ix := Index{FormatVersion: 1, Game: "minecraft", VersionID: version, Name: "FinalClient",
		Dependencies: map[string]string{"minecraft": "1.21.11", "fabric-loader": "0.19.5"}}
	for p, b := range files {
		u := "https://cdn.modrinth.com/data/x/" + filepath.Base(p)
		web.files[u] = b
		ix.Files = append(ix.Files, File{Path: p, Hashes: map[string]string{"sha512": sha(b)},
			Env: map[string]string{"client": "required", "server": "unsupported"}, Downloads: []string{u}})
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("modrinth.index.json")
	json.NewEncoder(w).Encode(ix)
	for p, b := range overrides {
		w, _ := zw.Create("overrides/" + p)
		w.Write(b)
	}
	zw.Close()
	return buf.Bytes(), web
}

func newMinecraftDir(t *testing.T) string {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "launcher_profiles.json"),
		[]byte(`{"profiles":{"abc":{"name":"Latest release","type":"latest-release"}},"settings":{"keepLauncherOpen":true},"version":3}`), 0o644)
	return dir
}

func TestInstallUndUpdate(t *testing.T) {
	mc := newMinecraftDir(t)
	data, web := buildPack(t, "1.0", map[string][]byte{"mods/a-1.jar": []byte("A1"), "mods/b.jar": []byte("B")},
		map[string][]byte{"mods/finalclient-1.0.jar": []byte("FC1")})
	pack, err := ReadPack(data)
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{MinecraftDir: mc, HTTP: &http.Client{Transport: web}, FabricMeta: "https://meta.test", Icon: []byte("png")}
	var last int
	if err := Install(context.Background(), pack, opts, func(done, total int, _ string) { last = done; _ = total }); err != nil {
		t.Fatal(err)
	}
	game := filepath.Join(mc, GameDirName)
	for _, p := range []string{"mods/a-1.jar", "mods/b.jar", "mods/finalclient-1.0.jar"} {
		if _, err := os.Stat(filepath.Join(game, p)); err != nil {
			t.Errorf("%s fehlt", p)
		}
	}
	if _, err := os.Stat(filepath.Join(mc, "versions/fabric-loader-0.19.5-1.21.11/fabric-loader-0.19.5-1.21.11.json")); err != nil {
		t.Error("Fabric-Version fehlt")
	}
	if last != 5 {
		t.Errorf("Fortschritt endet bei %d statt 5", last)
	}

	var doc map[string]any
	b, _ := os.ReadFile(filepath.Join(mc, "launcher_profiles.json"))
	json.Unmarshal(b, &doc)
	profiles := doc["profiles"].(map[string]any)
	fc := profiles[ProfileID].(map[string]any)
	if fc["lastVersionId"] != "fabric-loader-0.19.5-1.21.11" || fc["gameDir"] != game || !strings.HasPrefix(fc["icon"].(string), "data:image/png;base64,") {
		t.Errorf("Profil falsch: %v", fc)
	}
	if profiles["abc"] == nil || doc["settings"] == nil || doc["version"] != 3.0 {
		t.Error("bestehende Launcher-Einstellungen gingen verloren")
	}
	if _, err := os.Stat(filepath.Join(mc, "launcher_profiles.json.vor-finalclient")); err != nil {
		t.Error("keine Sicherung der Launcher-Profile")
	}
	if InstalledVersion(mc) != "1.0" {
		t.Error("Version nicht gemerkt")
	}

	// Eigener Mod des Spielers und eigene Speicher-Einstellung bleiben beim Update
	os.WriteFile(filepath.Join(game, "mods/eigener.jar"), []byte("X"), 0o644)
	fc["javaArgs"] = "-Xmx6G"
	b, _ = json.Marshal(doc)
	os.WriteFile(filepath.Join(mc, "launcher_profiles.json"), b, 0o644)

	data2, web2 := buildPack(t, "1.1", map[string][]byte{"mods/a-2.jar": []byte("A2"), "mods/b.jar": []byte("B")},
		map[string][]byte{"mods/finalclient-1.1.jar": []byte("FC2")})
	pack2, _ := ReadPack(data2)
	opts.HTTP = &http.Client{Transport: web2}
	if err := Install(context.Background(), pack2, opts, nil); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]bool{"mods/a-1.jar": false, "mods/finalclient-1.0.jar": false,
		"mods/a-2.jar": true, "mods/b.jar": true, "mods/finalclient-1.1.jar": true, "mods/eigener.jar": true} {
		_, err := os.Stat(filepath.Join(game, p))
		if (err == nil) != want {
			t.Errorf("%s: vorhanden=%v, erwartet %v", p, err == nil, want)
		}
	}
	if web2.requests != 2 { // Fabric + a-2; b.jar war schon da
		t.Errorf("%d Anfragen, erwartet 2 (vorhandene Dateien nicht neu laden)", web2.requests)
	}
	b, _ = os.ReadFile(filepath.Join(mc, "launcher_profiles.json"))
	json.Unmarshal(b, &doc)
	if doc["profiles"].(map[string]any)[ProfileID].(map[string]any)["javaArgs"] != "-Xmx6G" {
		t.Error("eigene Java-Einstellung überschrieben")
	}
}

func TestFalschePruefsummeWirdNichtGespeichert(t *testing.T) {
	mc := newMinecraftDir(t)
	data, web := buildPack(t, "1.0", map[string][]byte{"mods/a.jar": []byte("echt")}, nil)
	web.files["https://cdn.modrinth.com/data/x/a.jar"] = []byte("manipuliert")
	pack, _ := ReadPack(data)
	err := Install(context.Background(), pack, Options{MinecraftDir: mc, HTTP: &http.Client{Transport: web}, FabricMeta: "https://meta.test"}, nil)
	if err == nil || !strings.Contains(err.Error(), "Prüfsumme") {
		t.Fatalf("erwartet Prüfsummen-Fehler, bekam %v", err)
	}
	if _, err := os.Stat(filepath.Join(mc, GameDirName, "mods/a.jar")); err == nil {
		t.Error("manipulierte Datei wurde gespeichert")
	}
}

func TestOhneLauncher(t *testing.T) {
	data, web := buildPack(t, "1.0", nil, nil)
	pack, _ := ReadPack(data)
	err := Install(context.Background(), pack, Options{MinecraftDir: t.TempDir(), HTTP: &http.Client{Transport: web}}, nil)
	if err != ErrNoLauncher {
		t.Fatalf("erwartet ErrNoLauncher, bekam %v", err)
	}
}

func TestBoesesPaket(t *testing.T) {
	cases := map[string]func(*Index){
		"Pfad nach oben":   func(ix *Index) { ix.Files[0].Path = "../../evil.jar" },
		"absoluter Pfad":   func(ix *Index) { ix.Files[0].Path = "/etc/evil" },
		"Windows-Pfad":     func(ix *Index) { ix.Files[0].Path = `mods\..\..\evil` },
		"fremder Server":   func(ix *Index) { ix.Files[0].Downloads = []string{"https://evil.example/a.jar"} },
		"http statt https": func(ix *Index) { ix.Files[0].Downloads = []string{"http://cdn.modrinth.com/a.jar"} },
		"ohne Prüfsumme":   func(ix *Index) { ix.Files[0].Hashes = map[string]string{"sha1": "x"} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			ix := Index{FormatVersion: 1, Game: "minecraft", Dependencies: map[string]string{"minecraft": "1", "fabric-loader": "1"},
				Files: []File{{Path: "mods/a.jar", Hashes: map[string]string{"sha512": strings.Repeat("0", 128)}, Downloads: []string{"https://cdn.modrinth.com/a.jar"}}}}
			mutate(&ix)
			if err := ix.validate(); err == nil {
				t.Error("Paket wurde akzeptiert")
			}
		})
	}
	for _, p := range []string{"../x", "overrides/../../x"} {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		w, _ := zw.Create("modrinth.index.json")
		w.Write([]byte(`{"formatVersion":1,"game":"minecraft","dependencies":{"minecraft":"1","fabric-loader":"1"},"files":[]}`))
		w, _ = zw.Create("overrides/" + p)
		w.Write([]byte("x"))
		zw.Close()
		if _, err := ReadPack(buf.Bytes()); err == nil {
			t.Errorf("Override %q wurde akzeptiert", p)
		}
	}
}

func TestLatest(t *testing.T) {
	data, _ := buildPack(t, "2.0", nil, nil)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest.json":
			json.NewEncoder(w).Encode(Latest{Version: "2.0", Mrpack: "download/FinalClient-2.0.mrpack", SHA512: sha(data)})
		case "/download/FinalClient-2.0.mrpack":
			w.Write(data)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	l, err := FetchLatest(context.Background(), srv.Client(), srv.URL)
	if err != nil || l.Version != "2.0" {
		t.Fatal(l, err)
	}
	if pack, err := l.Download(context.Background(), srv.Client(), srv.URL); err != nil || pack.Index.VersionID != "2.0" {
		t.Fatal(err)
	}
	l.SHA512 = strings.Repeat("0", 128)
	if _, err := l.Download(context.Background(), srv.Client(), srv.URL); err == nil {
		t.Error("falsche Paket-Prüfsumme akzeptiert")
	}
	l.Mrpack = "https://evil.example/x.mrpack"
	if _, err := l.Download(context.Background(), srv.Client(), srv.URL); err == nil {
		t.Error("Paket von fremder Seite akzeptiert")
	}
}
