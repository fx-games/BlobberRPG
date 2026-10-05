// Map editor: paints wall materials and edits event scripts on tiles.
//
//	go run ./cmd/editor [mapname]      (default: demo; edits maps/<name>.map and maps/<name>.evt)
//
// Mouse: left paints the palette material, right selects a tile for events, click a palette swatch
// to choose a material, click a keyword to insert its template into the script, click in the script
// to edit it. Keys: Ctrl+S save, N new event on the selected tile, F cycle facing, U toggle use,
// Delete remove event, Tab next event, Shift+Arrows grow the map, Ctrl+Shift+Arrows shrink it.
package main

import (
	"bufio"
	"fmt"
	"image/color"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	"blobber"
	"blobber/events"
	"blobber/party"
	"blobber/sprites"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	cell   = 24
	panelX = 760
	winW   = 1200
	winH   = 760
)

var facings = []string{"n", "e", "s", "w", "any"}

var templates = []string{
	`text sign "..."`, `npc Name face "..." confirm L`, `if mapflag NAME goto L`, `if gold >= 10 goto L`,
	`give gold 10`, `take gold 10`, `set mapflag NAME`, `whowill search "Who will?"`, `setchar 0`, `selrndchar`,
	`damage 5 fire`, `rnddamage poison 10`, `giveitem sword`, `teleport MAP X Y`, `fall MAP X Y 5`,
	`spawn goblin X Y`, `alterwall X Y n #`, `altercell X Y .`, `moveobj NAME X Y`, `exchobj A B`,
	`sound NAME`, `music NAME`, `cutscene NAME`, `town tavern`, `goto L`, `gotorandom A B`,
	`jumprnd 10 1 goto L`, `choose A B`, `confirm "Password?" "word" goto L`, `call X,Y any`, `return`,
	`exit`, `remove`, `nothing`, `alter 0 nop`, `nop`, `L:`,
}

type event struct {
	x, y   int
	facing int // index into facings
	use    bool
	body   []string
}

type editor struct {
	name    string
	grid    [][]byte
	evs     []*event
	brush   byte
	palette []byte
	selX    int
	selY    int
	cur     *event // selected event
	line    int    // cursor line/col in cur.body
	col     int
	focus   bool // keyboard goes to the script
	status  string
	swatch  map[byte]*ebiten.Image
	extra   []string // object/item/monster/deco lines of the .map file, kept verbatim
}

// ---------- files ----------

func (e *editor) load() {
	f, err := os.Open("maps/" + e.name + ".map")
	if err != nil {
		e.grid = [][]byte{[]byte("#####"), []byte("#...#"), []byte("#####")}
	} else {
		rows, extra, _ := blobber.LoadMap(f)
		e.extra = extra
		f.Close()
		for _, r := range rows {
			e.grid = append(e.grid, []byte(r))
		}
	}
	f, err = os.Open("maps/" + e.name + ".evt")
	if err != nil {
		return
	}
	defer f.Close()
	var cur *event
	for sc := bufio.NewScanner(f); sc.Scan(); {
		l := sc.Text()
		t := strings.Fields(strings.ToLower(l))
		if len(t) >= 3 && t[0] == "event" {
			cur = &event{use: len(t) > 3 && t[3] == "use"}
			fmt.Sscanf(t[1], "%d,%d", &cur.x, &cur.y)
			for i, f := range facings {
				if f == t[2] {
					cur.facing = i
				}
			}
			e.evs = append(e.evs, cur)
		} else if cur != nil {
			cur.body = append(cur.body, strings.TrimSpace(l))
		} else if strings.TrimSpace(l) != "" {
			e.status = "dropped line before first event: " + l
		}
	}
	for _, ev := range e.evs { // strip trailing blank lines
		for len(ev.body) > 0 && ev.body[len(ev.body)-1] == "" {
			ev.body = ev.body[:len(ev.body)-1]
		}
	}
}

func (e *editor) evtText() string {
	var b strings.Builder
	sort.SliceStable(e.evs, func(i, j int) bool {
		return e.evs[i].y < e.evs[j].y || e.evs[i].y == e.evs[j].y && e.evs[i].x < e.evs[j].x
	})
	for _, ev := range e.evs {
		fmt.Fprintf(&b, "event %d,%d %s", ev.x, ev.y, facings[ev.facing])
		if ev.use {
			b.WriteString(" use")
		}
		b.WriteString("\n")
		for _, l := range ev.body {
			if strings.HasSuffix(l, ":") || l == "" {
				b.WriteString(l + "\n")
			} else {
				b.WriteString("  " + l + "\n")
			}
		}
		b.WriteString("\n")
	}
	return b.String()
}

