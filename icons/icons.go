package icons

import (
	_ "embed"
	"fyne.io/fyne/v2"
)

//go:embed Icon.png
var appIcon []byte

func AppIcon() *fyne.StaticResource {
	return fyne.NewStaticResource("AppIcon", appIcon)
}
