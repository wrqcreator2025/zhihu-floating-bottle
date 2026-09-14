// dev-token is an explicit local development tool, never an HTTP login endpoint.
package main

import (
	"fmt"
	"os"
	"time"

	"driftbottle/internal/auth"
)

func main() {
	if os.Getenv("APP_ENV") == "production" || len(os.Args) != 2 || len(os.Getenv("AUTH_SIGNING_KEY")) < 32 {
		fmt.Fprintln(os.Stderr, "development only: set AUTH_SIGNING_KEY (32+ bytes), then go run ./cmd/dev-token <subject>")
		os.Exit(1)
	}
	issuer, audience := os.Getenv("AUTH_ISSUER"), os.Getenv("AUTH_AUDIENCE")
	if issuer == "" {
		issuer = "drift-bottle"
	}
	if audience == "" {
		audience = "drift-bottle-web"
	}
	a := auth.Auth{Key: []byte(os.Getenv("AUTH_SIGNING_KEY")), Issuer: issuer, Audience: audience}
	token, err := a.Issue(os.Args[1], 24*time.Hour)
	if err != nil {
		os.Exit(1)
	}
	fmt.Println(token)
}