func (e *editor) save() {
	txt := e.evtText()
	if err := events.New(nil).Load(e.name, strings.NewReader(txt)); err != nil {
		e.status = "NOT SAVED: " + err.Error()
		return
	}
	var m strings.Builder
	for _, r := range e.grid {
		m.Write(r)
		m.WriteByte('\n')
	}
	for _, l := range e.extra {
		m.WriteString(l + "\n")
	}
	if err := os.WriteFile("maps/"+e.name+".map", []byte(m.String()), 0o644); err != nil {
		e.status = err.Error()
		return
	}
	if err := os.WriteFile("maps/"+e.name+".evt", []byte(txt), 0o644); err != nil {
		e.status = err.Error()
		return
	}
	e.status = "saved maps/" + e.name + ".map and .evt"
}

// ---------- update ----------

func (e *editor) Update() error {
	mx, my := ebiten.CursorPosition()
	ctrl, shift := ebiten.IsKeyPressed(ebiten.KeyControl) || ebiten.IsKeyPressed(ebiten.KeyMeta), ebiten.IsKeyPressed(ebiten.KeyShift)
	if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyS) {
		e.save()
		return nil
	}
	if shift {
		e.resize(ctrl)
	}
	gx, gy := mx/cell, my/cell
	onGrid := mx < panelX && gy < len(e.grid) && gx < len(e.grid[gy])
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		e.focus = false
		e.click(mx, my)
	}
	if onGrid && ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) && mx < panelX {
		e.grid[gy][gx] = e.brush
	}
	if onGrid && inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonRight) {
		e.selX, e.selY, e.cur, e.focus = gx, gy, nil, false
		for _, ev := range e.evs {
			if ev.x == gx && ev.y == gy {
				e.cur = ev
				break
			}
		}
		e.line, e.col = 0, 0
	}
	if e.focus && e.cur != nil {
		e.editScript()
		return nil
	}
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyN):
		e.cur = &event{x: e.selX, y: e.selY, facing: 4, body: []string{`text "..."`}}
		e.evs = append(e.evs, e.cur)
		e.line, e.col = 0, 0
	case e.cur != nil && inpututil.IsKeyJustPressed(ebiten.KeyF):
		e.cur.facing = (e.cur.facing + 1) % len(facings)
	case e.cur != nil && inpututil.IsKeyJustPressed(ebiten.KeyU):
		e.cur.use = !e.cur.use
	case e.cur != nil && inpututil.IsKeyJustPressed(ebiten.KeyDelete):
		for i, ev := range e.evs {
			if ev == e.cur {
				e.evs = append(e.evs[:i], e.evs[i+1:]...)
			}
		}
		e.cur = nil
	case inpututil.IsKeyJustPressed(ebiten.KeyTab):
		here := e.tileEvents()
		for i, ev := range here {
			if ev == e.cur {
				e.cur = here[(i+1)%len(here)]
				e.line, e.col = 0, 0
				break
			}
		}
	}
	return nil
}

func (e *editor) resize(shrink bool) {
	w := len(e.grid[0])
	switch {
	case inpututil.IsKeyJustPressed(ebiten.KeyRight) && !shrink:
		for y := range e.grid {
			e.grid[y] = append(e.grid[y], '#')
		}
	case inpututil.IsKeyJustPressed(ebiten.KeyLeft) && shrink && w > 3:
		for y := range e.grid {
			e.grid[y] = e.grid[y][:w-1]
		}
	case inpututil.IsKeyJustPressed(ebiten.KeyDown) && !shrink:
		e.grid = append(e.grid, []byte(strings.Repeat("#", w)))
	case inpututil.IsKeyJustPressed(ebiten.KeyUp) && shrink && len(e.grid) > 3:
		e.grid = e.grid[:len(e.grid)-1]
	}
}

func (e *editor) tileEvents() []*event {
	var out []*event
	for _, ev := range e.evs {
		if ev.x == e.selX && ev.y == e.selY {
			out = append(out, ev)
		}
	}
	return out
}

// panel layout (y offsets)
const (
	palY    = 10
	evY     = 110
	scriptY = 200
	scriptH = 300
	tmplY   = scriptY + scriptH + 20
)

