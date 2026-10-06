package main

import (
	"bufio"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"

	"blobber"
	"blobber/events"
	"blobber/maps"
	"blobber/party"
	"blobber/ui"
)

// start is where a new game begins, from maps/world.txt.
type start struct {
	title     string
	m         string
	x, y, dir int
}

// loadWorld reads maps/world.txt: "title ..." and "start MAP X Y DIR".
func loadWorld() start {
	s := start{title: "Blobber", m: "demo", x: 1, y: 1, dir: 1}
	f, err := maps.Open("world.txt")
	if err != nil {
		return s
	}
	defer f.Close()
	for sc := bufio.NewScanner(f); sc.Scan(); {
		fs := strings.Fields(strings.SplitN(sc.Text(), "#", 2)[0])
		switch {
		case len(fs) >= 2 && fs[0] == "title":
			s.title = strings.Join(fs[1:], " ")
		case len(fs) == 5 && fs[0] == "start":
			s.m = fs[1]
			s.x, _ = strconv.Atoi(fs[2])
			s.y, _ = strconv.Atoi(fs[3])
			s.dir = max(0, strings.Index("nesw", strings.ToLower(fs[4])))
		}
	}
	return s
}

// newParty is the default party a new game starts with.
// ponytail: fixed heroes; make this a "party" block in world.txt when campaigns differ.
func newParty() *party.Party {
	pt := party.New(party.NewCharacter("Ann", "knight", 30, 0), party.NewCharacter("Bob", "monk", 12, 20), party.NewCharacter("Cyd", "robber", 20, 0))
	pt.Chars[0].Sex = "f"
	pt.Gold = 500
	for _, s := range []string{"bronze_dagger", "chainmail", "oak_shield", "helmet", "boots", "ring", "potion", "potion"} {
		pt.AddItem(party.NewItem(s, 0))
	}
	return pt
}

// reset swaps in party p (keeping the UI's callbacks) and forgets every loaded map and script.
func (c *console) reset(p *party.Party) {
	onHit, onCast := c.OnHit, c.OnCast
	*c.Party = *p
	c.OnHit, c.OnCast = onHit, onCast
	c.vm, c.worlds, c.m = events.New(c), map[string]*world{}, ""
	blobber.SwapPopulation(nil)
	blobber.Modal = nil
}

// newGame starts over at the world's start position.
func (c *console) newGame() {
	c.reset(newParty())
	w, err := c.enter(c.start.m)
	if err != nil {
		blobber.Message(err.Error())
		return
	}
	blobber.SetMap(w.lvl, c.start.x, c.start.y)
	blobber.Face(c.start.dir)
}

// title shows the title menu as a prompt; it stays up until a choice starts or loads a game.
func (c *console) title() {
	lines := []string{strings.ToUpper(c.start.title), "", "  1  New game", "  2  Load game"}
	if runtime.GOOS != "js" {
		lines = append(lines, "  3  Quit")
	}
	blobber.ShowPrompt(lines, false, func(a string) {
		switch a {
		case "1":
			c.newGame()
		case "2":
			if err := c.loadGame(); err != nil {
				blobber.Message(err.Error())
				c.title()
			}
		case "3":
			if runtime.GOOS != "js" {
				os.Exit(0)
			}
			c.title()
		default:
			c.title()
		}
	})
}

// End is the win / gameover opcode: a last word, then the title. A cutscene already on screen plays first.
func (c *console) End(won bool) {
	text := "Your quest is over. Thank you for playing!"
	if !won {
		text = "Your quest has failed."
	}
	last := &ui.Talk{Text: text, Done: func(bool) { c.title() }}
	if m := blobber.Modal; m != nil {
		blobber.Modal = &then{m, func() { blobber.Modal = last }}
		return
	}
	blobber.Modal = last
}

// then runs f when the wrapped overlay closes.
type then struct {
	blobber.Overlay
	f func()
}

func (t *then) Update() bool {
	if t.Overlay.Update() {
		return true
	}
	t.f()
	return blobber.Modal != t // f may have opened the next window
}

// rest sleeps eight hours: full heal and spell points for a ration each, unless monsters are near.
// ponytail: no random encounters, no partial rest.
func (c *console) rest() {
	x, y, _ := blobber.Pos()
	for _, m := range blobber.Live().Monsters() {
		if abs(m.X-x)+abs(m.Y-y) <= blobber.Sight {
			blobber.Message("Too dangerous to rest here.")
			return
		}
	}
	if c.Food < len(c.Chars) {
		blobber.Message("Not enough food to rest.")
		return
	}
	c.Food -= len(c.Chars)
	for _, ch := range c.Chars {
		if ch.HP > 0 {
			ch.HP, ch.SP = ch.MaxHP, ch.MaxSP
		}
	}
	for range 8 * 60 {
		c.tick()
	}
	blobber.Message(fmt.Sprintf("You rest. Day %d, %02d:%02d.", c.Day, c.Minutes/60, c.Minutes%60))
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
