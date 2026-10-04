// Blobber: a first-person grid crawler ("gridder") in pure 2D, Deepnight style.
// No 3D pipeline: every surface is a flat quad, transformed by hand, painted far-to-near.
// Textures and their fake-3D bump details are generated at startup from a heightmap.
package main

import (
	"fmt"
	"image/color"
	"log"
	"math"
	"math/rand"
	"sort"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

var (
	W, H  = 640, 480 // set by Layout to the real window size in device pixels
	focal = float64(W / 2)
)

const (
	near    = 0.05
	eyeBack = 0.3 // camera offset behind the tile center, in tiles (<0.5 keeps it inside the tile)
	radius  = 8   // max ray depth in cells
	tex     = 128 // texture size
)

var level = []string{
	"#############################",
	"#..............##############",
	"#.##.###.#####.#............#",
	"#.#....#.#...#.#............#",
	"#.#.##.#.#...#.#............#",
	"#...#....#...#.#............#",
	"###.#.####.#.#.#............#",
	"#...#....#.#................#",
	"#.#...##.#.#.#.#............#",
	"#.####.#.....#.#............#",
	"#......#.###.#.#............#",
	"#.####.#.#...#.#............#",
	"#......#...#...##############",
	"#############################",
}

func wall(x, y int) bool {
	return y < 0 || y >= len(level) || x < 0 || x >= len(level[y]) || level[y][x] == '#'
}

// ---------- startup texture generation (the "fake 3D" trick) ----------

var (
	rng  = rand.New(rand.NewSource(7))
	bump = 1.0 // heightmap strength baked into the textures
)

// genStone bakes a heightmap of blocks into a lit texture. lx,ly is the baked light direction,
// so a side-facing wall gets its grooves shaded as if seen from the corridor.
func genStone(bw, bh int, offset bool, base, mortar [3]float64, lx, ly float64) *ebiten.Image {
	h := make([]float64, tex*tex)
	noise := make([]float64, tex*tex)
	for y := 0; y < tex; y++ {
		for x := 0; x < tex; x++ {
			ox := 0
			if offset && (y/bh)%2 == 1 {
				ox = bw / 2
			}
			bx, by := (x+ox)%bw, y%bh
			d := math.Min(math.Min(float64(bx), float64(bw-1-bx)), math.Min(float64(by), float64(bh-1-by)))
			h[y*tex+x] = math.Max(0, math.Min(1, (d-3)/4)) + rng.Float64()*0.08
			noise[y*tex+x] = 0.85 + 0.3*rng.Float64()
		}
	}
	img := ebiten.NewImage(tex, tex)
	at := func(x, y int) float64 { return h[((y+tex)%tex)*tex+(x+tex)%tex] }
	for y := 0; y < tex; y++ {
		for x := 0; x < tex; x++ {
			gx, gy := at(x+1, y)-at(x-1, y), at(x, y+1)-at(x, y-1)
			shade := math.Max(0.3, math.Min(1.3, 0.85+0.7*bump*(gx*lx+gy*ly)))
			c := base
			if at(x, y) < 0.3 {
				c = mortar
			}
			n := noise[y*tex+x]
			img.Set(x, y, color.RGBA{u8(c[0] * n * shade), u8(c[1] * n * shade), u8(c[2] * n * shade), 255})
		}
	}
	return img
}

func u8(v float64) uint8 { return uint8(math.Max(0, math.Min(255, v))) }

func genGrass() *ebiten.Image {
	img := ebiten.NewImage(tex, tex)
	for i := 0; i < 40; i++ {
		x0, lean, hgt := 4+rng.Float64()*56, rng.Float64()*16-8, 20+rng.Float64()*36
		g := color.RGBA{uint8(40 + rng.Intn(60)), uint8(120 + rng.Intn(100)), uint8(30 + rng.Intn(40)), 255}
		for t := 0.0; t < hgt; t++ {
			img.Set(int(x0+lean*t*t/(hgt*hgt)), tex-1-int(t), g)
		}
	}
	return img
}

func genSolid(c color.RGBA) *ebiten.Image { i := ebiten.NewImage(8, 8); i.Fill(c); return i }

var (
	stone                                          = [3]float64{150, 130, 110}
	mortarC                                        = [3]float64{80, 72, 70}
	texFront, texRight, texLeft, texFloor, texCeil *ebiten.Image
	texGrass, texLantern, texFirefly               *ebiten.Image
	atlas                                          []struct {
		name string
		img  *ebiten.Image
	}
)

func genTextures() {
	rng.Seed(7)
	atlas = nil
	texFront = genStone(64, 32, true, stone, mortarC, 0.3, -1)
	texRight = genStone(64, 32, true, stone, mortarC, 1, -0.4) // wall on our left, face points right
	texLeft = genStone(64, 32, true, stone, mortarC, -1, -0.4) // wall on our right, face points left
	texFloor = genStone(64, 64, false, [3]float64{115, 110, 105}, [3]float64{60, 58, 55}, 0.4, -1)
	texCeil = genStone(64, 64, false, [3]float64{70, 66, 72}, [3]float64{40, 38, 42}, 0.4, 1)
	texGrass = genGrass()
	texLantern = genSolid(color.RGBA{255, 220, 150, 255})
	texFirefly = genSolid(color.RGBA{200, 255, 180, 255})
	for _, e := range []struct {
		name string
		img  *ebiten.Image
	}{{"front", texFront}, {"right", texRight}, {"left", texLeft}, {"floor", texFloor}, {"ceil", texCeil}, {"grass", texGrass}} {
		atlas = append(atlas, e)
	}
}

// ---------- entities ----------

type ent struct {
	kind        int // 0 grass, 1 lantern (moving light), 2 firefly
	x, y, z     float64
	hx, hz      float64 // home
	phase       float64
	size, light float64
	lr, lg, lb  float64
	tex         *ebiten.Image
}

var ents []*ent

func spawn() {
	add := func(kind, cx, cz int) {
		e := &ent{kind: kind, hx: float64(cx) + 0.5, hz: float64(cz) + 0.5, phase: rng.Float64() * 6.28}
		e.x, e.z = e.hx, e.hz
		switch kind {
		case 0:
			e.y, e.size, e.tex = 0.5-0.3, 0.6, texGrass
		case 1:
			e.y, e.size, e.tex, e.light, e.lr, e.lg, e.lb = -0.1, 0.12, texLantern, 0.9, 0.5, 0.7, 1
		case 2:
			e.y, e.size, e.tex, e.light, e.lr, e.lg, e.lb = 0.1, 0.04, texFirefly, 0.25, 0.6, 1, 0.5
		}
		ents = append(ents, e)
	}
	for _, c := range [][2]int{{10, 3}, {11, 4}, {12, 3}, {11, 3}, {8, 7}, {1, 10}, {3, 10}, {1, 7}} {
		add(0, c[0], c[1])
	}
	add(1, 1, 1)
	add(1, 8, 12)
	add(1, 21, 6)
	for _, c := range [][2]int{{10, 4}, {12, 4}, {11, 5}, {2, 7}, {4, 10}, {18, 4}, {25, 9}, {24, 3}} {
		add(2, c[0], c[1])
	}
}

func updateEnts(t float64) {
	for _, e := range ents {
		switch e.kind {
		case 1: // lanterns patrol their corridor
			if e.hx < 5 {
				e.x = 8 + 6.5*math.Sin(t*0.5+e.phase)
			} else if e.hx > 16 { // big room: slow orbit
				e.x, e.z = e.hx+3.5*math.Sin(t*0.3+e.phase), e.hz+3.5*math.Cos(t*0.3+e.phase)
			} else {
				e.x = 9.5 + math.Sin(t*0.8+e.phase)
			}
		case 2:
			e.x = e.hx + 0.35*math.Sin(t*1.3+e.phase)*math.Cos(t*0.7+e.phase)
			e.z = e.hz + 0.35*math.Cos(t*1.1+e.phase)
			e.y = 0.1 + 0.15*math.Sin(t*2.1+e.phase)
			e.light = 0.25 + 0.2*math.Sin(t*5+e.phase*3)
		}
	}
}

// ---------- game state ----------

type Game struct {
	x, y, dir int
	// smooth movement tween
	fx, fz, fa, tx, tz, ta float64 // from/to pos & angle
	t, dur, sway           float64 // tween progress, duration, roll direction
	moving                 bool
	queued                 ebiten.Key
	time                   float64
	// settings
	dither, smooth, menu, atlasView bool
	torch                           float64
	sel                             int
	scene                           *ebiten.Image
	shader, texShader               *ebiten.Shader
}

var dirs = [4][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}

func (g *Game) startMove(nx, ny, ndir int, sway float64) {
	g.fx, g.fz, g.fa = g.tx, g.tz, g.ta
	g.dur = 0.28
	if nx == g.x && ny == g.y {
		g.dur = 0.22 // turn
	}
	g.x, g.y, g.dir = nx, ny, ndir
	g.tx, g.tz = float64(nx)+0.5, float64(ny)+0.5
	g.ta = float64(ndir) * math.Pi / 2
	for g.ta-g.fa > math.Pi {
		g.ta -= 2 * math.Pi
	}
	for g.ta-g.fa < -math.Pi {
		g.ta += 2 * math.Pi
	}
	g.t, g.moving, g.sway = 0, true, sway
	if !g.smooth {
		g.t = 1
	}
}

func (g *Game) act(k ebiten.Key) {
	if g.moving {
		g.queued = k // ponytail: one-slot input queue, ring buffer if players mash keys
		return
	}
	f, r := dirs[g.dir], dirs[(g.dir+1)%4]
	step := func(dx, dy int, sway float64) {
		if !wall(g.x+dx, g.y+dy) {
			g.startMove(g.x+dx, g.y+dy, g.dir, sway)
		}
	}
	switch k {
	case ebiten.KeyW, ebiten.KeyUp:
		step(f[0], f[1], 0)
	case ebiten.KeyS, ebiten.KeyDown:
		step(-f[0], -f[1], 0)
	case ebiten.KeyA:
		step(-r[0], -r[1], -1)
	case ebiten.KeyD:
		step(r[0], r[1], 1)
	case ebiten.KeyQ, ebiten.KeyLeft:
		g.startMove(g.x, g.y, (g.dir+3)%4, 0)
	case ebiten.KeyE, ebiten.KeyRight:
		g.startMove(g.x, g.y, (g.dir+1)%4, 0)
	}
}

func (g *Game) Update() error {
	g.time += 1.0 / 60
	updateEnts(g.time)
	if inpututil.IsKeyJustPressed(ebiten.KeyF1) {
		g.atlasView = !g.atlasView
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) || inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
		g.menu = !g.menu
	}
	if g.menu {
		g.updateMenu()
	} else {
		for _, k := range []ebiten.Key{ebiten.KeyW, ebiten.KeyS, ebiten.KeyA, ebiten.KeyD, ebiten.KeyQ, ebiten.KeyE, ebiten.KeyUp, ebiten.KeyDown, ebiten.KeyLeft, ebiten.KeyRight} {
			if inpututil.IsKeyJustPressed(k) {
				g.act(k)
			}
		}
		// mouse / touch: middle third walks, sides turn
		px := -1
		if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
			px, _ = ebiten.CursorPosition()
		}
		for _, id := range inpututil.AppendJustPressedTouchIDs(nil) {
			px, _ = ebiten.TouchPosition(id)
		}
		switch {
		case px < 0:
		case px < W/3:
			g.act(ebiten.KeyQ)
		case px > 2*W/3:
			g.act(ebiten.KeyE)
		default:
			g.act(ebiten.KeyW)
		}
	}
	if g.moving {
		g.t += 1 / (60 * g.dur)
		if g.t >= 1 {
			g.t, g.moving = 1, false
			if g.queued != 0 {
				k := g.queued
				g.queued = 0
				g.act(k)
			}
		}
	}
	return nil
}

