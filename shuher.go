package main

import (
	"fmt"

	"github.com/jlaffaye/ftp"
)

// Global variables.
// Ftp server address with port.
var addr = ""
var user = ""
var password = ""

func main() {
	// Create client object with default config.
	connection, err := ftp.Dial(addr)
	if err != nil {
		panic(err)
	}
	// Properly close the connection on exit.
	defer connection.Quit()
	// Authenticate the client with specified user and password.
	err = connection.Login(user, password)
	if err != nil {
		panic(err)
	}
	entries, err := connection.List("/")
	for _, element := range entries {
		fmt.Println(element.Name)
	}
}