func (e *editor) click(mx, my int) {
	if mx < panelX {
		return
	}
	px := mx - panelX
	// palette
	if my >= palY && my < palY+40 {
		if i := px / 44; i < len(e.palette) {
			e.brush = e.palette[i]
		}
		return
	}
	// script area
	if e.cur != nil && my >= scriptY && my < scriptY+scriptH {
		e.focus = true
		e.line = min((my-scriptY)/16, len(e.cur.body)-1)
		e.col = min(px/6, len(e.cur.body[e.line]))
		return
	}
	// templates: two columns
	if my >= tmplY {
		i := (my-tmplY)/16 + (px/220)*((winH-tmplY)/16)
		if i < len(templates) && e.cur != nil {
			e.cur.body = append(e.cur.body[:e.line+1], append([]string{templates[i]}, e.cur.body[e.line+1:]...)...)
			e.line++
			e.col = len(templates[i])
			e.focus = true
		}
	}
}

func (e *editor) editScript() {
	b := &e.cur.body
	if len(*b) == 0 {
		*b = []string{""}
	}
	l := &(*b)[e.line]
	for _, r := range ebiten.AppendInputChars(nil) {
		*l = (*l)[:e.col] + string(r) + (*l)[e.col:]
		e.col++
	}
	rep := func(k ebiten.Key) bool { // key repeat
		d := inpututil.KeyPressDuration(k)
		return d == 1 || d > 20 && d%3 == 0
	}
	switch {
	case rep(ebiten.KeyBackspace):
		if e.col > 0 {
			*l = (*l)[:e.col-1] + (*l)[e.col:]
			e.col--
		} else if e.line > 0 {
			e.col = len((*b)[e.line-1])
			(*b)[e.line-1] += *l
			*b = append((*b)[:e.line], (*b)[e.line+1:]...)
			e.line--
		}
	case inpututil.IsKeyJustPressed(ebiten.KeyEnter):
		rest := (*l)[e.col:]
		*l = (*l)[:e.col]
		*b = append((*b)[:e.line+1], append([]string{rest}, (*b)[e.line+1:]...)...)
		e.line++
		e.col = 0
	case rep(ebiten.KeyLeft) && e.col > 0:
		e.col--
	case rep(ebiten.KeyRight) && e.col < len(*l):
		e.col++
	case rep(ebiten.KeyUp) && e.line > 0:
		e.line--
	case rep(ebiten.KeyDown) && e.line < len(*b)-1:
		e.line++
	case inpututil.IsKeyJustPressed(ebiten.KeyEscape):
		e.focus = false
	}
	e.col = min(e.col, len((*b)[e.line]))
}

// ---------- draw ----------

