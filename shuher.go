package main

import (
	"fmt"
	"time"

	"github.com/jlaffaye/ftp"
)

type entry struct {
	Name string
	Type entryType
	Size uint64
	Time time.Time
	Path string
}

type entryType int

const (
	entryTypeFile entryType = iota
	entryTypeFolder
	entryTypeLink
)

// Global variables.
// Ftp server address with port.
// var addr = ""
// var user = ""
// var password = ""

func parseFolder(connection *ftp.ServerConn, currentPath string) {
	entries, err := connection.List("")
	if err != nil {
		panic(err)
	}
	for _, element := range entries {
		if element.Type == 1 {
			fmt.Println(element)
		}
	}
}

func main() {
	// Initialize the connection to the specified ftp server address.
	connection, err := ftp.Dial(addr)
	if err != nil {
		panic(err)
	}
	fmt.Println("Connection to " + addr + " is initialized")
	// Properly close the connection on exit.
	defer connection.Quit()

	// Authenticate the client with specified user and password.
	err = connection.Login(user, password)
	if err != nil {
		panic(err)
	}
	fmt.Println("Logged in as " + user)
	var currentPath = "/"

	err = connection.ChangeDir("/AMEDIATEKA")
	if err != nil {
		panic(err)
	}
	currentPath = "/AMEDIATEKA"

	parseFolder(connection, currentPath)
}
