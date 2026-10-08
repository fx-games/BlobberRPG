package blobber

// Flip-screen renderer, after Deepnight's "Labyrinth": the view is grid-locked, and every wall, floor
// and ceiling piece is baked once per cell offset on the CPU into a pre-projected sprite, heightmap
// extrusion and stepped darkness included. A move or turn is a 2D warp of the last frame into the new one.

import (
	"image/color"
	"math"
	"math/rand"
	"sort"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// RW, RH is the size the world is rendered at before it is scaled to the window.
const RW, RH = 480, 360

// Camera, as Deepnight's FlipCamSettings: depth falls off as (z+1)^zMul instead of 1/z, which
// stretches the near cells and squeezes the far ones.
const (
	fovX             = 0.61
	closeHei, farHei = 0.40, 0.46 // eye height in wall heights; horizon as a share of RH
	zMul             = 1.96
	maxDZ            = 5          // cells drawn ahead; darkness reaches full at about this depth
	skyDZ            = 14         // in daylight nothing hides the distance
	extrude          = 20.0 / 256 // full heightmap extrusion of a side wall, of the wall's height on screen
)

var (
	fovY   = 2 * math.Atan(math.Tan(fovX/2)*RH/RW*150/128) // their wall texture is 150x128
	kx, ky = RW / 2 / math.Tan(fovX/2), RH / 2 / math.Tan(fovY/2)
	fogCol = color.RGBA{0x16, 0x10, 0x23, 255}
	fogMed = color.RGBA{0x1e, 0x18, 0x2b, 255}
	fogLit = color.RGBA{0x25, 0x1f, 0x32, 255}
)

// scrX is the screen x of column edge a (column c spans a = c..c+1, the eye at 0.5) at depth z
// (cell k spans z = k..k+1, the party's cell is 0). scrY is the screen y of height h (0 floor, 1 ceiling).
func scrX(a, z float64) float64 { return RW/2 + (a-0.5)*kx/math.Pow(z+1, zMul) }
func scrY(z, h float64) float64 { return RH*farHei + (closeHei-h)*ky/math.Pow(z+1, zMul) }

// mat is a texture with its heightmap, read back to the CPU for baking.
type mat struct {
	img, hei *ebiten.Image
	w, h     int
	pix      []byte
	height   []float64
	darkest  color.RGBA // darkest colour with luminance >= 0.1: the colour of extruded sides
}

func lum(c color.RGBA) float64 {
	return (0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)) / 255
}

func (m *mat) load() {
	if m.pix != nil {
		return
	}
	b := m.img.Bounds()
	m.w, m.h = b.Dx(), b.Dy()
	m.pix = make([]byte, 4*m.w*m.h)
	m.img.ReadPixels(m.pix)
	m.height = make([]float64, m.w*m.h)
	if m.hei != nil && m.hei.Bounds() == b {
		hp := make([]byte, 4*m.w*m.h)
		m.hei.ReadPixels(hp)
		for i := range m.height {
			m.height[i] = float64(hp[4*i]) / 255
		}
	}
	m.darkest = color.RGBA{255, 255, 255, 255}
	for i := 0; i < m.w*m.h; i++ {
		c := color.RGBA{m.pix[4*i], m.pix[4*i+1], m.pix[4*i+2], 255}
		if l := lum(c); l >= 0.1 && l < lum(m.darkest) {
			m.darkest = c
		}
	}
}

// at samples colour and height at (u, v), wrapping.
func (m *mat) at(u, v float64) (color.RGBA, float64) {
	x, y := int(math.Floor(u*float64(m.w))), int(math.Floor(v*float64(m.h)))
	x, y = (x%m.w+m.w)%m.w, (y%m.h+m.h)%m.h
	i := y*m.w + x
	return color.RGBA{m.pix[4*i], m.pix[4*i+1], m.pix[4*i+2], 255}, m.height[i]
}

func mix(a, b color.RGBA, t float64) color.RGBA {
	t = clamp01(t)
	f := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t + 0.5) }
	return color.RGBA{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), 255}
}

func clamp01(x float64) float64 { return math.Max(0, math.Min(1, x)) }

