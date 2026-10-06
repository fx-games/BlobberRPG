//go:build js

package main

import (
	"errors"
	"syscall/js"
)

const saveKey = "blobber.sav"

func writeSave(b []byte) error {
	js.Global().Get("localStorage").Call("setItem", saveKey, string(b))
	return nil
}

func readSave() ([]byte, error) {
	v := js.Global().Get("localStorage").Call("getItem", saveKey)
	if v.IsNull() {
		return nil, errors.New("no save")
	}
	return []byte(v.String()), nil
}
