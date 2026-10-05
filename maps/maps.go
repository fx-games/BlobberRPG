//go:build !js

// Package maps opens the game's maps, event scripts, NPC registry and cutscenes:
// from the maps/ directory on disk, except in the browser build, which embeds them.
package maps

import (
	"io"
	"os"
)

// Open opens maps/<name> on disk.
func Open(name string) (io.ReadCloser, error) { return os.Open("maps/" + name) }
