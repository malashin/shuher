package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jlaffaye/ftp"
)

func dial(addr string) *ftp.ServerConn {
	conn, err := ftp.Dial(addr)
	if err != nil {
		log.Fatalln(err)
	}
	consoleLog("Connection to " + addr + " initialized")
	return conn
}

func quit(conn *ftp.ServerConn) {
	conn.Quit()
	consoleLog("Connection closed")
}

func login(conn *ftp.ServerConn, user string, pass string) {
	err := conn.Login(user, pass)
	if err != nil {
		log.Fatalln(err)
	}
	consoleLog("Logged in as " + user)
}

func cwd(conn *ftp.ServerConn) string {
	cwd, err := conn.CurrentDir()
	if err != nil {
		log.Fatalln(err)
	}
	return cwd
}

func cd(conn *ftp.ServerConn, path string) {
	err := conn.ChangeDir(path)
	if err != nil {
		log.Fatalln(err)
	}
}

func cdup(conn *ftp.ServerConn) {
	conn.ChangeDirToParent()
}

func ls(conn *ftp.ServerConn, path string) (entries []*ftp.Entry) {
	entries, err := conn.List(path)
	if err != nil {
		log.Fatalln(err)
	}
	return entries
}

func walk(conn *ftp.ServerConn) {
	entries := ls(conn, "")
	cwd := cwd(conn)
	newLine := pad(cwd, len(lastLine))
	fmt.Print(newLine + "\r")
	lastLine = cwd
	for _, element := range entries {
		switch element.Type {
		case ftp.EntryTypeFile:
			if acceptFileName(element.Name) {
				key := cwd + "/" + element.Name
				entry, fileExists := fileList[key]
				if fileExists {
					// Old file with new date
					if !entry.Time.Equal(element.Time) {
						consoleLog("~ " + truncPad(key, 40, 'l') + " datetime changed")
						fileList[key] = newFileEntry(element)
					} else if entry.Size != element.Size {
						consoleLog("~ " + truncPad(key, 40, 'l') + " size changed")
						fileList[key] = newFileEntry(element)
					} else {
						entry.Found = true
						fileList[key] = entry
					}
				} else {
					// New file
					consoleLog("+ " + truncPad(key, 40, 'l') + " new file")
					fileList[key] = newFileEntry(element)
				}
			}
		case ftp.EntryTypeFolder:
			cd(conn, element.Name)
			walk(conn)
			cdup(conn)
		}
	}
}

func pad(s string, n int) string {
	if n > len(s) {
		return s + strings.Repeat(" ", n-len(s))
	}
	return s
}

