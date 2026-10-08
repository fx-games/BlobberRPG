package blobber

// Deepnight's "Labyrinth" look on the 3D quads: everything darkens in hard, dithered bands toward a
// purple fog instead of fading smoothly to black. Light maps (NAME_light.png) keep parts of a wall
// lit and put a glow in front of it; moving lights draw as additive glows with light circles on the
// floor and ceiling.

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
)

var fogCol = color.RGBA{0x16, 0x10, 0x23, 255}

// mat is a wall material: its front texture, the generated ones' left and right lit variants
// (nil for painted textures, which use img on every face), and an optional light map.
type mat struct {
	img, right, left, lit *ebiten.Image
	loaded                bool
	// light map summary (TextureAtlas): average colour weighted by brightness, the glow's height
	// (1 top) and strength (1 when 3% of the texture is lit)
	lightCol           [3]float64
	lightHei, lightPow float64
}

// face picks the texture for a face whose normal's lateral component in camera space is nx.
func (m *mat) face(nx float64) *ebiten.Image {
	if nx > 0.5 && m.right != nil {
		return m.right
	} else if nx < -0.5 && m.left != nil {
		return m.left
	}
	return m.img
}

func lum(r, g, b byte) float64 { return (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 255 }

// load reads the light map back once for its summary; a light map of another size is dropped.
func (m *mat) load() *mat {
	if m.loaded {
		return m
	}
	m.loaded = true
	b := m.img.Bounds()
	w, h := b.Dx(), b.Dy()
	if m.lit == nil || m.lit.Bounds().Size() != b.Size() {
		m.lit = nil
		return m
	}
	pix := make([]byte, 4*w*h)
	m.lit.ReadPixels(pix)
	var n, ys float64
	for i := 0; i < w*h; i++ {
		if l := lum(pix[4*i], pix[4*i+1], pix[4*i+2]); l > 0.1 {
			n += l
			ys += l * float64(i/w)
			for c := range 3 {
				m.lightCol[c] += l * float64(pix[4*i+c]) / 255
			}
		}
	}
	if n > 0 {
		for c := range 3 {
			m.lightCol[c] /= n
		}
		m.lightHei = 1 - ys/n/float64(h)
		m.lightPow = math.Min(1, n/float64(w*h)/0.03)
	}
	return m
}

// darkMul is how strongly distance darkens: 1 at torch 1, less with a brighter torch or a Light
// spell, 0 in daylight.
func (g *Game) darkMul() float64 {
	if openSky {
		return 0
	}
	return 1 / (g.torch + math.Min(1.5, lightLeft)*0.5)
}

// darkness is Deepnight's ramped distance darkness: 7 hard bands, scaled by mul. The shader has a copy.
func darkness(dist, mul float64) float64 {
	r := dist / 5.7
	if r >= 0.6 {
		r += 0.04
	}
	if r >= 0.4 {
		r += 0.08
	}
	if r >= 0.2 {
		r += 0.04
	}
	return math.Max(0, math.Min(1, math.Floor(r*r*7)/7*mul))
}

// Face modes for the texture shader.
const (
	modeShaded = iota // darkened by distance, lit by the moving lights
	modeUnlit         // drawn as is (signs, self-lit sprites)
	modeGlow          // additive, tinted by the vertex colour
)

// texSrc: perspective-correct texturing (custom = u/z, v/z, 1/z, y/z), then Deepnight's banded
// darkness with fog tint.
const texSrc = `//kage:unit pixels
package main

var Mode float
var Mul float     // darkness strength
var Inc float     // constant extra fade toward the fog: 0.3 sides, 0.4 floors, 0 fronts
var HasLit float  // 1 when image 2 is a light map
var Center vec2   // screen centre
var Focal float
var Px float      // screen pixels per view pixel of theirs (500 wide), for the checker dither

func tex(uv vec2) vec4 { return imageSrc0At(imageSrc0Origin() + fract(uv)*imageSrc0Size()) }

func darkness(d, mul float) float {
	r := d / 5.7
	if r >= 0.6 {
		r += 0.04
	}
	if r >= 0.4 {
		r += 0.08
	}
	if r >= 0.2 {
		r += 0.04
	}
	return clamp(floor(r*r*7)/7*mul, 0, 1)
}

func shade(c vec3, dark, inc float) vec3 {
	fog := vec3(0x16, 0x10, 0x23) / 255
	r := clamp(dark/0.6, 0, 1)
	l := dot(c, vec3(0.299, 0.587, 0.114))
	t := fog
	if l*2 > r {
		t = vec3(0x25, 0x1f, 0x32) / 255
	} else if l*4 > r {
		t = vec3(0x1e, 0x18, 0x2b) / 255
	}
	c = mix(c, t, r)
	return mix(c, fog, clamp(clamp((dark-0.7)/0.3, 0, 1)*0.6+inc, 0, 1))
}

func Fragment(dstPos vec4, srcPos vec2, color vec4, custom vec4) vec4 {
	z := 1 / custom.z
	uv := custom.xy * z
	if Mode == 2 {
		return tex(uv) * color
	}
	c := tex(uv)
	if c.a == 0 {
		discard()
	}
	if Mode == 1 {
		return c
	}
	rgb := c.rgb / c.a
	x := (dstPos.x - Center.x) / Focal * z
	h := 0.5 - custom.w*z // height above the floor
	q := floor(dstPos.xy / Px)
	chk := mod(q.x+q.y, 2)
	dist := length(vec2(x*1.5, z*0.7)) + (h+chk*0.15)*1.8
	lit := 0.0
	if HasLit > 0 {
		lit = imageSrc2At(imageSrc2Origin() + fract(uv)*imageSrc2Size()).r
	}
	// the moving lights (vertex colour) push the darkness back and tint what they reach
	near := clamp(max(color.r, max(color.g, color.b)), 0, 1)
	out := shade(rgb, darkness(dist, Mul*(1-0.7*lit)*(1-near)), Inc*(1-lit)*(1-near)) + rgb*color.rgb*0.3
	return vec4(out*c.a, c.a)
}
`

// faceOptions sets the texture shader's images and uniforms for face f seen from c.
func (g *Game) faceOptions(c cam, f *face) *ebiten.DrawTrianglesShaderOptions {
	op := &ebiten.DrawTrianglesShaderOptions{}
	op.Images[0], op.Images[1], op.Images[2] = f.tex, f.tex, f.tex
	u := map[string]any{"Mode": float32(f.mode), "Mul": float32(g.darkMul()), "Inc": float32(f.inc),
		"Center": []float32{float32(W) / 2, float32(H) / 2}, "Focal": float32(focal), "Px": float32(math.Max(1, float64(W)/500))}
	if m := f.m; m != nil && m.lit != nil {
		op.Images[2], u["HasLit"] = m.lit, float32(1)
	}
	if f.mode == modeGlow {
		op.Blend = ebiten.BlendLighter
	}
	op.Uniforms = u
	return op
}
