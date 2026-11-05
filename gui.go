package main

import (
	"context"
	"errors"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/steedalot/finalclient-installer/internal/install"
)

// Ein Fenster, ein Knopf. Alles andere passiert von selbst.
func runGUI(cfg config) {
	a := app.NewWithID("io.gaida.finalclient.installer")
	w := newWindow(a, cfg)
	w.CenterOnScreen()
	w.ShowAndRun()
}

func newWindow(a fyne.App, cfg config) fyne.Window {
	w := a.NewWindow("FinalClient-Installer")
	res := fyne.NewStaticResource("icon.png", icon)
	w.SetIcon(res)

	logo := canvas.NewImageFromResource(res)
	logo.FillMode = canvas.ImageFillContain
	logo.SetMinSize(fyne.NewSize(96, 96))
	title := widget.NewLabelWithStyle("FinalClient", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	status := widget.NewLabel("Suche aktuelle Version …")
	status.Alignment = fyne.TextAlignCenter
	status.Wrapping = fyne.TextWrapWord
	hint := widget.NewLabel("Schließe den Minecraft-Launcher, bevor du installierst.")
	hint.Alignment = fyne.TextAlignCenter
	hint.Wrapping = fyne.TextWrapWord
	hint.Importance = widget.LowImportance
	bar := widget.NewProgressBar()
	bar.Hide()
	button := widget.NewButtonWithIcon("Installieren", theme.DownloadIcon(), nil)
	button.Importance = widget.HighImportance
	button.Disable()

	var pack *install.Pack
	ctx := context.Background()

	showReady := func() {
		installed := install.InstalledVersion(cfg.mcDir)
		switch {
		case installed == "":
			status.SetText(fmt.Sprintf("Version %s ist bereit.", pack.Index.VersionID))
			button.SetText("Installieren")
		case installed == pack.Index.VersionID:
			status.SetText(fmt.Sprintf("Version %s ist schon installiert.", installed))
			button.SetText("Reparieren")
		default:
			status.SetText(fmt.Sprintf("Update von %s auf %s.", installed, pack.Index.VersionID))
			button.SetText("Aktualisieren")
		}
		button.Enable()
	}
	showError := func(err error) {
		if errors.Is(err, install.ErrNoLauncher) {
			status.SetText("Kein Minecraft-Launcher gefunden. Starte ihn einmal, melde dich an und schließe ihn wieder. Dann hier noch mal versuchen.")
		} else {
			status.SetText("Das hat nicht geklappt: " + err.Error())
		}
	}

	button.OnTapped = func() {
		button.Disable()
		bar.SetValue(0)
		bar.Show()
		go func() {
			err := install.Install(ctx, pack, install.Options{MinecraftDir: cfg.mcDir, Icon: icon, HTTP: httpClient},
				func(done, total int, msg string) {
					fyne.Do(func() {
						bar.SetValue(float64(done) / float64(total))
						status.SetText(msg)
					})
				})
			fyne.Do(func() {
				if err != nil {
					showError(err)
					button.SetText("Noch mal versuchen")
					button.Enable()
					return
				}
				bar.Hide()
				hint.Hide()
				status.SetText("Fertig! Öffne den Minecraft-Launcher, wähle das Profil „FinalClient“ und klicke auf Spielen.")
				button.SetText("Schließen")
				button.SetIcon(theme.ConfirmIcon())
				button.OnTapped = a.Quit
				button.Enable()
			})
		}()
	}

	go func() {
		p, err := loadPack(ctx, cfg)
		fyne.Do(func() {
			if err != nil {
				showError(err)
				return
			}
			pack = p
			showReady()
		})
	}()

	w.SetContent(container.NewPadded(container.NewVBox(
		logo, title, status, layout.NewSpacer(), bar, button, hint,
	)))
	w.Resize(fyne.NewSize(420, 360))
	w.SetFixedSize(true)
	return w
}
