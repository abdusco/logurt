package main

import (
	"log"
	"os"
	"strconv"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	portNum, err := strconv.Atoi(port)
	if err != nil {
		log.Fatalln("PORT must be a number")
	}

	jwtLifetimeMinutes := os.Getenv("JWT_LIFETIME_MINUTES")
	if jwtLifetimeMinutes == "" {
		jwtLifetimeMinutes = "60"
	}
	jwtLifetimeMinutesNum, err := strconv.Atoi(jwtLifetimeMinutes)
	if err != nil {
		log.Fatalln("JWT_LIFETIME_MINUTES must be a number")
	}

	apiSecret := os.Getenv("API_SECRET")
	signingKey := os.Getenv("JWT_SIGNING_KEY")
	ingestionKey := os.Getenv("LOG_INGESTION_KEY")

	config := AppConfig{
		Port:               portNum,
		ApiSecret:          apiSecret,
		JwtSigningKey:      signingKey,
		LogIngestionKey:    ingestionKey,
		JwtLifetimeMinutes: jwtLifetimeMinutesNum,
	}

	log.Printf("Parsed config: %#v", config)

	app := NewApp(config)
	app.Run()
}