func (g *Game) updateMenu() {
	n := 5
	if inpututil.IsKeyJustPressed(ebiten.KeyUp) {
		g.sel = (g.sel + n - 1) % n
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyDown) {
		g.sel = (g.sel + 1) % n
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyLeft) || inpututil.IsKeyJustPressed(ebiten.KeyRight) || inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		switch g.sel {
		case 0:
			g.dither = !g.dither
		case 1:
			g.smooth = !g.smooth
		case 2:
			g.torch = math.Mod(g.torch, 1.5) + 0.5 // 0.5 → 1 → 1.5 → 0.5
		case 3:
			d := 0.25
			if inpututil.IsKeyJustPressed(ebiten.KeyLeft) {
				d = -0.25
			}
			bump = math.Max(0, math.Min(2, bump+d))
			genTextures() // baked at startup, so re-bake
		case 4:
			ebiten.SetFullscreen(!ebiten.IsFullscreen())
		}
	}
}

// ---------- rendering ----------

type vert struct{ x, y, z, u, v, r, g, b float64 } // world pos, uv, light

type face struct {
	v    [4]vert
	tex  *ebiten.Image
	dist float64
	sub  int
}

func lerp(a, b vert, t float64) vert {
	return vert{a.x + (b.x-a.x)*t, a.y + (b.y-a.y)*t, a.z + (b.z-a.z)*t, a.u + (b.u-a.u)*t, a.v + (b.v-a.v)*t,
		a.r + (b.r-a.r)*t, a.g + (b.g-a.g)*t, a.b + (b.b-a.b)*t}
}

