package main

import (
	"bufio"
	"fmt"
	"io"
	"math/rand"
	"strconv"
	"strings"

	"blobber"
	"blobber/party"
	"blobber/sprites"
)

// kind is one row of maps/monsters.txt.
type kind struct {
	Sprite           string
	HP, AC, Dmg, Exp int
}

var kinds = map[string]kind{}

// loadMonsters reads "id sprite hp ac dmg exp" lines; # starts a comment.
func loadMonsters(r io.Reader) error {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Fields(strings.SplitN(sc.Text(), "#", 2)[0])
		if len(f) == 0 {
			continue
		}
		if len(f) != 6 {
			return fmt.Errorf("monsters.txt: %q: want id sprite hp ac dmg exp", sc.Text())
		}
		var n [4]int
		for i := range n {
			n[i], _ = strconv.Atoi(f[2+i])
		}
		kinds[f[0]] = kind{f[1], n[0], n[1], n[2], n[3]}
	}
	return sc.Err()
}

// kindOf looks a monster up; unknown ids are drawn with a sprite of their own name and get stock stats.
func kindOf(id string) kind {
	if k, ok := kinds[id]; ok {
		return k
	}
	return kind{id, 10, 3, 3, 5}
}

// spawn places monster id at (x, y) with hp hit points (hp < 0 = full).
func (c *console) spawn(id string, x, y, hp int) error {
	k := kindOf(id)
	if hp < 0 {
		hp = k.HP
	}
	if s, ok := sheets[k.Sprite]; ok {
		blobber.SpawnSheet(id, s, x, y, hp)
		return nil
	}
	if img := sprites.Get(k.Sprite); img != nil {
		blobber.SpawnActor(id, img, x, y, hp)
		return nil
	}
	if s, err := loadSheet(k.Sprite); err == nil {
		sheets[k.Sprite] = s
		blobber.SpawnSheet(id, s, x, y, hp)
		return nil
	}
	img, err := loadMonster(k.Sprite)
	if err != nil {
		fmt.Println("  spawn failed:", err)
		return err
	}
	blobber.SpawnActor(id, img, x, y, hp)
	return nil
}

// sheets caches loaded sprite sheets by folder name.
var sheets = map[string]*blobber.Sheet{}

// ---------- turn-based rules ----------
// ponytail: d20 rolls against AC, one attack per character and monster, no criticals, no ranged row.

var roll = rand.Intn

// attack is the party striking m: every living character swings once.
func (c *console) attack(m blobber.Monster) {
	k := kindOf(m.Name)
	for _, ch := range c.Chars {
		if ch.HP <= 0 {
			continue
		}
		if roll(20)+1+ch.Level+ch.Num["acc"]/5 <= k.AC+10 {
			blobber.Message(ch.Name + " misses the " + m.Name)
			continue
		}
		dmg := 2 // fists
		if w := ch.Equipped[party.RightHand]; w != nil {
			dmg = w.Damage + w.Bonus
		}
		dmg += ch.Num["might"] / 10
		blobber.Message(fmt.Sprintf("%s hits the %s for %d", ch.Name, m.Name, dmg))
		c.PlaySound("hit")
		if !blobber.HitMonster(m.X, m.Y, dmg) {
			return // dead
		}
	}
}

// defend is monster m striking a random living character.
func (c *console) defend(m blobber.Monster) {
	var alive []int
	for i, ch := range c.Chars {
		if ch.HP > 0 {
			alive = append(alive, i+1)
		}
	}
	if len(alive) == 0 {
		return
	}
	who := alive[roll(len(alive))]
	ch := c.Chars[who-1]
	if roll(20)+1 <= ch.AC+5 {
		blobber.Message("The " + m.Name + " misses " + ch.Name)
		return
	}
	dmg := roll(kindOf(m.Name).Dmg) + 1
	blobber.Message(fmt.Sprintf("The %s hits %s for %d", m.Name, ch.Name, dmg))
	c.Damage(who, dmg, "physical")
	if ch.HP <= 0 {
		blobber.Message(ch.Name + " falls!")
	}
}

// killed awards the monster's experience to the living.
func (c *console) killed(m blobber.Monster) {
	exp := kindOf(m.Name).Exp
	for _, ch := range c.Chars {
		if ch.HP > 0 {
			ch.Exp += exp
		}
	}
	blobber.Message(fmt.Sprintf("The %s dies. +%d exp", m.Name, exp))
}

// tick is one party turn: a minute passes.
func (c *console) tick() {
	if c.Minutes++; c.Minutes >= 24*60 {
		c.Minutes, c.Day = 0, c.Day+1
	}
}

// Wiped reports whether nobody is left standing.
func (c *console) Wiped() bool {
	for _, ch := range c.Chars {
		if ch.HP > 0 {
			return false
		}
	}
	return true
}

// installCombat wires the engine's turn hooks to the rules above.
func (c *console) installCombat() {
	blobber.OnBump = c.attack
	blobber.OnAttack = c.defend
	blobber.OnKill = c.killed
	blobber.OnTurn = c.tick
}
