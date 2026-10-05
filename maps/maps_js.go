//go:build js

package maps

import (
	"embed"
	"io"
)

//go:embed *.map *.evt *.txt cutscenes
var embedded embed.FS

// Open opens the copy of maps/<name> built into the browser version.
func Open(name string) (io.ReadCloser, error) { return embedded.Open(name) }
