package main

import (
	_ "embed"
	"log"

	"fluentlauncher/internal/app"

	// The catalog's languages are the app's; internal/app says the messages,
	// this keeps the registration visible from the main package too.
	_ "fluentlauncher/locales"
)

//go:embed resources/icon.png
var brandIcon []byte

func main() {
	app.SetBrandIcon(brandIcon)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
