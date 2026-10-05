package party

import (
	"fmt"
	"strings"
)

// Spell is one entry in the spell list. Knowing it is c.Has["spell:<ID>"], as scripts already address it.
type Spell struct {
	ID, Name, Desc string
	SP, Price      int
	Power          int // HP healed or damage dealt
}

// Spells is the full list, in the order the guild sells them.
var Spells = []Spell{
	{"light", "Light", "brighter torch for a minute", 1, 25, 0},
	{"heal", "Heal", "heals the most wounded 15 HP", 3, 60, 15},
	{"lightning", "Lightning", "fast bolt, 8 damage", 4, 120, 8},
	{"fireball", "Fireball", "slow bolt, 14 damage", 6, 200, 14},
}

// Casters are the classes that can learn spells.
var Casters = map[string]bool{"sorcerer": true, "cleric": true, "monk": true, "mage": true}

func SpellByID(id string) (Spell, bool) {
	for _, s := range Spells {
		if s.ID == id {
			return s, true
		}
	}
	return Spell{}, false
}

func (c *Character) Knows(id string) bool { return c.Has["spell:"+id] }

func (c *Character) CanLearn() bool { return Casters[strings.ToLower(c.Class)] }

// Learn teaches c spell id (the guild has already been paid).
func (c *Character) Learn(id string) error {
	switch _, ok := SpellByID(id); {
	case !ok:
		return fmt.Errorf("no spell %q", id)
	case !c.CanLearn():
		return fmt.Errorf("%s cannot learn spells", c.Name)
	case c.Knows(id):
		return fmt.Errorf("%s already knows it", c.Name)
	}
	c.Has["spell:"+id] = true
	return nil
}

// Cast spends c's SP on spell id and applies what the party itself can: heal.
// Light and the bolts are left to OnCast, which the game wires to the engine.
func (p *Party) Cast(c *Character, id string) (string, error) {
	s, ok := SpellByID(id)
	switch {
	case !ok || !c.Knows(id):
		return "", fmt.Errorf("%s does not know that spell", c.Name)
	case c.HP <= 0 || c.Has["condition:dead"]:
		return "", fmt.Errorf("%s cannot cast", c.Name)
	case c.SP < s.SP:
		return "", fmt.Errorf("not enough spell points")
	}
	c.SP -= s.SP
	msg := fmt.Sprintf("%s casts %s", c.Name, s.Name)
	if id == "heal" { // ponytail: auto-targets the most wounded; add a target picker when it matters
		var t *Character
		for _, x := range p.Chars {
			if x.HP > 0 && (t == nil || x.MaxHP-x.HP > t.MaxHP-t.HP) {
				t = x
			}
		}
		before := t.HP
		t.HP = min(t.MaxHP, t.HP+s.Power)
		msg += fmt.Sprintf(": %s +%d HP", t.Name, t.HP-before)
	}
	if p.OnCast != nil {
		p.OnCast(s)
	}
	return msg, nil
}