// darkness is Deepnight's ramped distance darkness: 7 hard bands, scaled by mul (0 = daylight).
func darkness(dist, mul float64) float64 {
	r := dist / (maxDZ + 0.7)
	if r >= 0.6 {
		r += 0.04
	}
	if r >= 0.4 {
		r += 0.08
	}
	if r >= 0.2 {
		r += 0.04
	}
	return clamp01(math.Floor(r*r*7) / 7 * mul)
}

// shade tints c toward the fog as it darkens (bright pixels toward a lighter fog, so details stay
// readable), then fades it out past 0.7; inc is a constant extra fade for sides and floors.
func shade(c color.RGBA, dark, inc float64) color.RGBA {
	r := clamp01(dark / 0.6)
	l := lum(c)
	t := fogCol
	if l*2 > r {
		t = fogLit
	} else if l*4 > r {
		t = fogMed
	}
	c = mix(c, t, r)
	return mix(c, fogCol, clamp01((dark-0.7)/0.3)*0.6+inc)
}

func checker(x, y int) float64 { return float64((x + y) & 1) }

// canvas is a baked piece: pixels for the screen rectangle x0,y0..x1,y1.
type canvas struct {
	x0, y0, x1, y1 int
	pix            []byte
	set            []bool
}

func newCanvas(x0, y0, x1, y1 float64) *canvas {
	c := &canvas{max(0, int(math.Floor(x0))), max(0, int(math.Floor(y0))), min(RW, int(math.Ceil(x1))), min(RH, int(math.Ceil(y1))), nil, nil}
	if c.x1 <= c.x0 || c.y1 <= c.y0 {
		return nil
	}
	n := (c.x1 - c.x0) * (c.y1 - c.y0)
	c.pix, c.set = make([]byte, 4*n), make([]bool, n)
	return c
}

func (c *canvas) idx(x, y int) int {
	if x < c.x0 || x >= c.x1 || y < c.y0 || y >= c.y1 {
		return -1
	}
	return (y-c.y0)*(c.x1-c.x0) + x - c.x0
}

func (c *canvas) put(i int, col color.RGBA) {
	c.pix[4*i], c.pix[4*i+1], c.pix[4*i+2], c.pix[4*i+3] = col.R, col.G, col.B, 255
	c.set[i] = true
}

// sprite is a baked piece placed on the view.
type sprite struct {
	img  *ebiten.Image
	x, y int
}

func (c *canvas) sprite() *sprite {
	if c == nil {
		return nil
	}
	img := ebiten.NewImage(c.x1-c.x0, c.y1-c.y0)
	img.WritePixels(c.pix)
	return &sprite{img, c.x0, c.y0}
}

// sideWall bakes the wall face along column edge a over cell depth rz..rz+1 (TextureBuilder.makeSideWall).
// Each texel is smeared toward the corridor by its height; the smear is the brick's side, shaded toward
// the texture's darkest colour with noise. Near columns are drawn first and keep their pixels.
// bevel slopes the height to 0 at the far end, where the wall stops.
func sideWall(m *mat, a, rz int, bevel bool, mul float64) *sprite {
	m.load()
	af, z0 := float64(a), float64(rz)
	xn, xf := scrX(af, z0), scrX(af, z0+1)
	dir := 1 // smear toward the centre of the view
	if xn > xf {
		dir = -1
	}
	maxOff := extrude * bump * (scrY(z0, 0) - scrY(z0, 1))
	lo, hi := math.Min(xn, xf), math.Max(xn, xf)
	if dir > 0 {
		hi += maxOff
	} else {
		lo -= maxOff
	}
	c := newCanvas(lo, scrY(z0, 1), hi, scrY(z0, 0))
	if c == nil {
		return nil
	}
	rnd := rand.New(rand.NewSource(int64(a*31 + rz)))
	start, end := int(math.Floor(xn)), int(math.Floor(xf))
	for x := start; ; x += dir {
		// exact depth of this column, inverted from scrX
		z := math.Pow((af-0.5)*kx/(float64(x)+0.5-RW/2), 1/zMul) - 1
		u := clamp01(z - z0)
		top, bot := scrY(z, 1), scrY(z, 0)
		sh := bot - top
		for y := max(c.y0, int(math.Floor(top))); y < min(c.y1, int(math.Ceil(bot))); y++ {
			v := clamp01((float64(y) + 0.5 - top) / sh)
			col, h := m.at(u, v)
			if p := 0.017; bevel && u >= 1-p {
				h = math.Min(h, (1-u)/p)
			}
			off := int(math.Round(h * extrude * bump * sh))
			for o := 0; o <= off; o++ {
				ox := x + dir*o
				i := c.idx(ox, y)
				if i < 0 || c.set[i] {
					continue
				}
				pc := col
				if o < off-1 { // side of the extruded texel
					if _, above := m.at(u, v-1/float64(m.h)); above == h {
						pc, _ = m.at(u+0.25/float64(m.w)*float64(off+1-o), v)
					}
					r := float64(o)/maxOff + rnd.Float64()*0.3
					if rnd.Intn(100) < 10 {
						r += 0.2
					}
					pc = mix(pc, m.darkest, 0.2+0.3*clamp01(r))
				}
				bx, by := (af-0.5)*1.5, (z0+u+checker(ox, y)*0.15)*0.7
				pc = shade(pc, darkness(math.Hypot(bx, by)+(1-v)*1.8, mul), 0.3)
				c.put(i, pc)
			}
		}
		if x == end || (dir > 0 && x >= c.x1) || (dir < 0 && x < c.x0) {
			break
		}
	}
	return c.sprite()
}

