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

type ftpConn struct {
	conn      *ftp.ServerConn
	logger    ILogger
	connected bool
}

func newFtpConn() *ftpConn {
	return &ftpConn{}
}

func (f *ftpConn) setLogger(l ILogger) {
	f.logger = l
}

func (f *ftpConn) Log(text ...interface{}) {
	if f.logger != nil {
		f.logger.Log(text...)
	}
}

func (f *ftpConn) dial(addr string) {
	conn, err := ftp.Dial(addr)
	if err != nil {
		log.Panicln(err)
	}
	f.conn = conn
	f.connected = true
	f.Log("Connected to " + addr)
}

func (f *ftpConn) quit() {
	if !f.connected {
		return
	}
	f.conn.Quit()
	f.connected = false
	f.Log("Connection closed")
}

func (f *ftpConn) login(user string, pass string) {
	if !f.connected {
		f.Log("ERROR: ftpConn.login() No FTP connection")
		return
	}
	err := f.conn.Login(user, pass)
	if err != nil {
		log.Panicln(err)
	}
	f.Log("Logged in as " + user)
}

func (f *ftpConn) cwd() string {
	if !f.connected {
		f.Log("ERROR: ftpConn.cwd() No FTP connection")
		return ""
	}
	cwd, err := f.conn.CurrentDir()
	if err != nil {
		log.Panicln(err)
	}
	return cwd
}

func (f *ftpConn) cd(path string) {
	if !f.connected {
		f.Log("ERROR: ftpConn.cd() No FTP connection")
		return
	}
	err := f.conn.ChangeDir(path)
	if err != nil {
		log.Panicln(err)
	}
}

func (f *ftpConn) cdup() {
	if !f.connected {
		f.Log("ERROR: ftpConn.cdup() No FTP connection")
		return
	}
	f.conn.ChangeDirToParent()
}

func (f *ftpConn) ls(path string) (entries []*ftp.Entry) {
	if !f.connected {
		f.Log("ERROR: ftpConn.ls() No FTP connection")
		return
	}
	entries, err := f.conn.List(path)
	if err != nil {
		log.Panicln(err)
	}
	return entries
}

