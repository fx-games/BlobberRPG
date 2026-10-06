//go:build !js

package main

import "os"

var saveFile = "blobber.sav"

func writeSave(b []byte) error  { return os.WriteFile(saveFile, b, 0o644) }
func readSave() ([]byte, error) { return os.ReadFile(saveFile) }
