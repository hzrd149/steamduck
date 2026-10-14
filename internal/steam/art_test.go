package steam

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func testIcon(t *testing.T) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{200, 40, 40, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestWriteArtRespectsUserArt(t *testing.T) {
	dir := t.TempDir()
	grid := filepath.Join(dir, "grid")
	st, _ := LoadArtState(filepath.Join(dir, "art.json"))
	// the user already chose a portrait capsule in Steam
	_ = os.MkdirAll(grid, 0o755)
	userArt := filepath.Join(grid, "2147483649p.png")
	_ = os.WriteFile(userArt, []byte("mine"), 0o644)

	if err := st.WriteArt(grid, 2147483649, testIcon(t)); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(userArt); string(b) != "mine" {
		t.Fatal("user art replaced")
	}
	wide := filepath.Join(grid, "2147483649.png")
	f, err := os.Open(wide)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := png.DecodeConfig(f)
	f.Close()
	if err != nil || cfg.Width != 920 || cfg.Height != 430 {
		t.Fatalf("wide capsule %+v %v", cfg, err)
	}
	st.RemoveOwned(grid, 2147483649)
	if _, err := os.Stat(wide); !os.IsNotExist(err) {
		t.Fatal("owned art not removed")
	}
	if _, err := os.Stat(userArt); err != nil {
		t.Fatal("user art removed")
	}
}
