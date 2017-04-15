package main

import (
	"fmt"

	"github.com/jlaffaye/ftp"
)

// Global variables.
// Ftp server address with port
var addr = ""
var user = ""
var password = ""

func main() {
	// Create client object with default config.
	client, err := ftp.Dial(addr)
	if err != nil {
		panic(err)
	}
	err = client.Login(user, password)
	if err != nil {
		panic(err)
	}
	entries, err := client.List("/")
	for _, element := range entries {
		fmt.Println(element.Name)
	}
}
