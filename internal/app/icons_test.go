package app

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image"
	"image/png"
	"os"
	"testing"
)

func TestTrayIconUsesOfficialCueDexArtwork(t *testing.T) {
	data := trayIcon()
	if !bytes.Equal(data, officialTrayIcon) {
		t.Fatal("trayIcon did not return the embedded official image bytes")
	}
	// Pin the original icon_32x32.png bytes, not just an arbitrary valid PNG.
	const wantSHA256 = "955288a29160caf1070b9bd9cdcbc6471593e0ef70dfeb1f23a5448e7be0210a"
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != wantSHA256 {
		t.Fatalf("tray icon SHA256 = %s, want official artwork %s", got, wantSHA256)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode trayIcon PNG: %v", err)
	}
	if got, want := img.Bounds(), image.Rect(0, 0, 32, 32); got != want {
		t.Fatalf("tray icon bounds = %v, want %v", got, want)
	}
}

func TestPackagingIconUsesOfficialCueDexArtwork(t *testing.T) {
	data, err := os.ReadFile("../../resources/icon.png")
	if err != nil {
		t.Fatalf("read packaging icon: %v", err)
	}
	const wantSHA256 = "09db888b1c657eae3dc8f794232dbfe8ca79e4d615a7f1b6f02231916af33aea"
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != wantSHA256 {
		t.Fatalf("packaging icon SHA256 = %s, want official artwork %s", got, wantSHA256)
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode packaging icon PNG: %v", err)
	}
	if got, want := img.Bounds(), image.Rect(0, 0, 1024, 1024); got != want {
		t.Fatalf("packaging icon bounds = %v, want %v", got, want)
	}
}