// camera for this frame
type cam struct{ x, z, ang, roll, fwx, fwz, rtx, rtz float64 }

func (g *Game) camera() cam {
	e := g.t * g.t * (3 - 2*g.t) // smoothstep
	c := cam{x: g.fx + (g.tx-g.fx)*e, z: g.fz + (g.tz-g.fz)*e, ang: g.fa + (g.ta-g.fa)*e}
	if g.moving {
		s := math.Sin(math.Pi * g.t)
		c.ang += 0.025 * s * g.sway
		c.roll = 0.012 * s * g.sway
	}
	c.fwx, c.fwz = math.Sin(c.ang), -math.Cos(c.ang)
	c.rtx, c.rtz = math.Cos(c.ang), math.Sin(c.ang)
	c.x, c.z = c.x-c.fwx*eyeBack, c.z-c.fwz*eyeBack // eye sits behind tile center: side exits visible, turns orbit the center
	return c
}

// visibleCells casts one DDA ray per 2 screen columns across the frustum (plus a margin for sway/roll);
// every open cell a ray crosses before hitting a wall is visible. Frustum + occlusion culling in one loop.
// ponytail: cached per camera; rays can skip a far cell seen through a 1-cell gap, cast more if that shows.
var visCache struct {
	c     cam
	cells map[[2]int]bool
}