// frontWall bakes the face of the wall cell at column a, depth plane rz (TextureBuilder.makeFrontWall):
// heights fall in 5 layers, each pushed out from the view centre by its layer times k, copies below a
// texel's own layer darkened 60% toward the darkest colour; a z-map keeps the higher layer on top.
func frontWall(m *mat, a, rz int, mul float64) *sprite {
	m.load()
	af, z := float64(a), float64(rz)
	const layers = 5
	k := []float64{2.5, 2.5, 1.4, 0.8, 0.4, 0.2, 0}[min(rz, 6)] * bump
	pad := k * layers
	x0, x1, y0, y1 := scrX(af, z), scrX(af+1, z), scrY(z, 1), scrY(z, 0)
	c := newCanvas(x0-pad, y0-pad, x1+pad, y1+pad)
	if c == nil {
		return nil
	}
	zmap := make([]int8, len(c.set))
	for l := layers - 1; l >= 0; l-- {
		for y := max(int(math.Floor(y0)), c.y0-int(pad)-1); y < min(int(math.Ceil(y1)), c.y1+int(pad)+1); y++ {
			for x := max(int(math.Floor(x0)), c.x0-int(pad)-1); x < min(int(math.Ceil(x1)), c.x1+int(pad)+1); x++ {
				u, v := clamp01((float64(x)+0.5-x0)/(x1-x0)), clamp01((float64(y)+0.5-y0)/(y1-y0))
				col, h := m.at(u, v)
				if int(h*(layers-1)) != l {
					continue
				}
				for s := l; s >= 0; s-- {
					tx := x + int(math.Floor((af+u-0.5)*float64(s)*k))
					ty := y + int(math.Floor((v-(1-farHei))*float64(s)*k))
					i := c.idx(tx, ty)
					if i < 0 || zmap[i] >= int8(s+1) {
						continue
					}
					pc := col
					if s != l {
						pc = mix(col, m.darkest, 0.6)
					}
					bx, by := (af+u-0.5)*1.5, z*0.7
					pc = shade(pc, darkness(math.Hypot(bx, by)+(1-v+checker(tx, ty)*0.15)*1.8, mul), 0)
					c.put(i, pc)
					zmap[i] = int8(s + 1)
				}
			}
		}
	}
	return c.sprite()
}

// flat bakes the floor (h 0) or ceiling (h 1) of the cell at column a, depth rz. The texture stays
// aligned to the world: f and r are the facing's forward and right vectors.
func flat(m *mat, a, rz int, h float64, f, r [2]int, mul float64) *sprite {
	m.load()
	af, z0 := float64(a), float64(rz)
	ya, yb := scrY(z0, h), scrY(z0+1, h)
	c := newCanvas(math.Min(scrX(af, z0), scrX(af, z0+1)), math.Min(ya, yb), math.Max(scrX(af+1, z0), scrX(af+1, z0+1)), math.Max(ya, yb))
	if c == nil {
		return nil
	}
	for y := c.y0; y < c.y1; y++ {
		z := math.Pow((closeHei-h)*ky/(float64(y)+0.5-RH*farHei), 1/zMul) - 1
		if math.IsNaN(z) || z < z0 || z > z0+1 {
			continue
		}
		v := z - z0
		xl, xr := scrX(af, z), scrX(af+1, z)
		for x := max(c.x0, int(math.Floor(xl))); x < min(c.x1, int(math.Ceil(xr))); x++ {
			u := clamp01((float64(x) + 0.5 - xl) / (xr - xl))
			// local (u right, v forward) to world: the floor tiles the same whichever way we face
			wu := u*float64(r[0]) + v*float64(f[0])
			wv := u*float64(r[1]) + v*float64(f[1])
			col, _ := m.at(wu, wv)
			bx, by := (af+u-0.5)*1.5, z*0.7
			c.put(c.idx(x, y), shade(col, darkness(math.Hypot(bx, by)+h*1.8, mul), 0.4))
		}
	}
	return c.sprite()
}

