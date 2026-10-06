package blobber

import (
	"bytes"
	"embed"
	"image/png"

	"github.com/hajimehoshi/ebiten/v2"
)

//go:embed assets/walls/*.png
var wallFS embed.FS

// wallTexture loads assets/walls/<name>.png, or nil when there is none; a painted texture
// replaces the generated one for its material character.
func wallTexture(name string) *ebiten.Image {
	b, err := wallFS.ReadFile("assets/walls/" + name + ".png")
	if err != nil {
		return nil
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return nil
	}
	return ebiten.NewImageFromImage(img)
}
