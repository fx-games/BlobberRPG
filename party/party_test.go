package party

import "testing"

func TestProps(t *testing.T) {
	p := New(NewCharacter("Ann", "knight", 30, 0), NewCharacter("Bob", "sorcerer", 12, 20))
	p.Give(0, "gold", "", 50)
	if p.Gold != 50 || p.Get(1, "gold", "") != 50 {
		t.Fatal("gold")
	}
	if p.Take(0, "gold", "", 80) || p.Gold != 0 {
		t.Fatal("take more than we have must fail and floor at 0")
	}
	p.Give(2, "item", "wand", 1)
	if p.Get(0, "item", "wand") != 1 || !p.Take(0, "item", "wand", 1) || p.Get(0, "item", "wand") != 0 || p.Take(0, "item", "wand", 1) {
		t.Fatal("items")
	}
	p.Give(0, "key", "brass", 1)
	if p.Get(0, "key", "brass") != 1 || !p.Take(0, "key", "brass", 1) || p.Get(0, "key", "brass") != 0 {
		t.Fatal("keys")
	}
	p.Give(1, "award", "hero", 1)
	p.Set(0, "flag", "met_king", 1)
	if p.Get(1, "award", "hero") != 1 || p.Get(2, "award", "hero") != 0 || p.Get(0, "flag", "met_king") != 1 {
		t.Fatal("award/flag")
	}
	p.Give(1, "mighttmp", "", 5)
	p.Set(2, "fire", "", 4)
	if p.Get(1, "mighttmp", "") != 5 || p.Get(1, "might", "") != 10 {
		t.Fatal("stats")
	}
	p.Damage(0, 6, "fire") // Bob resists 4
	if p.Chars[0].HP != 24 || p.Chars[1].HP != 10 {
		t.Fatalf("hp %d %d", p.Chars[0].HP, p.Chars[1].HP)
	}
	p.Give(0, "hp", "", 100)
	if p.Chars[0].HP != 30 {
		t.Fatal("hp must clamp to max")
	}
	p.Damage(2, 0, "dead")
	if p.Get(2, "condition", "dead") != 1 || p.Get(1, "class", "knight") != 1 {
		t.Fatal("dead / class")
	}
	if len(p.Summary()) < 4 {
		t.Fatal("summary")
	}
}

func TestEquip(t *testing.T) {
	ann := NewCharacter("Ann", "knight", 30, 0)
	p := New(ann)
	p.Give(0, "item", "ring", 3)
	p.Give(0, "item", "shield", 1)
	if len(p.Inventory) != 2 || len(p.Inventory[0]) != 3 {
		t.Fatalf("stacks %v", p.Inventory)
	}
	ac := ann.AC
	r1, r2, r3 := p.Inventory[0][0], p.Inventory[0][1], p.Inventory[0][2]
	p.Equip(ann, r1)
	p.Equip(ann, r2)
	if ann.Equipped[RingL] != r1 || ann.Equipped[RingR] != r2 || ann.AC != ac+2 {
		t.Fatalf("rings %v ac %d", ann.Equipped, ann.AC)
	}
	if len(p.Inventory) != 4 { // ring x1, shield, ring(worn), ring(worn)
		t.Fatalf("worn items must not stack: %d stacks", len(p.Inventory))
	}
	p.Equip(ann, r3) // both ring slots full: replaces the left one
	if ann.Equipped[RingL] != r3 || r1.Wearer != nil || ann.AC != ac+2 {
		t.Fatal("ring swap")
	}
	p.RemoveItem(r2) // dropping worn item unequips
	if ann.Equipped[RingR] != nil || ann.AC != ac+1 {
		t.Fatal("drop worn")
	}
	ann.HP = 10
	p.Give(0, "item", "potion", 1)
	if msg, err := p.Use(ann, p.FindItem("potion")); err != nil || ann.HP != 20 || p.FindItem("potion") != nil {
		t.Fatal(msg, err)
	}
	if _, err := p.Use(ann, p.FindItem("shield")); err == nil {
		t.Fatal("shield is not usable")
	}
	if err := p.Equip(ann, NewItem("gem", 0)); err == nil {
		t.Fatal("gem is not equippable")
	}
}

func TestEquipAt(t *testing.T) {
	ann := NewCharacter("Ann", "knight", 10, 0)
	p := New(ann)
	r, h := NewItem("ring", 0), NewItem("helmet", 0)
	p.AddItem(r)
	p.AddItem(h)
	if p.EquipAt(ann, r, RingR) != nil || ann.Equipped[RingR] != r {
		t.Fatal("ring into right ring slot")
	}
	if p.EquipAt(ann, h, Body) == nil || h.Wearer != nil {
		t.Fatal("helmet must not fit the body slot")
	}
}

func TestSpells(t *testing.T) {
	monk, knight := NewCharacter("Bob", "monk", 20, 10), NewCharacter("Ann", "knight", 30, 0)
	p := New(knight, monk)
	if knight.Learn("heal") == nil {
		t.Fatal("knights cannot learn")
	}
	if monk.Learn("heal") != nil || monk.Learn("heal") == nil {
		t.Fatal("learn once")
	}
	knight.HP = 10
	var cast []string
	p.OnCast = func(s Spell) { cast = append(cast, s.ID) }
	if _, err := p.Cast(monk, "heal"); err != nil || knight.HP != 25 || monk.SP != 7 || len(cast) != 1 {
		t.Fatalf("%v hp %d sp %d", err, knight.HP, monk.SP)
	}
	if _, err := p.Cast(monk, "fireball"); err == nil {
		t.Fatal("unknown spell cast")
	}
	monk.Learn("fireball")
	monk.SP = 2
	if _, err := p.Cast(monk, "fireball"); err == nil {
		t.Fatal("cast without SP")
	}
}