// ---------- bake cache ----------

type bakeKey struct {
	kind  byte // 's' side, 'S' side with bevel, 'f' front, 'F' floor, 'C' ceiling
	m     *mat
	a, z  int
	dir   int
	light float64
}

var (
	baked    = map[bakeKey]*sprite{}
	flatMats = map[*ebiten.Image]*mat{}
)

// clearBakes drops every baked piece; genTextures calls it.
func clearBakes() {
	for _, s := range baked {
		if s != nil {
			s.img.Deallocate()
		}
	}
	baked, flatMats = map[bakeKey]*sprite{}, map[*ebiten.Image]*mat{}
	prebake.mul = -1 // re-queue every view
}

func bake(k bakeKey, make func() *sprite) *sprite {
	s, ok := baked[k]
	if !ok {
		s = make()
		baked[k] = s
	}
	return s
}

func flatMat(img *ebiten.Image) *mat {
	m := flatMats[img]
	if m == nil {
		m = &mat{img: img}
		flatMats[img] = m
	}
	return m
}

// ---------- scene ----------

// darkMul is how strongly distance darkens: 1 at torch 1, less with a brighter torch or a Light spell,
// 0 in daylight. Quantized so baked pieces are shared.
func (g *Game) darkMul() float64 {
	if openSky {
		return 0
	}
	return math.Round(4/(g.torch+math.Min(1.5, lightLeft)*0.5)) / 4
}

type drawItem struct {
	depth float64
	draw  func(dst *ebiten.Image)
}

// drawWorld renders the view from the party's cell into dst (RW x RH).
func (g *Game) drawWorld(dst *ebiten.Image) {
	dst.Fill(fogCol)
	if openSky {
		g.drawSky(dst)
	}
	mul := g.darkMul()
	var items []drawItem
	pieces(g.x, g.y, g.dir, mul, func(depth float64, s *sprite) {
		if s == nil {
			return
		}
		items = append(items, drawItem{depth, func(dst *ebiten.Image) {
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(float64(s.x), float64(s.y))
			dst.DrawImage(s.img, op)
		}})
	})
	items = append(items, g.billboards(mul)...)
	sort.SliceStable(items, func(i, j int) bool { return items[i].depth > items[j].depth })
	for _, it := range items {
		it.draw(dst)
	}
}

// pieces bakes (or takes from the cache) every floor, ceiling and wall piece seen from cell (x, y)
// facing dir, handing each to put with its draw depth, far pieces first within a row.
func pieces(x, y, dir int, mul float64, put func(depth float64, s *sprite)) {
	f, r := dirs[dir], dirs[(dir+1)%4]
	at := func(a, z int) (int, int) { return x + a*r[0] + z*f[0], y + a*r[1] + z*f[1] }
	isWall := func(a, z int) bool { return wall(at(a, z)) }
	matAt := func(a, z int) *mat { return material(at(a, z)) }
	floorM, ceilM := flatMat(texFloor), flatMat(texCeil)
	depth := maxDZ
	if openSky {
		depth = skyDZ
	}
	for rz := 0; rz <= depth; rz++ {
		// columns until the row's far edge leaves the view: the view widens as (z+1)^zMul
		width := int(math.Ceil(RW / 2 / kx * math.Pow(float64(rz)+2, zMul)))
		for d := width; d >= 0; d-- { // outside in: inner pieces of a row paint over outer ones
			for _, a := range []int{-d, d} {
				if (d == 0 && a != 0) || isWall(a, rz) {
					continue
				}
				put(float64(rz)+2, bake(bakeKey{'F', floorM, a, rz, dir, mul}, func() *sprite { return flat(floorM, a, rz, 0, f, r, mul) }))
				if !openSky {
					put(float64(rz)+2, bake(bakeKey{'C', ceilM, a, rz, dir, mul}, func() *sprite { return flat(ceilM, a, rz, 1, f, r, mul) }))
				}
				// side walls only where they face us: left walls left of the eye, right walls right of it
				for _, s := range [][2]int{{a - 1, a}, {a + 1, a + 1}} {
					wc, edge := s[0], s[1]
					if !isWall(wc, rz) || (wc < a && a > 0) || (wc > a && a < 0) {
						continue
					}
					m, bevel := matAt(wc, rz), !isWall(wc, rz+1)
					kind := byte('s')
					if bevel {
						kind = 'S'
					}
					put(float64(rz)+0.9, bake(bakeKey{kind, m, edge, rz, 0, mul}, func() *sprite { return sideWall(m, edge, rz, bevel, mul) }))
				}
				if isWall(a, rz+1) {
					m := matAt(a, rz+1)
					put(float64(rz+1), bake(bakeKey{'f', m, a, rz + 1, 0, mul}, func() *sprite { return frontWall(m, a, rz+1, mul) }))
				}
			}
		}
	}
}

