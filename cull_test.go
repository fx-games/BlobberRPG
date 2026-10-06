package blobber

import (
	"image"
	"math"
	"os"
	"testing"
)

var testLevel = []string{
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

// Visible cells must be a subset of the old radius box, and far fewer on average.
func TestVisibleCellsCullsRadiusBox(t *testing.T) {
	level = testLevel
	var old, new int
	for y := range level {
		for x := range level[y] {
			if wall(x, y) {
				continue
			}
			for dir := 0; dir < 4; dir++ {
				ang := float64(dir) * math.Pi / 2
				c := cam{x: float64(x) + 0.5, z: float64(y) + 0.5, ang: ang}
				c.fwx, c.fwz = math.Sin(ang), -math.Cos(ang)
				c.rtx, c.rtz = math.Cos(ang), math.Sin(ang)
				for cell := range visibleCells(c) {
					if cell[0] < x-2*radius || cell[0] > x+2*radius || cell[1] < y-2*radius || cell[1] > y+2*radius || wall(cell[0], cell[1]) {
						t.Fatalf("bad visible cell %v from %d,%d", cell, x, y)
					}
					new++
				}
				for cy := y - radius; cy <= y+radius; cy++ {
					for cx := x - radius; cx <= x+radius; cx++ {
						if !wall(cx, cy) {
							old++
						}
					}
				}
			}
		}
	}
	t.Logf("open cells drawn, summed over every position+facing: radius box %d, raycast %d (%.0f%%)", old, new, 100*float64(new)/float64(old))
	if new*2 > old {
		t.Fatalf("culling barely helps: %d vs %d", new, old)
	}
}

func BenchmarkVisibleCells(b *testing.B) {
	c := cam{x: 1.5, z: 1.5, fwx: 0, fwz: -1, rtx: 1, rtz: 0}
	for i := 0; i < b.N; i++ {
		visCache.cells = nil
		visibleCells(c)
	}
}

func TestBoltHitsActor(t *testing.T) {
	defer SwapPopulation(SwapPopulation(nil))
	SpawnActor("rat", image.NewRGBA(image.Rect(0, 0, 1, 1)), 3, 4, 10)
	if HitMonster(2, 4, 5) || !HitMonster(3, 4, 5) || len(actors) != 1 {
		t.Fatal("first hit wounds")
	}
	killed := false
	OnKill = func(m Monster) { killed = m.Name == "rat" && m.X == 3 && m.Y == 4 }
	defer func() { OnKill = nil }()
	if !HitMonster(3, 4, 5) || len(actors) != 0 || !killed {
		t.Fatal("second hit kills")
	}
}

func TestDoorIsPassable(t *testing.T) {
	old := level
	defer func() { level = old }()
	level = []string{"###", "#D#", "#L#"}
	if wall(1, 1) || !wall(1, 2) || !wall(0, 1) {
		t.Fatal("D open, L and # closed")
	}
}

// A sheet-backed monster plays hurt on a wound and death on a kill, and is gone once death ends.
func TestSheetAnimation(t *testing.T) {
	defer SwapPopulation(SwapPopulation(nil))
	s, err := LoadSheet(os.DirFS("cmd/blobber/assets/monsters"), "ice_giant")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Anims["idle"].Frames) != 4 || len(s.Anims["death"].Frames) != 5 || s.Anims["attack"].Loop {
		t.Fatalf("rows: %+v", s.Anims)
	}
	SpawnSheet("ice_giant", s, 2, 2, 10)
	g := &Game{}
	HitMonster(2, 2, 3)
	if a := actorAt(2, 2); a == nil || a.anim != s.Anims["hurt"] {
		t.Fatal("hurt not playing")
	}
	HitMonster(2, 2, 10)
	if actorAt(2, 2) != nil || len(actors) != 1 || !actors[0].dying || MonsterAt(2, 2) != "" {
		t.Fatal("dying monster should linger but not count")
	}
	g.updateActors(10)
	if len(actors) != 0 {
		t.Fatal("death animation should end and remove the actor")
	}
}