func visibleCells(c cam) map[[2]int]bool {
	if visCache.cells != nil && visCache.c == c {
		return visCache.cells
	}
	cells := map[[2]int]bool{{int(math.Floor(c.x)), int(math.Floor(c.z))}: true}
	for sx := -W/2 - 40; sx <= W/2+40; sx += 2 {
		k := float64(sx) / focal
		dx, dz := c.fwx+c.rtx*k, c.fwz+c.rtz*k
		// DDA from (c.x,c.z) along (dx,dz)
		cx, cz := int(math.Floor(c.x)), int(math.Floor(c.z))
		stepX, stepZ := 1, 1
		tdx, tdz := math.Abs(1/dx), math.Abs(1/dz)
		tx, tz := (float64(cx+1)-c.x)*tdx, (float64(cz+1)-c.z)*tdz
		if dx < 0 {
			stepX, tx = -1, (c.x-float64(cx))*tdx
		}
		if dz < 0 {
			stepZ, tz = -1, (c.z-float64(cz))*tdz
		}
		for i := 0; i < 2*radius; i++ {
			if tx < tz {
				cx, tx = cx+stepX, tx+tdx
			} else {
				cz, tz = cz+stepZ, tz+tdz
			}
			if wall(cx, cz) {
				break
			}
			cells[[2]int{cx, cz}] = true
		}
	}
	visCache.c, visCache.cells = c, cells
	return cells
}

func (g *Game) light(c cam, v *vert) {
	add := func(lx, ly, lz, i, r, gg, b float64) {
		d := (v.x-lx)*(v.x-lx) + (v.y-ly)*(v.y-ly) + (v.z-lz)*(v.z-lz)
		a := i / (1 + 0.5*d*d/i)
		v.r, v.g, v.b = v.r+a*r, v.g+a*gg, v.b+a*b
	}
	v.r, v.g, v.b = 0.03, 0.03, 0.04
	add(c.x, 0, c.z, g.torch, 1, 0.85, 0.65)
	for _, e := range ents {
		if e.light > 0 {
			add(e.x, e.y, e.z, e.light, e.lr, e.lg, e.lb)
		}
	}
	v.r, v.g, v.b = math.Min(1, v.r), math.Min(1, v.g), math.Min(1, v.b)
}

