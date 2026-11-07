package main

import (
	_ "embed"
	"log"

	"github.com/mistquay/fluentlauncher/internal/app"
)

//go:embed resources/icon.png
var brandIcon []byte

func main() {
	app.SetBrandIcon(brandIcon)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
