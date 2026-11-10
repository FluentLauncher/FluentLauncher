package main

import (
	"log"

	"fluentlauncher/internal/app"
	"fluentlauncher/internal/bundled"

	// The catalog's languages are the app's; internal/app says the messages,
	// this keeps the registration visible from the main package too.
	_ "fluentlauncher/locales"
)

func main() {
	app.SetBrandIcon(bundled.Icon)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
