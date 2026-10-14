package steam

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"

	"golang.org/x/image/draw"
)

// Artwork Steam shows for a shortcut, by grid file suffix. Kwakore only
// gives a square icon, so each image is the icon centered on a background
// tinted from it.
var artSizes = []struct {
	suffix string
	w, h   int
	scale  float64 // icon size as a fraction of the shorter side
}{
	{"p", 600, 900, 0.6},      // library capsule (portrait)
	{"", 920, 430, 0.7},       // wide capsule, also recent games
	{"_hero", 1920, 620, 0.5}, // header behind the game page
}

// ArtState remembers the hash of every grid file steamduck wrote, so it
// only replaces its own images and never art the user picked in Steam.
type ArtState struct {
	path    string
	Written map[string]string `json:"written"` // file path -> sha256
}

// LoadArtState reads the state file at path; a missing file is empty.
func LoadArtState(path string) (*ArtState, error) {
	s := &ArtState{path: path, Written: map[string]string{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if s.Written == nil {
		s.Written = map[string]string{}
	}
	return s, nil
}

// Save writes the state file.
func (s *ArtState) Save() error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

// writeOwned writes data to path unless the file exists and is not one
// steamduck wrote.
func (s *ArtState) writeOwned(path string, data []byte) error {
	if cur, err := os.ReadFile(path); err == nil {
		if s.Written[path] != sum(cur) {
			return nil // the user's own art
		}
		if bytes.Equal(cur, data) {
			return nil
		}
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	s.Written[path] = sum(data)
	return nil
}

// RemoveOwned deletes the grid files steamduck wrote for appID.
func (s *ArtState) RemoveOwned(gridDir string, appID uint32) {
	for _, a := range artSizes {
		p := filepath.Join(gridDir, strconv.FormatUint(uint64(appID), 10)+a.suffix+".png")
		if cur, err := os.ReadFile(p); err == nil && s.Written[p] == sum(cur) {
			_ = os.Remove(p)
		}
		delete(s.Written, p)
	}
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// WriteArt renders and writes the grid images for appID from a PNG icon.
func (s *ArtState) WriteArt(gridDir string, appID uint32, iconPNG []byte) error {
	icon, err := png.Decode(bytes.NewReader(iconPNG))
	if err != nil {
		return fmt.Errorf("decode icon: %w", err)
	}
	if err := os.MkdirAll(gridDir, 0o755); err != nil {
		return err
	}
	bg := backdrop(icon)
	for _, a := range artSizes {
		var buf bytes.Buffer
		if err := png.Encode(&buf, compose(icon, bg, a.w, a.h, a.scale)); err != nil {
			return err
		}
		p := filepath.Join(gridDir, strconv.FormatUint(uint64(appID), 10)+a.suffix+".png")
		if err := s.writeOwned(p, buf.Bytes()); err != nil {
			return err
		}
	}
	return nil
}

// backdrop is the icon's average opaque color, darkened so the icon stands
// out against it.
func backdrop(img image.Image) color.RGBA {
	var r, g, b, n uint64
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			cr, cg, cb, ca := img.At(x, y).RGBA()
			if ca < 0x8000 {
				continue
			}
			r, g, b, n = r+uint64(cr>>8), g+uint64(cg>>8), b+uint64(cb>>8), n+1
		}
	}
	if n == 0 {
		return color.RGBA{0x1b, 0x28, 0x38, 0xff}
	}
	return color.RGBA{uint8(r / n * 2 / 5), uint8(g / n * 2 / 5), uint8(b / n * 2 / 5), 0xff}
}

func compose(icon image.Image, bg color.RGBA, w, h int, scale float64) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	// vertical gradient from bg to a darker shade
	for y := 0; y < h; y++ {
		f := 1 - 0.5*float64(y)/float64(h)
		c := color.RGBA{uint8(float64(bg.R) * f), uint8(float64(bg.G) * f), uint8(float64(bg.B) * f), 0xff}
		for x := 0; x < w; x++ {
			dst.SetRGBA(x, y, c)
		}
	}
	side := int(float64(min(w, h)) * scale)
	x0, y0 := (w-side)/2, (h-side)/2
	draw.CatmullRom.Scale(dst, image.Rect(x0, y0, x0+side, y0+side), icon, icon.Bounds(), draw.Over, nil)
	return dst
}
