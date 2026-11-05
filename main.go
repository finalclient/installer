// FinalClient-Installer: richtet FinalClient im offiziellen Minecraft-Launcher ein.
//
// Ohne Argumente öffnet sich ein Fenster. Für Tests und Fortgeschrittene:
//
//	finalclient-installer -cli                     ohne Fenster
//	finalclient-installer -mrpack Datei.mrpack     lokales Paket statt Website
//	finalclient-installer -minecraft <Ordner>      anderer Minecraft-Ordner
package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/steedalot/finalclient-installer/internal/install"
)

//go:embed assets/icon.png
var icon []byte

type config struct {
	site, mrpack, mcDir string
}

func main() {
	var cfg config
	cli := flag.Bool("cli", false, "ohne Fenster installieren")
	flag.StringVar(&cfg.site, "site", install.DefaultSite, "Website mit latest.json")
	flag.StringVar(&cfg.mrpack, "mrpack", "", "lokale .mrpack-Datei statt Website")
	flag.StringVar(&cfg.mcDir, "minecraft", "", "Minecraft-Ordner (Standard: der des offiziellen Launchers)")
	flag.Parse()
	if cfg.mcDir == "" {
		dir, err := install.DefaultMinecraftDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		cfg.mcDir = dir
	}
	if *cli {
		if err := runCLI(cfg); err != nil {
			fmt.Fprintln(os.Stderr, "Fehler:", err)
			os.Exit(1)
		}
		return
	}
	runGUI(cfg)
}

var httpClient = &http.Client{Timeout: 5 * time.Minute}

// loadPack holt das Paket von der Website oder aus einer lokalen Datei.
func loadPack(ctx context.Context, cfg config) (*install.Pack, error) {
	if cfg.mrpack != "" {
		data, err := os.ReadFile(cfg.mrpack)
		if err != nil {
			return nil, err
		}
		return install.ReadPack(data)
	}
	latest, err := install.FetchLatest(ctx, httpClient, cfg.site)
	if err != nil {
		return nil, err
	}
	return latest.Download(ctx, httpClient, cfg.site)
}

func runCLI(cfg config) error {
	ctx := context.Background()
	pack, err := loadPack(ctx, cfg)
	if err != nil {
		return err
	}
	fmt.Printf("%s %s → %s\n", pack.Index.Name, pack.Index.VersionID, cfg.mcDir)
	err = install.Install(ctx, pack, install.Options{MinecraftDir: cfg.mcDir, Icon: icon, HTTP: httpClient},
		func(done, total int, msg string) { fmt.Printf("[%d/%d] %s\n", done, total, msg) })
	if err != nil {
		return err
	}
	fmt.Println("Fertig. Im Minecraft-Launcher das Profil „FinalClient“ wählen.")
	return nil
}