// prebake is the queue of views still to bake: every open cell in all four facings, filled when the
// map, a cell or the light changes, so walking never waits on a bake.
var prebake struct {
	todo []int // x | y<<12 | dir<<24
	mul  float64
	full bool // drain in one go (map load) instead of within a frame budget
}

func queueBakes(full bool) {
	prebake.todo, prebake.full = prebake.todo[:0], prebake.full || full
	for y, row := range level {
		for x := range row {
			if !wall(x, y) {
				for d := 0; d < 4; d++ {
					prebake.todo = append(prebake.todo, x|y<<12|d<<24)
				}
			}
		}
	}
}

// drainBakes bakes queued views: all of them after a map load, else for about 6 ms a frame.
func (g *Game) drainBakes() {
	if mul := g.darkMul(); mul != prebake.mul {
		prebake.mul = mul
		queueBakes(false)
	}
	start := time.Now()
	for len(prebake.todo) > 0 && (prebake.full || time.Since(start) < 6*time.Millisecond) {
		v := prebake.todo[len(prebake.todo)-1]
		prebake.todo = prebake.todo[:len(prebake.todo)-1]
		pieces(v&0xfff, v>>12&0xfff, v>>24, prebake.mul, func(float64, *sprite) {})
	}
	prebake.full = false
}