func (e *editor) Draw(screen *ebiten.Image) {
	screen.Fill(color.RGBA{30, 30, 34, 255})
	for y, row := range e.grid {
		for x, ch := range row {
			if ch == '.' {
				vector.DrawFilledRect(screen, float32(x*cell), float32(y*cell), cell-1, cell-1, color.RGBA{60, 58, 55, 255}, false)
			} else if sw := e.swatch[ch]; sw != nil {
				op := &ebiten.DrawImageOptions{}
				op.GeoM.Scale(float64(cell-1)/float64(sw.Bounds().Dx()), float64(cell-1)/float64(sw.Bounds().Dy()))
				op.GeoM.Translate(float64(x*cell), float64(y*cell))
				screen.DrawImage(sw, op)
			} else {
				vector.DrawFilledRect(screen, float32(x*cell), float32(y*cell), cell-1, cell-1, color.RGBA{200, 50, 200, 255}, false)
			}
		}
	}
	for _, ev := range e.evs {
		cx, cy := float32(ev.x*cell+cell/2), float32(ev.y*cell+cell/2)
		c := color.RGBA{255, 210, 60, 255}
		if ev.use {
			c = color.RGBA{80, 200, 255, 255}
		}
		if ev.facing == 4 {
			vector.DrawFilledCircle(screen, cx, cy, 5, c, true)
		} else {
			d := [4][2]float32{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}[ev.facing]
			vector.StrokeLine(screen, cx, cy, cx+d[0]*10, cy+d[1]*10, 3, c, true)
			vector.DrawFilledCircle(screen, cx+d[0]*10, cy+d[1]*10, 3, c, true)
		}
	}
	for _, l := range e.extra { // props, floor items, monsters: their sprite at their cell
		f := strings.Fields(l)
		name, xi := f[1], 2
		if f[0] == "object" {
			name, xi = f[2], 3
		} else if f[0] == "item" {
			if it, ok := party.Catalog[f[1]]; ok {
				name = it.Icon
			}
		}
		var x, y int
		if len(f) > xi+1 {
			x, _ = strconv.Atoi(f[xi])
			y, _ = strconv.Atoi(f[xi+1])
		}
		if img := sprites.Get(name); img != nil && x >= 0 && y >= 0 {
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Scale(float64(cell-4)/float64(img.Bounds().Dy()), float64(cell-4)/float64(img.Bounds().Dy()))
			op.GeoM.Translate(float64(x*cell+2), float64(y*cell+2))
			screen.DrawImage(img, op)
		}
	}
	vector.StrokeRect(screen, float32(e.selX*cell)-1, float32(e.selY*cell)-1, cell+1, cell+1, 2, color.RGBA{255, 255, 255, 255}, false)

	// panel
	px := float32(panelX)
	vector.DrawFilledRect(screen, px, 0, winW-px, winH, color.RGBA{20, 20, 24, 255}, false)
	for i, ch := range e.palette {
		x := px + float32(i*44)
		if ch == '.' {
			vector.DrawFilledRect(screen, x, palY, 40, 40, color.RGBA{60, 58, 55, 255}, false)
		} else {
			op := &ebiten.DrawImageOptions{}
			sw := e.swatch[ch]
			op.GeoM.Scale(40/float64(sw.Bounds().Dx()), 40/float64(sw.Bounds().Dy()))
			op.GeoM.Translate(float64(x), palY)
			screen.DrawImage(sw, op)
		}
		if ch == e.brush {
			vector.StrokeRect(screen, x-1, palY-1, 42, 42, 2, color.RGBA{255, 255, 255, 255}, false)
		}
		ebitenutil.DebugPrintAt(screen, string(ch), int(x)+16, palY+42)
	}
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("map %s  %dx%d   Ctrl+S save   Shift+Arrows grow, Ctrl+Shift+Arrows shrink", e.name, len(e.grid[0]), len(e.grid)), panelX, palY+66)

	here := e.tileEvents()
	ebitenutil.DebugPrintAt(screen, fmt.Sprintf("tile %d,%d: %d event(s)   N new  F facing  U use  Tab next  Del remove", e.selX, e.selY, len(here)), panelX, evY)
	for i, ev := range here {
		s := fmt.Sprintf("  event %d,%d %s", ev.x, ev.y, facings[ev.facing])
		if ev.use {
			s += " use"
		}
		if ev == e.cur {
			s = ">" + s[1:]
		}
		ebitenutil.DebugPrintAt(screen, s, panelX, evY+16+i*16)
	}
	if e.cur != nil {
		bg := color.RGBA{35, 35, 45, 255}
		if e.focus {
			bg = color.RGBA{40, 45, 60, 255}
		}
		vector.DrawFilledRect(screen, px, scriptY, winW-px, scriptH, bg, false)
		for i, l := range e.cur.body {
			ebitenutil.DebugPrintAt(screen, l, panelX, scriptY+i*16)
		}
		if e.focus {
			vector.DrawFilledRect(screen, px+float32(e.col*6), float32(scriptY+e.line*16), 2, 16, color.RGBA{255, 255, 255, 255}, false)
		}
		ebitenutil.DebugPrintAt(screen, "click a keyword to insert it after the cursor line, Esc leaves the script:", panelX, tmplY-18)
		rows := (winH - tmplY) / 16
		for i, t := range templates {
			ebitenutil.DebugPrintAt(screen, t, panelX+(i/rows)*220, tmplY+(i%rows)*16)
		}
	}
	ebitenutil.DebugPrintAt(screen, e.status, 4, winH-18)
}

func (e *editor) Layout(int, int) (int, int) { return winW, winH }

func main() {
	e := &editor{name: "demo", brush: '#', swatch: map[byte]*ebiten.Image{}}
	if len(os.Args) > 1 {
		e.name = os.Args[1]
	}
	e.load()
	blobber.GenTextures()
	e.palette = []byte{'.'}
	for ch := range blobber.MatColors {
		e.palette = append(e.palette, ch)
		e.swatch[ch] = blobber.Material(ch)
	}
	sort.Slice(e.palette, func(i, j int) bool { return e.palette[i] < e.palette[j] })
	ebiten.SetWindowSize(winW, winH)
	ebiten.SetWindowTitle("Blobber map editor: " + e.name)
	if err := ebiten.RunGame(e); err != nil {
		log.Fatal(err)
	}
}
