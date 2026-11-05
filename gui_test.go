package main

import (
	"image/png"
	"os"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// Mit GUI_SHOT=<Ordner> legt der Test Bilder des Fensters ab (vorher und nachher).
func TestFenster(t *testing.T) {
	mc := t.TempDir()
	os.WriteFile(mc+"/launcher_profiles.json", []byte(`{"profiles":{}}`), 0o644)
	pack := os.Getenv("GUI_PACK")
	if pack == "" {
		t.Skip("GUI_PACK=<Datei.mrpack> setzen")
	}
	a := test.NewApp()
	w := newWindow(a, config{mrpack: pack, mcDir: mc})
	w.Show()
	shot := func(name string) {
		if dir := os.Getenv("GUI_SHOT"); dir != "" {
			f, _ := os.Create(dir + "/" + name + ".png")
			png.Encode(f, w.Canvas().Capture())
			f.Close()
		}
	}
	time.Sleep(500 * time.Millisecond)
	shot("bereit")
	var btn fyne.Tappable
	for _, o := range test.LaidOutObjects(w.Content()) {
		if b, ok := o.(fyne.Tappable); ok {
			btn = b
		}
	}
	test.Tap(btn)
	time.Sleep(30 * time.Second)
	shot("fertig")
	for _, o := range test.LaidOutObjects(w.Content()) {
		if l, ok := o.(*widget.Label); ok {
			t.Logf("Label: %q size=%v", l.Text, l.Size())
		}
	}
	if _, err := os.Stat(mc + "/finalclient/.finalclient-installer.json"); err != nil {
		t.Error("nicht installiert")
	}
}
