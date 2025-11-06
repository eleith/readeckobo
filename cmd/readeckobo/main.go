package main

import (
	"log"

	"readeckobo/internal/app"
	"readeckobo/internal/config"
	"readeckobo/internal/crypto"
	"readeckobo/internal/readeck"
	"readeckobo/internal/webserver"
)

func main() {
	cfg, err := config.Load("./config.yaml")
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	decryptedToken, err := crypto.DecryptAESECB(cfg.Readeck.AccessToken, cfg.Kobo.Serial)
	if err != nil {
		log.Fatalf("Error decrypting Readeck access token: %v", err)
	}

	// Create Readeck API client
	readeckClient, err := readeck.NewClient(cfg.Readeck.Host, decryptedToken)
	if err != nil {
		log.Fatalf("Error creating Readeck client: %v", err)
	}

	// Initialize application
	application := app.NewApp(
		app.WithConfig(cfg),
		app.WithReadeckClient(readeckClient),
	)

	// Initialize and start the web server
	webserver.ListenAndServe(cfg.Server.Port, application)

	// Keep the main goroutine alive
	select {}
}