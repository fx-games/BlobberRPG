package blobber

// Gamepad support: every connected standard-layout gamepad presses the keyboard keys the game
// already reads, so all screens work without knowing about pads. Read keys through KeyJustPressed
// and KeyPressed instead of inpututil/ebiten directly.
//
//	d-pad / left stick   arrows: walk and turn, move through menus
//	LB / RB              A / D: strafe
//	A                    F (use) in the dungeon; Space in the menu; Enter, Space in windows
//	B                    Esc
//	X                    Space: wait a turn
//	Y                    I: inventory
//	LT / RT              C: spellbook, J: journal
//	Back / Start         M: map, Esc: menu

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

var padHeld, padPrev = map[ebiten.Key]bool{}, map[ebiten.Key]bool{}

// pollPads works out which keys the pads hold this frame; context is where input goes:
// 'w' the dungeon, 'm' the game menu, anything else a window or prompt.
func pollPads(context byte) {
	padPrev, padHeld = padHeld, map[ebiten.Key]bool{}
	hold := func(on bool, ks ...ebiten.Key) {
		if on {
			for _, k := range ks {
				padHeld[k] = true
			}
		}
	}
	for _, id := range ebiten.AppendGamepadIDs(nil) {
		if !ebiten.IsStandardGamepadLayoutAvailable(id) {
			continue
		}
		b := func(s ebiten.StandardGamepadButton) bool { return ebiten.IsStandardGamepadButtonPressed(id, s) }
		x := ebiten.StandardGamepadAxisValue(id, ebiten.StandardGamepadAxisLeftStickHorizontal)
		y := ebiten.StandardGamepadAxisValue(id, ebiten.StandardGamepadAxisLeftStickVertical)
		hold(b(ebiten.StandardGamepadButtonLeftTop) || y < -0.5, ebiten.KeyUp)
		hold(b(ebiten.StandardGamepadButtonLeftBottom) || y > 0.5, ebiten.KeyDown)
		hold(b(ebiten.StandardGamepadButtonLeftLeft) || x < -0.5, ebiten.KeyLeft)
		hold(b(ebiten.StandardGamepadButtonLeftRight) || x > 0.5, ebiten.KeyRight)
		hold(b(ebiten.StandardGamepadButtonCenterRight), ebiten.KeyEscape)
		switch context {
		case 'w':
			hold(b(ebiten.StandardGamepadButtonFrontTopLeft), ebiten.KeyA)
			hold(b(ebiten.StandardGamepadButtonFrontTopRight), ebiten.KeyD)
			hold(b(ebiten.StandardGamepadButtonRightBottom), ebiten.KeyF)
			hold(b(ebiten.StandardGamepadButtonRightLeft), ebiten.KeySpace)
			hold(b(ebiten.StandardGamepadButtonRightTop), ebiten.KeyI)
			hold(b(ebiten.StandardGamepadButtonFrontBottomLeft), ebiten.KeyC)
			hold(b(ebiten.StandardGamepadButtonFrontBottomRight), ebiten.KeyJ)
			hold(b(ebiten.StandardGamepadButtonCenterLeft), ebiten.KeyM)
		case 'm':
			hold(b(ebiten.StandardGamepadButtonRightBottom), ebiten.KeySpace)
			hold(b(ebiten.StandardGamepadButtonRightRight), ebiten.KeyEscape)
		default:
			hold(b(ebiten.StandardGamepadButtonRightBottom), ebiten.KeyEnter, ebiten.KeySpace)
			hold(b(ebiten.StandardGamepadButtonRightRight), ebiten.KeyEscape)
			hold(b(ebiten.StandardGamepadButtonCenterLeft), ebiten.KeyM) // closes the map
		}
	}
}

// KeyJustPressed reports a key pressed this frame on the keyboard or a gamepad.
func KeyJustPressed(k ebiten.Key) bool {
	return inpututil.IsKeyJustPressed(k) || (padHeld[k] && !padPrev[k])
}

// KeyPressed reports a key held on the keyboard or a gamepad.
func KeyPressed(k ebiten.Key) bool { return ebiten.IsKeyPressed(k) || padHeld[k] }
