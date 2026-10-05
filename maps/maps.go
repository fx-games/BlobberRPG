// Package maps embeds the game's maps, event scripts, NPC registry and cutscenes,
// so the game (including the browser build) needs no files beside it.
package maps

import "embed"

//go:embed *.map *.evt *.txt cutscenes
var FS embed.FS
