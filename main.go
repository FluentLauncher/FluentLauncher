package main

import (
	"log"

	"github.com/mistquay/fluentlauncher/internal/app"
)

func main() {
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
