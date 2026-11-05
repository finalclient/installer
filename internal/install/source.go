package install

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

// DefaultSite ist die FinalClient-Website. Dort liegt latest.json mit der aktuellen Version.
const DefaultSite = "https://client.finalsmp.net"

// Latest beschreibt die aktuelle Version auf der Website.
type Latest struct {
	Version string `json:"version"`
	Mrpack  string `json:"mrpack"` // relativ zur Website
	SHA512  string `json:"sha512"`
}

// FetchLatest fragt die Website nach der aktuellen Version.
func FetchLatest(ctx context.Context, client *http.Client, site string) (Latest, error) {
	var l Latest
	body, err := get(ctx, client, site+"/latest.json", 64<<10)
	if err != nil {
		return l, fmt.Errorf("Website nicht erreichbar: %w", err)
	}
	if err := json.Unmarshal(body, &l); err != nil {
		return l, fmt.Errorf("latest.json: %w", err)
	}
	if l.Version == "" || l.Mrpack == "" || len(l.SHA512) != 128 {
		return l, fmt.Errorf("latest.json ist unvollständig")
	}
	return l, nil
}

// Download lädt das Paket von der Website und prüft es gegen die Prüfsumme aus latest.json.
func (l Latest) Download(ctx context.Context, client *http.Client, site string) (*Pack, error) {
	base, err := url.Parse(site + "/")
	if err != nil {
		return nil, err
	}
	ref, err := url.Parse(l.Mrpack)
	if err != nil {
		return nil, err
	}
	u := base.ResolveReference(ref)
	if u.Host != base.Host { // Paket nur von der eigenen Website
		return nil, fmt.Errorf("Paket-Adresse zeigt auf fremde Seite: %s", u)
	}
	data, err := get(ctx, client, u.String(), 64<<20)
	if err != nil {
		return nil, err
	}
	sum := sha512.Sum512(data)
	if hex.EncodeToString(sum[:]) != l.SHA512 {
		return nil, fmt.Errorf("Prüfsumme des Pakets stimmt nicht")
	}
	return ReadPack(data)
}
