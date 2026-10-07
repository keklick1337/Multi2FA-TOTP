package assets

import (
	_ "embed"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

//go:embed icon.png
var iconPNG []byte

var Icon = &fyne.StaticResource{StaticName: "icon.png", StaticContent: iconPNG}

const lockSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path fill="#000" d="M12 1.5a5.5 5.5 0 0 0-5.5 5.5v3H5.75A1.75 1.75 0 0 0 4 11.75v9.5C4 22.22 4.78 23 5.75 23h12.5c.97 0 1.75-.78 1.75-1.75v-9.5c0-.97-.78-1.75-1.75-1.75H17.5V7A5.5 5.5 0 0 0 12 1.5Zm-3.5 5.5a3.5 3.5 0 1 1 7 0v3h-7V7Zm3.5 7a1.75 1.75 0 0 1 1 3.19V19a1 1 0 1 1-2 0v-1.81A1.75 1.75 0 0 1 12 14Z"/></svg>`

// LockIcon is a padlock that follows the theme foreground color.
var LockIcon = theme.NewThemedResource(&fyne.StaticResource{StaticName: "lock.svg", StaticContent: []byte(lockSVG)})
