package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"time"
)

func main() {
	config := parseConfig()
	log.Printf("Parsed config: %+v", config)

	app := NewApp(config)
	app.Run()
}

func parseConfig() AppConfig {
	args := flag.NewFlagSet("logurt", flag.ExitOnError)
	port := args.Int("port", envOrDefaultInt("PORT", 8080), "Port to listen on. (env:PORT)")
	apiSecret := args.String("api-secret", envOrDefault("API_SECRET", ""), "Secret for protecting API endpoints. (env:API_SECRET)")
	signingKey := args.String("jwt-signing-key", envOrDefault("JWT_SIGNING_KEY", ""), "JWT signing key. (env:JWT_SIGNING_KEY)")
	jwtLifetimeMinutes := args.Int("jwt-lifetime-minutes", envOrDefaultInt("JWT_LIFETIME_MINUTES", 60), "JWT token lifetime in minutes. (env:JWT_LIFETIME_MINUTES)")

	args.Parse(os.Args[1:])

	config := AppConfig{
		Port:               *port,
		ApiSecret:          *apiSecret,
		JwtSigningKey:      *signingKey,
		JwtLifetimeMinutes: *jwtLifetimeMinutes,
	}

	if config.ApiSecret == "" {
		log.Fatal("API secret is not set")
	}
	if config.JwtSigningKey == "" {
		log.Fatal("JWT signing key must be set")
	}

	return config
}

func randomString(length int) string {
	rand.Seed(time.Now().UnixNano())
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		log.Fatal(err)
	}
	return fmt.Sprintf("%x", b)
}

func envOrDefault(env string, defaultValue string) string {
	value, ok := os.LookupEnv(env)
	if !ok {
		return defaultValue
	}
	return value
}

func envOrDefaultInt(env string, defaultValue int) int {
	value, ok := os.LookupEnv(env)
	if !ok {
		return defaultValue
	}
	intValue, err := strconv.Atoi(value)
	if err != nil {
		log.Fatalf("Expected an integer for env var %s, got: %v", env, value)
	}
	return intValue
}
