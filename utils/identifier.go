package utils

import (
	"math/rand"
	"os"
	"time"
)

func GetUniqueIdentifier() (string, error) {
	hostname, err := GetHostname()
	if err != nil {
		return "", err
	}
	return hostname + "_" + RandomString(5), nil
}

func GetHostname() (string, error) {
	return os.Hostname()
}

func RandomString(n int) string {
	var letter = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	rand.Seed(time.Now().UnixNano())
	for i := range b {
		b[i] = letter[rand.Intn(len(letter))]
	}
	return string(b)
}
