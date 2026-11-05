// Package install richtet ein Modrinth-Paket (.mrpack) als Profil im
// offiziellen Minecraft-Launcher ein. Es kennt keine FinalClient-Funktionen:
// Alles, was installiert wird, steht im Paket.
package install

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
)

// Pack ist ein geöffnetes .mrpack.
type Pack struct {
	Index     Index
	Overrides map[string][]byte // Pfad im Spielordner → Inhalt
}

// Index ist die modrinth.index.json (https://support.modrinth.com/en/articles/8802351).
type Index struct {
	FormatVersion int               `json:"formatVersion"`
	Game          string            `json:"game"`
	VersionID     string            `json:"versionId"`
	Name          string            `json:"name"`
	Dependencies  map[string]string `json:"dependencies"`
	Files         []File            `json:"files"`
}

type File struct {
	Path      string            `json:"path"`
	Hashes    map[string]string `json:"hashes"`
	Env       map[string]string `json:"env"`
	Downloads []string          `json:"downloads"`
	FileSize  int64             `json:"fileSize"`
}

// Erlaubte Download-Quellen, wie im mrpack-Format vorgegeben.
var allowedHosts = map[string]bool{
	"cdn.modrinth.com":          true,
	"github.com":                true,
	"raw.githubusercontent.com": true,
	"gitlab.com":                true,
}

// ReadPack öffnet ein .mrpack und prüft alles, was später auf die Platte geht.
func ReadPack(data []byte) (*Pack, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("Paket ist keine gültige .mrpack-Datei: %w", err)
	}
	p := &Pack{Overrides: map[string][]byte{}}
	foundIndex := false
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		switch {
		case f.Name == "modrinth.index.json":
			b, err := readZip(f)
			if err != nil {
				return nil, err
			}
			if err := json.Unmarshal(b, &p.Index); err != nil {
				return nil, fmt.Errorf("modrinth.index.json: %w", err)
			}
			foundIndex = true
		case strings.HasPrefix(f.Name, "overrides/"), strings.HasPrefix(f.Name, "client-overrides/"):
			rel := f.Name[strings.Index(f.Name, "/")+1:]
			if err := checkPath(rel); err != nil {
				return nil, err
			}
			b, err := readZip(f)
			if err != nil {
				return nil, err
			}
			// client-overrides gewinnt gegen overrides
			if _, exists := p.Overrides[rel]; !exists || strings.HasPrefix(f.Name, "client-") {
				p.Overrides[rel] = b
			}
		}
	}
	if !foundIndex {
		return nil, errors.New("Paket enthält keine modrinth.index.json")
	}
	return p, p.Index.validate()
}

func (ix Index) validate() error {
	if ix.FormatVersion != 1 || ix.Game != "minecraft" {
		return fmt.Errorf("unbekanntes Paketformat (formatVersion %d, game %q)", ix.FormatVersion, ix.Game)
	}
	if ix.Dependencies["minecraft"] == "" || ix.Dependencies["fabric-loader"] == "" {
		return errors.New("Paket braucht Minecraft und Fabric als Abhängigkeit")
	}
	for _, f := range ix.Files {
		if err := checkPath(f.Path); err != nil {
			return err
		}
		if len(f.Hashes["sha512"]) != 128 {
			return fmt.Errorf("%s: sha512-Prüfsumme fehlt", f.Path)
		}
		if len(f.Downloads) == 0 {
			return fmt.Errorf("%s: keine Download-Adresse", f.Path)
		}
		for _, d := range f.Downloads {
			u, err := url.Parse(d)
			if err != nil || u.Scheme != "https" || !allowedHosts[u.Hostname()] {
				return fmt.Errorf("%s: Download-Adresse nicht erlaubt: %s", f.Path, d)
			}
		}
	}
	return nil
}

// ClientFiles sind alle Dateien, die der Client braucht (optionale werden mitgenommen).
func (ix Index) ClientFiles() []File {
	var out []File
	for _, f := range ix.Files {
		if f.Env["client"] != "unsupported" {
			out = append(out, f)
		}
	}
	return out
}

// checkPath verhindert, dass ein Paket außerhalb des Spielordners schreibt.
func checkPath(p string) error {
	clean := path.Clean(p)
	if p == "" || clean != p || path.IsAbs(p) || strings.HasPrefix(clean, "../") || clean == ".." ||
		strings.ContainsAny(p, `\:`) {
		return fmt.Errorf("unerlaubter Pfad im Paket: %q", p)
	}
	return nil
}

func readZip(f *zip.File) ([]byte, error) {
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(io.LimitReader(r, 256<<20))
}
