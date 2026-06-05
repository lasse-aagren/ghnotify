package tray

import _ "embed"

//go:embed icon.png
var idlePNG []byte

//go:embed active.png
var activePNG []byte

func iconBytes() []byte       { return idlePNG }
func iconActiveBytes() []byte { return activePNG }