var white = genSolid(color.RGBA{255, 255, 255, 255})

// drawQuad subdivides, lights, transforms, near-clips and projects one textured quad.
func (g *Game) drawQuad(dst *ebiten.Image, c cam, f *face) {
	n := f.sub
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			s0, s1, t0, t1 := float64(i)/float64(n), float64(i+1)/float64(n), float64(j)/float64(n), float64(j+1)/float64(n)
			bil := func(s, t float64) vert {
				a, b := lerp(f.v[0], f.v[1], s), lerp(f.v[3], f.v[2], s)
				v := lerp(a, b, t)
				if f.tex == texFirefly || f.tex == texLantern {
					v.r, v.g, v.b = 1, 1, 1 // self-lit
				} else {
					g.light(c, &v)
				}
				// to camera space (x right, y down, z forward)
				dx, dz := v.x-c.x, v.z-c.z
				v.x, v.z = dx*c.rtx+dz*c.rtz, dx*c.fwx+dz*c.fwz
				return v
			}
			poly := []vert{bil(s0, t0), bil(s1, t0), bil(s1, t1), bil(s0, t1)}
			// clip against near plane
			var out []vert
			for k := range poly {
				a, b := poly[k], poly[(k+1)%4]
				if a.z >= near {
					out = append(out, a)
				}
				if (a.z >= near) != (b.z >= near) {
					out = append(out, lerp(a, b, (near-a.z)/(b.z-a.z)))
				}
			}
			if len(out) < 3 {
				continue
			}
			cr, sr := math.Cos(c.roll), math.Sin(c.roll)
			vs := make([]ebiten.Vertex, len(out))
			for k, v := range out {
				iz := 1 / v.z
				px, py := v.x*focal*iz, v.y*focal*iz
				// perspective-correct: u/z, v/z, 1/z in Custom0..2 (SrcX/Y get the atlas origin added, so they can't carry it)
				vs[k] = ebiten.Vertex{DstX: float32(float64(W)/2 + px*cr - py*sr), DstY: float32(float64(H)/2 + px*sr + py*cr),
					ColorR: float32(v.r), ColorG: float32(v.g), ColorB: float32(v.b), ColorA: 1,
					Custom0: float32(v.u * iz), Custom1: float32(v.v * iz), Custom2: float32(iz)}
			}
			var idx []uint16
			for k := 1; k+1 < len(out); k++ {
				idx = append(idx, 0, uint16(k), uint16(k+1))
			}
			op := &ebiten.DrawTrianglesShaderOptions{}
			op.Images[0] = f.tex
			dst.DrawTrianglesShader(vs, idx, g.texShader, op)
		}
	}
}