// billboards places entities, objects, monsters and the sign. Only cells a ray reaches are drawn:
// a sprite would otherwise paint over the side wall hiding it.
func (g *Game) billboards(mul float64) []drawItem {
	f, r := dirs[g.dir], dirs[(g.dir+1)%4]
	ff, rf := [2]float64{float64(f[0]), float64(f[1])}, [2]float64{float64(r[0]), float64(r[1])}
	vis := visibleCells(g.gridCam())
	var items []drawItem
	// add draws img standing at world (wx, wz) from height h0, wid x hgt in wall heights.
	add := func(img *ebiten.Image, wx, wz, h0, wid, hgt float64, lit bool) {
		if img == nil {
			return
		}
		if _, ok := vis[[2]int{int(math.Floor(wx)), int(math.Floor(wz))}]; !ok {
			return
		}
		ox, oz := wx-float64(g.x)-0.5, wz-float64(g.y)-0.5
		a, z := ox*rf[0]+oz*rf[1]+0.5, ox*ff[0]+oz*ff[1]+0.5
		if z < 0.2 || z > skyDZ+1 {
			return
		}
		s := ky / math.Pow(z+1, zMul) // px per wall height
		w, h := wid*s, hgt*s
		b := img.Bounds()
		op := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
		op.GeoM.Scale(w/float64(b.Dx()), h/float64(b.Dy()))
		op.GeoM.Translate(scrX(a, z)-w/2, scrY(z, h0)-h)
		if !lit {
			k := 1 - darkness(math.Hypot((a-0.5)*1.5, z*0.7)+(h0+hgt/2)*1.8, mul)
			op.ColorScale.Scale(float32(k), float32(k), float32(k), 1)
		}
		items = append(items, drawItem{z, func(dst *ebiten.Image) { dst.DrawImage(img, op) }})
	}
	for _, e := range ents {
		lit := e.kind != Grass
		add(e.tex, e.x, e.z, 0.5-(e.y+e.size/2), e.size, e.size, lit)
	}
	for _, o := range Objects {
		if o.Img == nil {
			continue
		}
		b := o.Img.Bounds()
		k := o.Size / math.Max(float64(b.Dx()), float64(b.Dy()))
		add(o.Img, float64(o.X)+0.5+ff[0]*eyeBack, float64(o.Y)+0.5+ff[1]*eyeBack, 0, k*float64(b.Dx()), k*float64(b.Dy()), false)
	}
	for _, ac := range actors {
		ax, az := ac.pos()
		if ac.x == g.x && ac.y == g.y { // in melee: at the front edge of our cell
			ax, az = float64(g.x)+0.5+ff[0]*0.5, float64(g.y)+0.5+ff[1]*0.5
		} else { // at the front of its cell, facing us
			ax, az = ax-ff[0]*eyeBack, az-ff[1]*eyeBack
		}
		img := ac.img()
		b := img.Bounds()
		size := ac.sheet.Size
		if size == 0 {
			size = 0.6
		}
		k := math.Min(size/float64(b.Dy()), math.Max(1, size)/float64(b.Dx()))
		if ac.sheet.density {
			k = 1.0 / PxPerTile
		}
		add(img, ax, az, 0, k*float64(b.Dx()), k*float64(b.Dy()), false)
	}
	if sign.tex != nil {
		d := dirs[sign.dir]
		b := sign.tex.Bounds()
		wid, hgt := float64(b.Dx())/400, float64(b.Dy())/400 // 400 texture px per wall height
		fwd, h0 := eyeBack, closeHei-hgt/2
		if sign.top {
			fwd, h0 = 0.48, 1-0.04-hgt
		}
		add(sign.tex, float64(sign.x)+0.5+float64(d[0])*fwd, float64(sign.y)+0.5+float64(d[1])*fwd, h0, wid, hgt, true)
	}
	return items
}

// gridCam is the camera at rest in the party's cell, for the ray-cast visibility.
func (g *Game) gridCam() cam {
	ang := float64(g.dir) * math.Pi / 2
	c := cam{ang: ang, fwx: math.Sin(ang), fwz: -math.Cos(ang), rtx: math.Cos(ang), rtz: math.Sin(ang)}
	c.x, c.z = float64(g.x)+0.5-c.fwx*eyeBack, float64(g.y)+0.5-c.fwz*eyeBack
	return c
}

// drawSky shows the ceiling texture as a panorama above the horizon: the picture and its mirror image
// side by side make one full turn, a quarter per facing.
func (g *Game) drawSky(dst *ebiten.Image) {
	b := texCeil.Bounds()
	s := RH * farHei / float64(b.Dy())
	tw := float64(b.Dx()) * s
	x := -float64(g.dir) / 4 * 2 * tw
	for i := 0; x < RW; i, x = i+1, x+tw {
		op := &ebiten.DrawImageOptions{}
		if i%2 == 1 {
			op.GeoM.Scale(-s, s)
			op.GeoM.Translate(tw, 0)
		} else {
			op.GeoM.Scale(s, s)
		}
		op.GeoM.Translate(x, 0)
		dst.DrawImage(texCeil, op)
	}
}

// ---------- transitions ----------

// Transition kinds, as SceneTransition's actions.
const (
	trTurnLeft = iota
	trTurnRight
	trForward
	trBack
	trStrafeRight
	trStrafeLeft
)

// bez is a cubic ease from 0 to 1 with inner control values p1, p2.
func bez(t, p1, p2 float64) float64 {
	t = clamp01(t)
	n := 1 - t
	return 3*t*n*n*p1 + 3*t*t*n*p2 + t*t*t
}

