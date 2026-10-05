package main

import (
	"bytes"
	"cmp"
	"embed"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"strings"

	"blobber"
	"blobber/events"
	"blobber/maps"
	"blobber/party"
	"blobber/sfx"
	"blobber/ui"

	"github.com/hajimehoshi/ebiten/v2"
)

//go:embed assets/monsters/*.png
var monsters embed.FS

// loadMonster decodes assets/monsters/<name>.png.
func loadMonster(name string) (image.Image, error) {
	b, err := monsters.ReadFile("assets/monsters/" + name + ".png")
	if err != nil {
		return nil, err
	}
	return png.Decode(bytes.NewReader(b))
}

func main() {
	pt := party.New(party.NewCharacter("Ann", "knight", 30, 0), party.NewCharacter("Bob", "monk", 12, 20), party.NewCharacter("Cyd", "robber", 20, 0))
	pt.Chars[0].Sex = "f"
	pt.Gold = 500
	for _, s := range []string{"dagger", "chainmail", "shield", "helmet", "boots", "ring"} {
		pt.AddItem(party.NewItem(s, 0))
	}
	pt.AddItem(party.NewItem("potion", 0))
	pt.AddItem(party.NewItem("potion", 0))
	if f, err := maps.Open("npcs.txt"); err == nil {
		ui.LoadNPCs(f)
		f.Close()
	}
	host := newConsole(pt)
	vm := host.vm
	w, err := host.enter("demo")
	if err != nil {
		log.Fatal(err)
	}

	blobber.DebugLines = pt.Summary
	blobber.HUD = func(s *ebiten.Image) { ui.DrawParty(s, pt) }
	blobber.TileMarks = func() map[[2]int]int { return vm.Marks(host.m) }
	blobber.OnPromptAnswer = func() []string { // character list for "who will" prompts
		var l []string
		for i, c := range pt.Chars {
			l = append(l, fmt.Sprintf("  %d  %s", i+1, c.Name))
		}
		return l
	}
	blobber.OnMove = func(x, y, dir int) {
		blobber.ClearSign()
		pump(vm, vm.Trigger(host.m, x, y, events.Facing(dir)))
	}
	// F: take what lies here; with nothing to take, search the tile.
	blobber.OnUse = func(x, y, dir int) {
		if got := host.TakeFloor(x, y); len(got) > 0 {
			blobber.ShowSign(x, y, dir, []string{"Taken: " + strings.Join(got, ", ")})
			return
		}
		pump(vm, vm.Use(host.m, x, y, events.Facing(dir)))
	}
	blobber.Keys[ebiten.KeyI] = func() { blobber.Modal = ui.NewInventory(pt) }
	sfx.Init()
	blobber.Keys[ebiten.KeyC] = func() { blobber.Modal = ui.NewSpellbook(pt) }
	pt.OnCast = func(s party.Spell) {
		sfx.Play("magic")
		switch s.ID {
		case "light":
			blobber.Light(60)
		case "lightning":
			blobber.Bolt(s.Power, 14, 0.6, 0.8, 1)
		case "fireball":
			blobber.Bolt(s.Power, 5, 1, 0.3, 0.1)
		}
	}
	blobber.OnKill = func(x, y int) { sfx.Play("fanfare") }
	pt.OnHit = func(who, n int, kind string) {
		ui.Hit(who, n, kind)
		sfx.Play("hit")
	}
	// clicking a portrait opens that character's inventory
	blobber.OnClick = func(x, y int) bool {
		i := ui.PartyAt(pt, x, y)
		if i >= 0 {
			inv := ui.NewInventory(pt)
			inv.Select(i)
			blobber.Modal = inv
		}
		return i >= 0
	}

	ui.Install()
	devOpen(host)

	if err := blobber.Run(w.lvl, 1, 1, 1); err != nil {
		log.Fatal(err)
	}
}

// devOpen opens windows named in BLOBBER_OPEN (comma separated) at startup, for checking the look
// together with BLOBBER_SHOT: inventory[:ITEM to wear], town:KIND, cutscene:NAME, hit, prompt, sign.
func devOpen(host *console) {
	for _, s := range strings.Split(os.Getenv("BLOBBER_OPEN"), ",") {
		kind, arg, _ := strings.Cut(s, ":")
		switch kind {
		case "inventory":
			inv := ui.NewInventory(host.Party)
			if arg != "" { // a character's slots with the first equippable item worn
				inv.Select(0)
				host.Party.Equip(host.Party.Chars[0], host.Party.FindItem(arg))
			}
			blobber.Modal = inv
		case "town":
			host.Town(arg)
		case "cutscene":
			host.Cutscene(arg)
		case "hit":
			host.Party.Damage(0, 3, cmp.Or(arg, "fire"))
		case "spells": // Bob knows every spell, book open
			for _, s := range party.Spells {
				host.Party.Chars[1].Learn(s.ID)
			}
			blobber.Modal = ui.NewSpellbook(host.Party)
		case "npc": // npc:ID shows that NPC talking
			blobber.Modal = &ui.Talk{Who: arg, Text: "Peace, travellers. The temple heals the wounded and lifts curses, for an offering. Will you give?", Ask: true}
		case "prompt":
			blobber.ShowPrompt([]string{"Who will search the rubble?", "  1  Ann", "  2  Bob", "  3  Cyd"}, false, func(string) {})
		case "sign": // after the first frame: Run clears signs
			hud := blobber.HUD
			blobber.HUD = func(s *ebiten.Image) {
				blobber.ShowSign(1, 1, 1, []string{"Welcome to the demo dungeon.", "Press F here for the opcode lab."})
				blobber.HUD = hud
				hud(s)
			}
		}
	}
}