func truncPad(s string, n int, side byte) string {
	if len(s) > n {
		if n >= 3 {
			return "..." + s[0+n:len(s)]
		}
		return s[0:n]
	}
	if side == 'r' {
		return strings.Repeat(" ", n-len(s)) + s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func acceptFileName(fileName string) bool {
	if fileMask.MatchString(fileName) {
		return true
	}
	return false
}

func newFileEntry(entry *ftp.Entry) fileEntry {
	file := fileEntry{}
	file.Name = entry.Name
	file.Type = entryType(entry.Type)
	file.Size = entry.Size
	file.Time = entry.Time
	file.Found = true
	return file
}

type fileEntry struct {
	Name  string
	Type  entryType
	Size  uint64
	Time  time.Time
	Found bool
}

func (fe *fileEntry) pack() string {
	return fe.Name + "?|" + fmt.Sprintf("%v", fe.Type) + "?|" + fmt.Sprintf("%v", fe.Size) + "?|" + fe.Time.String()
}

type entryType int

const (
	entryTypeFile entryType = iota
	entryTypeFolder
	entryTypeLink
)

type tFileList map[string]fileEntry

func (fl *tFileList) pack() string {
	output := []string{}
	for key, value := range *fl {
		output = append(output, "?{"+key+"?}"+value.pack()+"\n")
	}
	sort.Strings(output)
	return strings.Join(output, "")
}

func (fl *tFileList) clean() {
	for key, value := range *fl {
		if !value.Found {
			delete(*fl, key)
			consoleLog("- " + truncPad(key, 40, 'l') + " deleted")
		}
	}
}

func (fl tFileList) String() string {
	return fl.pack()
}

func (fl *tFileList) save(filepath string) {
	file, err := os.Create(filepath)
	if err != nil {
		log.Fatalln(err)
	}
	defer file.Close()

	_, err = io.Copy(file, strings.NewReader(fl.pack()))
	if err != nil {
		log.Fatalln(err)
	}
}

func (fl *tFileList) load(filepath string) {
	consoleLog("Loading \"" + filepath + "\"...")
	file, err := os.Open(filepath)
	if err != nil {
		consoleLog("\"" + filepath + "\" not found")
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, entry := parseLine(scanner.Text())
		if key != "" {
			fileList[key] = entry
		}
	}

	err = scanner.Err()
	if err != nil {
		log.Fatalln(err)
	}
}

func createLog() (*os.File, *log.Logger) {
	file, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		log.Fatalln(err)
	}
	return file, log.New(file, "", log.LstdFlags)
}

func closeLog(file *os.File) {
	defer file.Close()
}

func consoleLog(text ...interface{}) {
	log.Println(text...)
	logger.Print(text...)
}

func parseLine(line string) (string, fileEntry) {
	// "?{/AMEDIATEKA/ANIMALS_2/SER_05620.mxf?}SER_05620.mxf?|0?|13114515508?|2017-03-17 14:39:39 +0000 UTC"
	if !regExpLine.MatchString(line) {
		consoleLog("ERROR: Wrong input in file list (" + line + ")")
		return "", fileEntry{}
	}
	matches := regExpLine.FindStringSubmatch(line)
	key := matches[1]
	entry := fileEntry{}
	entry.Name = matches[2]
	entryFileType, err := strconv.Atoi(matches[3])
	if err != nil {
		consoleLog(err)
		return "", fileEntry{}
	}
	entry.Type = entryType(entryFileType)
	entrySize, err := strconv.Atoi(matches[4])
	entry.Size = uint64(entrySize)
	entry.Time, err = time.Parse("2006-01-02 15:04:05 +0000 UTC", matches[5])
	if err != nil {
		consoleLog(err)
		return "", fileEntry{}
	}
	return key, entry
}

// Global variables are set in private file.
// Ftp server address with port.
// var addr = ""
// var user = ""
// var pass = ""

var fileList = tFileList{}
var regExpLine = regexp.MustCompile(`\?\{(.*)\?\}(.*)\?\|(\d)\?\|(\d+)\?\|(.*)$`)
var logFilePath = "shuher.log"
var watcherRootPath = "/AMEDIATEKA"
var fileMask = regexp.MustCompile(`^.*\.mxf$`)
var logger *log.Logger
var lastLine string
var sleepTime = 10 * time.Minute

func main() {
	// Open log file.
	var logFile *os.File
	logFile, logger = createLog()
	defer closeLog(logFile)
	// Load file list.
	fileList.load("shuherFileList.txt")

	for {
		// Initialize the connection to the specified ftp server address.
		conn := dial(addr)
		// Properly close the connection on exit.
		defer quit(conn)
		// Authenticate the client with specified user and password.
		login(conn, user, pass)
		// Change directory watcherRootPath.
		cd(conn, watcherRootPath)
		// Walk the directory tree.
		consoleLog("Looking for new files...")
		walk(conn)
		// Remove deleted files from the fileList.
		fileList.clean()
		// Save new fileList.
		fileList.save("shuherFileList.txt")
		// Terminate the FTP connection.
		quit(conn)
		// Wait for sleepTime before checking again.
		time.Sleep(sleepTime)
	}
}