// warp draws img like the reference's CustomTransformation filter: each source column's height is
// scaled about the middle row by a factor running from lh (left edge) to rh (right edge), then the
// image is scaled by (sx, sy) about pivot (px, py) (shares of the view) and shifted by (ox, oy).
func warp(dst, img *ebiten.Image, sx, sy, px, py, ox, oy, lh, rh, alpha float64) {
	const cols = 16
	a := float32(alpha)
	var vs []ebiten.Vertex
	var is []uint16
	for i := 0; i <= cols; i++ {
		u := float64(i) / cols
		k := lh + u*(rh-lh)
		for _, v := range []float64{0, 1} {
			x := (px + (u-px)*sx) * RW
			y := (py + (0.5+(v-0.5)*k-py)*sy) * RH
			vs = append(vs, ebiten.Vertex{DstX: float32(x + ox), DstY: float32(y + oy), SrcX: float32(u * RW), SrcY: float32(v * RH), ColorR: a, ColorG: a, ColorB: a, ColorA: a})
		}
		if i > 0 {
			n := uint16(2 * i)
			is = append(is, n-2, n, n+1, n-2, n+1, n-1)
		}
	}
	dst.DrawTriangles(vs, is, img, &ebiten.DrawTrianglesOptions{})
}

// drawTransition blends the frame before the move (g.prev) into the new one (g.view) at progress t,
// with the reference's SceneTransition parameters.
func (g *Game) drawTransition(dst *ebiten.Image, t float64) {
	dst.Fill(fogCol)
	r := bez(t, 0, 1.2) // elasticOut
	easeIn, easeOut := bez(t, 0, 0.5), bez(t, 0.3, 1)
	bob := math.Sin(t * 2 * math.Pi)
	switch g.trans {
	case trTurnLeft:
		warp(dst, g.view, 1.3-0.3*r, 0.9+0.1*r, 1.3, farHei, 0, 0, 2.5-1.5*r, 0.5+0.5*r, 1)
		warp(dst, g.prev, 1-0.6*r, 1, 0.9, farHei, r*RW/2, 0, 1-0.95*r, 1+3*r, 1-1.8*r)
	case trTurnRight:
		warp(dst, g.view, 1.3-0.3*r, 0.9+0.1*r, -0.3, farHei, 0, 0, 0.5+0.5*r, 2.5-1.5*r, 1)
		warp(dst, g.prev, 1-0.6*r, 1, 0.1, farHei, -r*RW/2, 0, 1+3*r, 1-0.95*r, 1-1.8*r)
	case trForward:
		k := 0.75 + 0.25*r
		warp(dst, g.view, k, k, 0.5, farHei, 0, 0, 1, 1, 1)
		k = 1 + 0.4*easeIn
		warp(dst, g.prev, k, k, 0.5, farHei, 0, 0, 1, 1, 1-clamp01((t-0.3)/0.7))
	case trBack:
		k := 1.3 - 0.3*r
		warp(dst, g.view, k, k, 0.5, farHei, 0, 4*bob, 1, 1, 1)
		k = 1 - 0.4*r
		warp(dst, g.prev, k, k, 0.5, farHei, 0, 4*bob, 1, 1, 1-r)
	case trStrafeRight, trStrafeLeft:
		s := 1.0
		if g.trans == trStrafeLeft {
			s = -1
		}
		prevDist, curDist := 0.8+g.closeWall*0.8, 0.6+g.closeWall*1.2
		warp(dst, g.view, 1, 1, 0.5, farHei, s*(1-easeOut)*0.2*RW*curDist, 3*bob, 1, 1, 1)
		warp(dst, g.prev, 1+0.1*easeIn, 1, 0.5, farHei, -s*easeIn*0.1*RW*prevDist, 3*bob, 1, 1, math.Max(0, 1-easeIn*(2-g.closeWall)))
	}
}

// transitionFor names the move from the party's current cell and facing to (nx, ny, ndir).
func (g *Game) transitionFor(nx, ny, ndir int) int {
	if ndir != g.dir {
		if ndir == (g.dir+3)%4 {
			return trTurnLeft
		}
		return trTurnRight
	}
	f, r := dirs[g.dir], dirs[(g.dir+1)%4]
	dx, dy := nx-g.x, ny-g.y
	switch {
	case dx*f[0]+dy*f[1] > 0:
		return trForward
	case dx*f[0]+dy*f[1] < 0:
		return trBack
	}
	// strafe: how close the wall ahead is, in the new column or the one we left (reference closeWallFactor)
	d := 1
	for ; d <= 3 && !wall(nx+d*f[0], ny+d*f[1]) && !wall(g.x+d*f[0], g.y+d*f[1]); d++ {
	}
	g.closeWall = 1 - float64(d-1)/3
	if dx*r[0]+dy*r[1] > 0 {
		return trStrafeRight
	}
	return trStrafeLeft
}
