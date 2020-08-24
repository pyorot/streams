package utils

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type utilsEnv struct{}

// Env : utility functions for environment variables
var Env utilsEnv

// Load environment variables from .env file if existent (else assume pre-loaded)
func (utilsEnv) Load() {
	err := godotenv.Load()
	if err == nil {
		Log.Insta <- ". | env vars loaded from .env"
	} else if os.IsNotExist(err) {
		Log.Insta <- ". | env vars pre-loaded"
	} else {
		panic(err)
	}
}

// Get mandatory environment variable (else exit)
func (utilsEnv) GetOrExit(key string) string {
	val, exists := os.LookupEnv(key)
	if !exists {
		panic(fmt.Sprintf("Missing env var: %s", key))
	}
	return val
}

// Get optional environment variable (else return "")
func (utilsEnv) GetOrEmpty(key string) string {
	val, _ := os.LookupEnv(key)
	return val
}
