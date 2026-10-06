package ui

import (
	"fmt"
	"image"

	"blobber"
	"blobber/font"
	"blobber/party"
	"blobber/sprites"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	hitFrames      = 70 // how long a damage icon stays on a portrait
	cardW, cardH   = 142, 61
	cardGap        = 2
	portraitX, pY0 = 8, 8 // just inside the frame's 8 px border
	textX          = portraitX + PortraitSize + 6
)

type hitMark struct {
	kind string
	n, t int
}

var (
	frameNo int
	hits    = map[int]hitMark{} // 0-based character -> last hit
	cards   []*ebiten.Image
)

// Hit shows a damage icon of the given kind over character who's (1-based) portrait.
// It matches party.Party.OnHit.
func Hit(who, n int, kind string) { hits[who-1] = hitMark{kind, n, frameNo} }

// card is character i's rectangle on screen, in device pixels, for a party of n.
func card(i, n int) image.Rectangle {
	k := K(cw, ch)
	if (cardW+cardGap)*k*n > blobber.W { // six cards: shrink until the row fits
		k = max(1, blobber.W/((cardW+cardGap)*n))
	}
	w := (cardW + cardGap) * k
	x := (blobber.W-w*n)/2 + i*w
	y := blobber.H - (cardH+4)*k
	return image.Rect(x, y, x+cardW*k, y+cardH*k)
}

// PartyAt returns the 0-based character whose card contains screen point x,y, or -1.
func PartyAt(p *party.Party, x, y int) int {
	for i := range p.Chars {
		if image.Pt(x, y).In(card(i, len(p.Chars))) {
			return i
		}
	}
	return -1
}

// DrawParty draws one framed card per character along the bottom edge: portrait, name, HP and SP bars,
// and for a moment after a hit the damage-type icon and amount over the portrait.
func DrawParty(screen *ebiten.Image, p *party.Party) {
	frameNo++
	k := K(cw, ch)
	for len(cards) < len(p.Chars) {
		cards = append(cards, ebiten.NewImage(cardW, cardH))
	}
	for i, c := range p.Chars {
		d := cards[i]
		d.Clear()
		drawCard(d, i, c)
		r := card(i, len(p.Chars))
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Scale(float64(k), float64(k))
		op.GeoM.Translate(float64(r.Min.X), float64(r.Min.Y))
		screen.DrawImage(d, op)
	}
}

func drawCard(d *ebiten.Image, i int, c *party.Character) {
	DarkFrame(d, rc(0, 0, cardW, cardH))
	img := Portrait(c)
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(portraitX, pY0)
	dead := c.HP <= 0 || c.Has["condition:dead"]
	if dead {
		op.ColorScale.Scale(0.35, 0.35, 0.35, 1)
	}
	h, hit := hits[i]
	age := frameNo - h.t
	hit = hit && age < hitFrames
	if hit && age < 10 {
		op.ColorScale.Scale(1, 0.5, 0.5, 1) // brief red flash
	}
	d.DrawImage(img, op)

	name := c.Name
	for _, k := range party.Conditions {
		if c.Has["condition:"+k] {
			name += " - " + k
			break
		}
	}
	if dead && !c.Has["condition:dead"] {
		name += " - dead"
	}
	Print(d, name, textX, 6, gold)
	hp := GreenFill
	if c.HP*4 < c.MaxHP {
		hp = RedFill
	}
	Bar(d, textX-4, 24, cardW-textX-4, c.HP, c.MaxHP, hp)
	if c.MaxSP > 0 {
		Bar(d, textX-4, 37, cardW-textX-4, c.SP, c.MaxSP, BlueFill)
	}
	if hit {
		drawHit(d, h, age)
	}
}

// drawHit puts the kind's icon over the portrait, rising and fading out at the end.
func drawHit(d *ebiten.Image, h hitMark, age int) {
	ic := sprites.Get("hit_" + h.kind)
	if ic == nil {
		ic = sprites.Get("hit_physical")
	}
	b := ic.Bounds()
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(2, 2)
	op.GeoM.Translate(float64(portraitX+PortraitSize/2-b.Dx()), float64(pY0+PortraitSize/2-b.Dy()-age/12))
	if fade := hitFrames - age; fade < 20 {
		op.ColorScale.ScaleAlpha(float32(fade) / 20)
	}
	d.DrawImage(ic, op)
	if h.kind != "dead" {
		font.Print(d, fmt.Sprintf("-%d", h.n), portraitX+PortraitSize-14, pY0+PortraitSize-16)
	}
}
