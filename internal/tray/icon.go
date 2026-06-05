package tray

import _ "embed"

//go:embed icon_idle.png
var idlePNG []byte

//go:embed icon_active.png
var activePNG []byte

func iconBytes() []byte       { return idlePNG }
func iconActiveBytes() []byte { return activePNG }