func (g *Game) Draw(screen *ebiten.Image) {
	if g.atlasView {
		g.drawAtlas(screen)
		return
	}
	c := g.camera()
	dst := screen
	if g.dither {
		dst = g.scene
	}
	dst.Fill(color.Black)

	var faces []*face
	q := func(a, b, cc, d vert, t *ebiten.Image, sub int) {
		f := &face{v: [4]vert{a, b, cc, d}, tex: t, sub: sub}
		mx, mz := (a.x+cc.x)/2-c.x, (a.z+cc.z)/2-c.z
		f.dist = mx*c.fwx + mz*c.fwz
		if f.dist > -1 {
			faces = append(faces, f)
		}
	}
	for cell := range visibleCells(c) {
		cx, cy := cell[0], cell[1]
		x0, z0, x1, z1 := float64(cx), float64(cy), float64(cx+1), float64(cy+1)
		q(vert{x0, 0.5, z0, 0, 0, 0, 0, 0}, vert{x1, 0.5, z0, 1, 0, 0, 0, 0}, vert{x1, 0.5, z1, 1, 1, 0, 0, 0}, vert{x0, 0.5, z1, 0, 1, 0, 0, 0}, texFloor, 3)
		q(vert{x0, -0.5, z0, 0, 0, 0, 0, 0}, vert{x1, -0.5, z0, 1, 0, 0, 0, 0}, vert{x1, -0.5, z1, 1, 1, 0, 0, 0}, vert{x0, -0.5, z1, 0, 1, 0, 0, 0}, texCeil, 3)
		// wall faces on the four sides of this floor cell, textured by how they face the camera
		type side struct{ ax, az, bx, bz, nx, nz float64 }
		for i, s := range []side{{x0, z0, x1, z0, 0, 1}, {x1, z0, x1, z1, -1, 0}, {x1, z1, x0, z1, 0, -1}, {x0, z1, x0, z0, 1, 0}} {
			if !wall(cx+dirs[i][0], cy+dirs[i][1]) {
				continue
			}
			nx := s.nx*c.rtx + s.nz*c.rtz // normal in camera space, lateral component
			t := texFront
			if nx > 0.5 {
				t = texRight
			} else if nx < -0.5 {
				t = texLeft
			}
			q(vert{s.ax, -0.5, s.az, 0, 0, 0, 0, 0}, vert{s.bx, -0.5, s.bz, 1, 0, 0, 0, 0}, vert{s.bx, 0.5, s.bz, 1, 1, 0, 0, 0}, vert{s.ax, 0.5, s.az, 0, 1, 0, 0, 0}, t, 3)
		}
	}
	for _, e := range ents { // billboards, only in cells a ray reached
		if _, ok := visibleCells(c)[[2]int{int(math.Floor(e.x)), int(math.Floor(e.z))}]; !ok {
			continue
		}
		hx, hz := c.rtx*e.size/2, c.rtz*e.size/2
		q(vert{e.x - hx, e.y - e.size/2, e.z - hz, 0, 0, 0, 0, 0}, vert{e.x + hx, e.y - e.size/2, e.z + hz, 1, 0, 0, 0, 0},
			vert{e.x + hx, e.y + e.size/2, e.z + hz, 1, 1, 0, 0, 0}, vert{e.x - hx, e.y + e.size/2, e.z - hz, 0, 1, 0, 0, 0}, e.tex, 1)
	}
	// ponytail: painter's sort by face-center depth; z-buffer if overlap artifacts show up.
	// Floor/ceiling first: at eye height they can't occlude walls or billboards, but their centers can sort nearer.
	flat := func(f *face) bool { return f.tex == texFloor || f.tex == texCeil }
	sort.Slice(faces, func(i, j int) bool {
		if fi, fj := flat(faces[i]), flat(faces[j]); fi != fj {
			return fi
		}
		return faces[i].dist > faces[j].dist
	})
	for _, f := range faces {
		g.drawQuad(dst, c, f)
	}
	if g.dither {
		op := &ebiten.DrawRectShaderOptions{}
		op.Images[0] = g.scene
		screen.DrawRectShader(W, H, g.shader, op)
	}
	g.drawMinimap(screen, c)
	ebitenutil.DebugPrint(screen, fmt.Sprintf("%.0f fps  pos %d,%d facing %c   WASD move  Q/E turn  click: sides turn, middle walk  Enter: menu  F1: atlas", ebiten.ActualFPS(), g.x, g.y, "NESW"[g.dir]))
	if g.menu {
		g.drawMenu(screen)
	}
}

func onOff(b bool) string {
	if b {
		return "ON"
	}
	return "OFF"
}

func (g *Game) drawMenu(screen *ebiten.Image) {
	ebitenutil.DrawRect(screen, float64(W/2-130), float64(H/2-60), 260, 152, color.RGBA{0, 0, 0, 200})
	items := []string{"Dithered darkness: " + onOff(g.dither), "Smooth movement:   " + onOff(g.smooth), fmt.Sprintf("Torch intensity:   %.1f", g.torch), fmt.Sprintf("Heightmap bump:    %.2f", bump), "Fullscreen:        " + onOff(ebiten.IsFullscreen())}
	ebitenutil.DebugPrintAt(screen, "SETTINGS  (up/down, left/right, Enter closes)", W/2-120, H/2-50)
	for i, s := range items {
		cur := "  "
		if i == g.sel {
			cur = "> "
		}
		ebitenutil.DebugPrintAt(screen, cur+s, W/2-120, H/2-20+i*16)
	}
}