func (f *ftpConn) walk(fl map[string]fileEntry) {
	if !f.connected {
		f.Log("ERROR: ftpConn.walk() No FTP connection")
		return
	}
	entries := f.ls("")
	cwd := f.cwd()
	newLine := pad(cwd, len(lastLine))
	fmt.Print(newLine + "\r")
	lastLine = cwd
	for _, element := range entries {
		switch element.Type {
		case ftp.EntryTypeFile:
			if acceptFileName(element.Name) {
				key := cwd + "/" + element.Name
				entry, fileExists := fl[key]
				if fileExists {
					// Old file with new date
					if !entry.Time.Equal(element.Time) {
						f.Log("~ " + truncPad(key, 40, 'l') + " datetime changed")
						fl[key] = newFileEntry(element)
					} else if entry.Size != element.Size {
						f.Log("~ " + truncPad(key, 40, 'l') + " size changed")
						fl[key] = newFileEntry(element)
					} else {
						entry.Found = true
						fl[key] = entry
					}
				} else {
					// New file
					f.Log("+ " + truncPad(key, 40, 'l') + " new file")
					fl[key] = newFileEntry(element)
				}
			}
		case ftp.EntryTypeFolder:
			f.cd(element.Name)
			f.walk(fl)
			f.cdup()
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
	file.Size = entry.Size
	file.Time = entry.Time
	file.Found = true
	return file
}

type fileEntry struct {
	Name  string
	Size  uint64
	Time  time.Time
	Found bool
}

func (fe *fileEntry) pack() string {
	return fe.Name + "?|" + fmt.Sprintf("%v", fe.Size) + "?|" + fe.Time.String()
}

type tFileList struct {
	file   map[string]fileEntry
	logger ILogger
}

func newFileList() *tFileList {
	return &tFileList{file: map[string]fileEntry{}}
}

func (fl *tFileList) setLogger(l ILogger) {
	fl.logger = l
}

func (fl *tFileList) Log(text ...interface{}) {
	if fl.logger != nil {
		fl.logger.Log(text...)
	}
}

func (fl *tFileList) pack() string {
	output := []string{}
	for key, value := range fl.file {
		output = append(output, "?{"+key+"?}"+value.pack()+"\n")
	}
	sort.Strings(output)
	return strings.Join(output, "")
}

func (fl *tFileList) clean() {
	for key, value := range fl.file {
		if !value.Found {
			delete(fl.file, key)
			fl.Log("- " + truncPad(key, 40, 'l') + " deleted")
		}
	}
}

func (fl tFileList) String() string {
	return fl.pack()
}

func (fl *tFileList) save(filepath string) {
	file, err := os.Create(filepath)
	if err != nil {
		log.Panicln(err)
	}
	defer file.Close()

	_, err = io.Copy(file, strings.NewReader(fl.pack()))
	if err != nil {
		log.Panicln(err)
	}
}

func (fl *tFileList) load(filepath string) {
	fl.Log("Loading \"" + filepath + "\"...")
	file, err := os.Open(filepath)
	if err != nil {
		fl.Log("\"" + filepath + "\" not found")
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, entry := fl.parseLine(scanner.Text())
		if key != "" {
			fl.file[key] = entry
		}
	}

	err = scanner.Err()
	if err != nil {
		log.Panicln(err)
	}
}

func (fl *tFileList) parseLine(line string) (string, fileEntry) {
	// "?{/AMEDIATEKA/ANIMALS_2/SER_05620.mxf?}SER_05620.mxf?|13114515508?|2017-03-17 14:39:39 +0000 UTC"
	if !regExpLine.MatchString(line) {
		fl.Log("ERROR: Wrong input in file list (" + line + ")")
		return "", fileEntry{}
	}
	matches := regExpLine.FindStringSubmatch(line)
	key := matches[1]
	entry := fileEntry{}
	entry.Name = matches[2]
	entrySize, err := strconv.Atoi(matches[3])
	entry.Size = uint64(entrySize)
	entry.Time, err = time.Parse("2006-01-02 15:04:05 +0000 UTC", matches[4])
	if err != nil {
		fl.Log(err)
		return "", fileEntry{}
	}
	return key, entry
}

// ILogger ...
type ILogger interface {
	Log(text ...interface{})
}

type logger struct {
	file   *os.File
	logger *log.Logger
}

func newLogger() *logger {
	l := &logger{}
	file, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		log.Panicln(err)
	}
	l.file = file
	l.logger = log.New(file, "", log.LstdFlags)
	return l
}

func (l *logger) close() {
	l.file.Close()
}

func (l *logger) Log(text ...interface{}) {
	log.Println(text...)
	l.logger.Print(text...)
}

// Global variables are set in private file.
// Ftp server address with port.
// var addr = ""
// var user = ""
// var pass = ""

var regExpLine = regexp.MustCompile(`\?\{(.*)\?\}(.*)\?\|(\d+)\?\|(.*)$`)
var logFilePath = "shuher.log"
var fileListPath = "shuherFileList.txt"
var watcherRootPath = "/AMEDIATEKA"
var fileMask = regexp.MustCompile(`^.*\.mxf$`)
var lastLine string
var sleepTime = 10 * time.Minute

func main() {
	// Create objects.
	logger := newLogger()
	defer logger.close()
	ftpConn := newFtpConn()
	ftpConn.setLogger(logger)
	fileList := newFileList()
	fileList.setLogger(logger)
	// Load file list.
	fileList.load(fileListPath)
	// Properly close the connection on exit.
	defer ftpConn.quit()

	for {
		// Initialize the connection to the specified ftp server address.
		ftpConn.dial(addr)
		// Authenticate the client with specified user and password.
		ftpConn.login(user, pass)
		// Change directory watcherRootPath.
		ftpConn.cd(watcherRootPath)
		// Walk the directory tree.
		logger.Log("Looking for new files...")
		ftpConn.walk(fileList.file)
		// Remove deleted files from the fileList.
		fileList.clean()
		// Save new fileList.
		fileList.save(fileListPath)
		// Terminate the FTP connection.
		ftpConn.quit()
		// Wait for sleepTime before checking again.
		time.Sleep(sleepTime)
	}
}
