package main

import (
	"fmt"
	"os"
)

// ternary if macro
func ifThenElse(cond bool, valueIfTrue interface{}, valueIfFalse interface{}) interface{} {
	if cond {
		return valueIfTrue
	}
	return valueIfFalse
}

// fatal error macro, used in initialisations
func exitIfError(err error) {
	if err != nil {
		panic(err)
	}
}

// get mandatory environment variable (else exit)
func getEnvOrExit(key string) string {
	val, exists := os.LookupEnv(key)
	if !exists {
		panic(fmt.Sprintf("Missing env var: %s", key))
	}
	return val
}

// get optional environment variable (else return "")
func getEnvOrEmpty(key string) string {
	val, _ := os.LookupEnv(key)
	return val
}