func (g *Game) drawAtlas(screen *ebiten.Image) {
	screen.Fill(color.RGBA{30, 30, 40, 255})
	ebitenutil.DebugPrint(screen, "Generated texture atlas (F1 to return). Bump details baked from the heightmap per facing.")
	for i, a := range atlas {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(2, 2)
		op.GeoM.Translate(float64(20+i*140%560), float64(40+(i*140/560)*170))
		screen.DrawImage(a.img, op)
		ebitenutil.DebugPrintAt(screen, a.name, 20+i*140%560, 40+(i*140/560)*170+132)
	}
}

// drawMinimap: 15x15 cells around the player top-right, walls as squares, player as an arrow along the camera.
func (g *Game) drawMinimap(screen *ebiten.Image, c cam) {
	const r = 7                                             // half window
	cs := float32(5 * ebiten.Monitor().DeviceScaleFactor()) // cell size in px
	ox, oy := float32(W)-cs*(2*r+3), 2*cs
	vector.DrawFilledRect(screen, ox-cs, oy-cs, cs*(2*r+3), cs*(2*r+3), color.RGBA{0, 0, 0, 160}, false)
	for y := g.y - r; y <= g.y+r; y++ {
		for x := g.x - r; x <= g.x+r; x++ {
			if wall(x, y) {
				vector.DrawFilledRect(screen, ox+cs*float32(x-g.x+r), oy+cs*float32(y-g.y+r), cs, cs, color.RGBA{170, 160, 150, 255}, false)
			}
		}
	}
	px, py := ox+cs*float32(c.x-float64(g.x-r)), oy+cs*float32(c.z-float64(g.y-r))
	fx, fy := float32(c.fwx)*cs, float32(c.fwz)*cs // forward
	rx, ry := float32(c.rtx)*cs*0.6, float32(c.rtz)*cs*0.6
	yellow := color.RGBA{255, 220, 60, 255}
	vector.StrokeLine(screen, px-fx, py-fy, px+fx, py+fy, cs/3, yellow, true)
	vector.StrokeLine(screen, px+fx, py+fy, px-rx, py-ry, cs/3, yellow, true)
	vector.StrokeLine(screen, px+fx, py+fy, px+rx, py+ry, cs/3, yellow, true)
}

func (g *Game) Layout(ow, oh int) (int, int) {
	sc := ebiten.Monitor().DeviceScaleFactor()
	if w, h := int(float64(ow)*sc), int(float64(oh)*sc); w != W || h != H {
		W, H, focal = w, h, float64(w)/2
		g.scene = ebiten.NewImage(W, H)
	}
	return W, H
}

// Ordered 4x4 Bayer dither with 8 brightness levels: retro darkness without smooth gradients.
const ditherSrc = `//kage:unit pixels
package main

func bayer2(x, y float) float { return 2*x + 3*y - 4*x*y }

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	c := imageSrc0At(srcPos)
	x, y := floor(dstPos.x), floor(dstPos.y)
	b := 4*bayer2(mod(x, 2), mod(y, 2)) + bayer2(mod(floor(x/2), 2), mod(floor(y/2), 2))
	t := (b + 0.5) / 16
	levels := 8.0
	return vec4(floor(c.rgb*levels+t)/levels, 1)
}
`

// Perspective-correct texturing: custom = (u/z, v/z, 1/z); divide, then sample.
const texSrc = `//kage:unit pixels
package main

func Fragment(dstPos vec4, srcPos vec2, color vec4, custom vec4) vec4 {
	uv := fract(custom.xy / custom.z)
	c := imageSrc0At(imageSrc0Origin() + uv*imageSrc0Size())
	return vec4(c.rgb*color.rgb, c.a)
}
`

func main() {
	genTextures()
	spawn()
	tsh, err := ebiten.NewShader([]byte(texSrc))
	if err != nil {
		log.Fatal(err)
	}
	sh, err := ebiten.NewShader([]byte(ditherSrc))
	if err != nil {
		log.Fatal(err)
	}
	g := &Game{x: 1, y: 1, dir: 1, dither: true, smooth: true, torch: 1, t: 1, scene: ebiten.NewImage(W, H), shader: sh, texShader: tsh}
	g.tx, g.tz, g.ta = 1.5, 1.5, math.Pi/2
	g.fx, g.fz, g.fa = g.tx, g.tz, g.ta
	ebiten.SetWindowSize(W, H)
	ebiten.SetWindowTitle("Blobber")
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}
