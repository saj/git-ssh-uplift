package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
)

func getenvInt(name string) (int, error) {
	s := os.Getenv(name)
	if s == "" {
		return 0, fmt.Errorf("environment: missing required variable: %s", name)
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("environment: %s: %s", name, err)
	}
	return i, nil
}

func mustGetenvInt(name string) int {
	i, err := getenvInt(name)
	if err != nil {
		log.Fatal(err)
	}
	return i
}
