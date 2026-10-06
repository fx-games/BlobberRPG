package blobber

import (
	"fmt"
	"image/png"
	"io/fs"
	"path"

	"github.com/hajimehoshi/ebiten/v2"
)

// Sheet is a monster's pictures: an idle pose, plus attack and hurt poses when it has them.
// Size is how tall it stands in tiles (0 = 0.6); it is never wider than max(1, Size) tiles.
type Sheet struct {
	Idle, Attack, Hurt *ebiten.Image
	Size               float64
}

// PoseTime is how long an attack or hurt pose shows before idle returns (or a dying monster goes).
const PoseTime = 0.45

// LoadSheet reads dir/idle.png and the optional dir/attack.png and dir/hurt.png from fsys.
func LoadSheet(fsys fs.FS, dir string) (*Sheet, error) {
	load := func(name string) (*ebiten.Image, error) {
		f, err := fsys.Open(path.Join(dir, name+".png"))
		if err != nil {
			return nil, err
		}
		defer f.Close()
		img, err := png.Decode(f)
		if err != nil {
			return nil, fmt.Errorf("%s/%s.png: %v", dir, name, err)
		}
		return ebiten.NewImageFromImage(img), nil
	}
	idle, err := load("idle")
	if err != nil {
		return nil, err
	}
	s := &Sheet{Idle: idle}
	s.Attack, _ = load("attack")
	s.Hurt, _ = load("hurt")
	return s, nil
}
