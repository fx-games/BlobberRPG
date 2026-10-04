package main

import (
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestCore(t *testing.T) {
	if !wall(-1, 0) || !wall(0, 0) || wall(1, 1) {
		t.Fatal("wall(): outside map and '#' must be wall, '.' must not")
	}
	if _, err := ebiten.NewShader([]byte(ditherSrc)); err != nil {
		t.Fatal("dither shader:", err)
	}
	g := &Game{x: 1, y: 1, dir: 0, smooth: true, t: 1, tx: 1.5, tz: 1.5}
	g.act(ebiten.KeyW) // wall to the north: no move
	if g.moving {
		t.Fatal("walked into a wall")
	}
	g.act(ebiten.KeyQ) // turn N->W wraps angle the short way
	if !g.moving || g.dur != 0.22 || math.Abs(g.ta-g.fa) > math.Pi+1e-9 {
		t.Fatalf("turn tween wrong: moving=%v dur=%v fa=%v ta=%v", g.moving, g.dur, g.fa, g.ta)
	}
	g.act(ebiten.KeyS)
	if g.queued != ebiten.KeyS {
		t.Fatal("input not queued while moving")
	}
}
