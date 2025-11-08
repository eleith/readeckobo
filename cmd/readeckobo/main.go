package main

import (
	"log"

	"readeckobo/internal/app"
	"readeckobo/internal/config"
	"readeckobo/internal/logger"
	"readeckobo/internal/webserver"
)

func main() {
	cfg, err := config.Load("./config.yaml")
	if err != nil {
		log.Fatalf("Error loading configuration: %v", err)
	}

	logLevel, err := logger.ParseLevel(cfg.LogLevel)
	if err != nil {
		log.Fatalf("Error parsing log level: %v", err)
	}
	appLogger := logger.New(logLevel)

	// Initialize application
	application := app.NewApp(
		app.WithConfig(cfg),
		app.WithLogger(appLogger),
	)

	// Initialize and start the web server
	webserver.ListenAndServe(cfg.Server.Port, application, appLogger)

	// Keep the main goroutine alive
	select {}
}