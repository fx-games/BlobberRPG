package blobber

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io/fs"
	"path"

	"github.com/hajimehoshi/ebiten/v2"
)

// Anim is one row of a sprite sheet: frames with their durations in seconds.
type Anim struct {
	Frames    []*ebiten.Image
	Durations []float64
	Loop      bool
}

// Sheet is an animated monster: "idle" plus optional "attack", "hurt" and "death" rows.
type Sheet struct{ Anims map[string]*Anim }

// Idle is the still picture of a sheet: its first idle frame.
func (s *Sheet) Idle() *ebiten.Image { return s.Anims["idle"].Frames[0] }

// LoadSheet reads dir/manifest.json and dir/sheet.png from fsys. The manifest's "animation" block
// names the cell size and, per row, its index, frame count, durations_ms and loop flag.
func LoadSheet(fsys fs.FS, dir string) (*Sheet, error) {
	var m struct {
		Animation struct {
			CellWidth, CellHeight int
			Rows                  map[string]struct {
				Row, Frames int
				DurationsMs []int `json:"durations_ms"`
				Loop        bool
			}
		}
	}
	b, err := fs.ReadFile(fsys, path.Join(dir, "manifest.json"))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("%s/manifest.json: %v", dir, err)
	}
	f, err := fsys.Open(path.Join(dir, "sheet.png"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s/sheet.png: %v", dir, err)
	}
	sheet := ebiten.NewImageFromImage(img)
	a := m.Animation
	s := &Sheet{Anims: map[string]*Anim{}}
	for name, r := range a.Rows {
		an := &Anim{Loop: r.Loop}
		for i := range r.Frames {
			rect := image.Rect(i*a.CellWidth, r.Row*a.CellHeight, (i+1)*a.CellWidth, (r.Row+1)*a.CellHeight)
			an.Frames = append(an.Frames, sheet.SubImage(rect).(*ebiten.Image))
			d := 0.125
			if i < len(r.DurationsMs) {
				d = float64(r.DurationsMs[i]) / 1000
			}
			an.Durations = append(an.Durations, d)
		}
		if len(an.Frames) > 0 {
			s.Anims[name] = an
		}
	}
	if s.Anims["idle"] == nil {
		return nil, fmt.Errorf("%s: manifest has no idle row", dir)
	}
	return s, nil
}
