package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"blobber"
	"blobber/events"
	"blobber/party"
	"blobber/ui"
)

var cdRoot = sync.OnceFunc(func() { os.Chdir("../..") })

// newTestHost starts a headless host on map m at (x, y).
func newTestHost(t *testing.T, m string, x, y int) *console {
	t.Helper()
	cdRoot()
	host := newConsole(party.New(party.NewCharacter("A", "knight", 30, 0), party.NewCharacter("B", "sorcerer", 12, 20)))
	w, err := host.enter(m)
	if err != nil {
		t.Fatal(err)
	}
	blobber.SetMap(w.lvl, x, y)
	return host
}

func use(h *console, x, y int) { pump(h.vm, h.vm.Use(h.m, x, y, events.N)) }

func TestTeleport(t *testing.T) {
	h := newTestHost(t, "demo", 26, 11)
	use(h, 26, 11)
	if x, y, _ := blobber.Pos(); x != 1 || y != 1 || h.m != "cave" {
		t.Fatalf("pos %d,%d map %s", x, y, h.m)
	}
	use(h, 1, 1)
	if x, y, _ := blobber.Pos(); x != 26 || y != 11 || h.m != "demo" {
		t.Fatalf("pos %d,%d map %s", x, y, h.m)
	}
}

func TestLever(t *testing.T) {
	h := newTestHost(t, "cave", 1, 5)
	lever, pulled := h.worlds["cave"].objs["lever"], h.worlds["cave"].objs["lever_pulled"]
	use(h, 1, 5)
	if it := h.FindItem("dagger"); it == nil || it.Bonus != 2 {
		t.Fatal("no dagger +2")
	}
	if pulled.X != 1 || pulled.Y != 5 || lever.X != -1 {
		t.Fatal("lever sprites not swapped")
	}
	if !h.Flags["cave:lever"] {
		t.Fatal("flag")
	}
}

func TestFloorItems(t *testing.T) {
	h := newTestHost(t, "cave", 7, 3)
	if got := h.TakeFloor(7, 3); len(got) != 1 || got[0] != "potion" || h.FindItem("potion") == nil {
		t.Fatalf("took %v", got)
	}
	if len(h.TakeFloor(7, 3)) != 0 {
		t.Fatal("item taken twice")
	}
}

// Monsters and decorations belong to their map and survive a round trip.
func TestPopulationPerMap(t *testing.T) {
	h := newTestHost(t, "demo", 26, 11)
	demo := blobber.SwapPopulation(nil) // peek: count demo actors by swapping out and back
	blobber.SwapPopulation(demo)
	h.Teleport("cave", 1, 1)
	cave := blobber.SwapPopulation(nil)
	blobber.SwapPopulation(cave)
	if cave == nil || demo == nil {
		t.Fatal("nil population")
	}
	h.Teleport("demo", 26, 11)
	back := blobber.SwapPopulation(nil)
	blobber.SwapPopulation(back)
	if back.Len() != demo.Len() || cave.Len() == demo.Len() {
		t.Fatalf("demo %d cave %d back %d", demo.Len(), cave.Len(), back.Len())
	}
}

// TestLab: the lab demonstrates every wired opcode, and every station runs headlessly.
func TestLab(t *testing.T) {
	h := newTestHost(t, "lab", 1, 2)
	src, _ := os.ReadFile("maps/lab.evt")
	words := map[string]bool{}
	for _, l := range strings.Split(string(src), "\n") {
		if f := strings.Fields(l); len(f) > 0 {
			words[f[0]] = true
		}
	}
	for _, op := range strings.Fields(`sound music cutscene town alterwall nop exit return remove nothing text npc teleport fall if give take set setchar
		selrndchar whowill damage rnddamage spawn altercell moveobj movewall exchobj giveitem itemtype goto
		gotorandom jumprnd choose confirm call alter`) {
		if !words[op] {
			t.Errorf("lab does not demonstrate %s", op)
		}
	}
	for x := 1; x <= 39; x += 2 {
		for _, y := range []int{1, 3} {
			h.enter("lab")
			blobber.SetMap(h.worlds["lab"].lvl, x, y)
			use(h, x, y)
		}
	}
	if h.m != "demo" && h.m != "lab" {
		t.Fatal(h.m)
	}
}

func TestAlterWallAndTown(t *testing.T) {
	h := newTestHost(t, "lab", 29, 3)
	use(h, 29, 3)
	if blobber.Cell(29, 4) != '.' {
		t.Fatal("wall not opened")
	}
	use(h, 29, 3)
	if blobber.Cell(29, 4) != 'I' {
		t.Fatalf("wall not closed: %q flags %v", blobber.Cell(29, 4), h.Flags)
	}
	blobber.Modal = nil
	use(h, 35, 1)
	shop, ok := blobber.Modal.(*ui.Shop)
	if !ok {
		t.Fatal("no smith")
	}
	h.Gold = 100
	shop.Choose(0) // cheapest item
	if h.Gold >= 100 || len(h.Inventory) == 0 {
		t.Fatal("bought nothing", h.Gold)
	}
	blobber.Modal = nil
	use(h, 33, 1)
	if c, ok := blobber.Modal.(*ui.Cutscene); !ok || len(c.Pages) != 4 {
		t.Fatalf("cutscene %#v", blobber.Modal)
	}
	blobber.Modal = nil
}

// Every npc a script names is in the registry, with a portrait from the pool.
func TestNPCRegistry(t *testing.T) {
	cdRoot()
	f, err := os.Open("maps/npcs.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	ui.LoadNPCs(f)
	files, _ := filepath.Glob("maps/*.evt")
	re := regexp.MustCompile(`(?m)^\s*npc\s+(\S+)`)
	for _, fn := range files {
		b, _ := os.ReadFile(fn)
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			if n, ok := ui.NPCs[m[1]]; !ok || ui.PortraitID(n.Portrait) == nil {
				t.Errorf("%s: npc %q missing from maps/npcs.txt or its portrait", fn, m[1])
			}
		}
	}
}
