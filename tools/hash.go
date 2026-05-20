// Одноразовая утилита: go run tools/hash.go YOUR_PASSWORD
package main

import (
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	pw := os.Args[1]
	if pw == "" {
		fmt.Println("usage: go run tools/hash.go YOUR_PASSWORD")
		os.Exit(1)
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(h))
}
