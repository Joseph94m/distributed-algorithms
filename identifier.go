package main

import (
	"math/rand"
	"os"
	"time"
)

func getUniqueIdentifier() (string, error) {
	hostname, err := getHostname()
	if err != nil {
		return "", err
	}
	return hostname + "_" + randomString(5), nil
}

func getHostname() (string, error) {
	return os.Hostname()
}

func randomString(n int) string {
	var letter = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	rand.Seed(time.Now().UnixNano())
	for i := range b {
		b[i] = letter[rand.Intn(len(letter))]
	}
	return string(b)
}
