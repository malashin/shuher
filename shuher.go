package main

import (
	"bufio"
	"errors"
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
	"github.com/malashin/pochta"
)

type Loggerer struct {
	logger ILogger
	err    error
}

// SetLogger sets objects logger
func (l *Loggerer) SetLogger(logger ILogger) {
	l.logger = logger
}

// Log logs the input text
func (l *Loggerer) Log(loglevel int, text ...interface{}) {
	if l.logger != nil {
		l.logger.Log(loglevel, text...)
	}
}

func (l *Loggerer) Error(text ...interface{}) {
	if l.logger != nil {
		l.logger.Log(Error, "ERROR: "+fmt.Sprint(text...))
	}
	if l.err == nil {
		l.err = errors.New(fmt.Sprint(text...))
	}
}

func (l *Loggerer) ResetError() {
	l.err = nil
}

func (l *Loggerer) GetError() error {
	return l.err
}

type ftpConn struct {
	Loggerer
	conn      *ftp.ServerConn
	connected bool
}

func newFtpConn() *ftpConn {
	return &ftpConn{}
}

func (f *ftpConn) dial(addr string) {
	f.ResetError()
	conn, err := ftp.Dial(addr)
	if err != nil {
		f.Error(err)
		return
	}
	f.conn = conn
	f.connected = true
	f.Log(Debug, "Connected to "+addr)
}

func (f *ftpConn) quit() {
	if !f.connected {
		return
	}
	f.conn.Quit()
	f.connected = false
	f.Log(Debug, "Connection closed correctly")
}

func (f *ftpConn) login(user string, pass string) {
	if f.GetError() != nil {
		return
	}
	err := f.conn.Login(user, pass)
	if err != nil {
		f.Error(err)
		return
	}
	f.Log(Debug, "Logged in as "+user)
}

func (f *ftpConn) cwd() string {
	if f.GetError() != nil {
		return ""
	}
	cwd, err := f.conn.CurrentDir()
	if err != nil {
		f.Error(err)
		return ""
	}
	return cwd
}

func (f *ftpConn) cd(path string) {
	if f.GetError() != nil {
		return
	}
	err := f.conn.ChangeDir(path)
	if err != nil {
		f.Error(err)
		return
	}
}

func (f *ftpConn) cdup() {
	if f.GetError() != nil {
		return
	}
	f.conn.ChangeDirToParent()
}

func (f *ftpConn) ls(path string) (entries []*ftp.Entry) {
	if f.GetError() != nil {
		return
	}
	entries, err := f.conn.List(path)
	if err != nil {
		f.Error(err)
		return
	}
	return entries
}

func (f *ftpConn) walk(fl map[string]fileEntry) {
	if f.GetError() != nil {
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
					if !entry.Time.Equal(element.Time) {
						// Old file with new date
						f.Log(Notice, "~ "+truncPad(key, 40, 'l')+" datetime changed")
						fl[key] = newFileEntry(element)

					} else if entry.Size != element.Size {
						// Old file with new size
						f.Log(Notice, "~ "+truncPad(key, 40, 'l')+" size changed")
						fl[key] = newFileEntry(element)
					} else {
						// Old file
						entry.Found = true
						fl[key] = entry
					}
				} else {
					// New file
					f.Log(Notice, "+ "+truncPad(key, 40, 'l')+" new file")
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
			return "..." + s[len(s)-n+3:len(s)]
		}
		return s[len(s)-n : len(s)]
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
	Loggerer
	files map[string]fileEntry
}

func newFileList() *tFileList {
	return &tFileList{files: map[string]fileEntry{}}
}

func (fl *tFileList) pack() string {
	output := []string{}
	for key, value := range fl.files {
		output = append(output, "?{"+key+"?}"+value.pack()+"\n")
	}
	sort.Strings(output)
	return strings.Join(output, "")
}

// Mark all files in a filelist to not found
func (fl *tFileList) unfind() {
	for _, value := range fl.files {
		value.Found = false
	}
}

func (fl *tFileList) clean() {
	for key, value := range fl.files {
		if !value.Found {
			delete(fl.files, key)
			fl.Log(Info, "- "+truncPad(key, 40, 'l')+" deleted")
		} else {
			value.Found = false
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
	fl.Log(Debug, "Loading \""+filepath+"\"...")
	file, err := os.Open(filepath)
	if err != nil {
		fl.Log(Error, "\""+filepath+"\" not found")
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, entry := fl.parseLine(scanner.Text())
		if key != "" {
			fl.files[key] = entry
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
		fl.Log(Error, "Wrong input in file list ("+line+")")
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
		fl.Log(Error, err)
		return "", fileEntry{}
	}
	return key, entry
}

type ILogger interface {
	Log(loglevel int, text ...interface{})
}

type logger struct {
	writers []tWriter
}

type tWriter struct {
	writer   io.Writer
	loglevel int
}

func newLogger() *logger {
	return &logger{}
}

func (l *logger) addLogger(loglevel int, writer io.Writer) {
	l.writers = append(l.writers, tWriter{writer, loglevel})
}

func (l *logger) Log(loglevel int, text ...interface{}) {
	for _, writer := range l.writers {
		if loglevel&writer.loglevel != 0 {
			_, err := writer.writer.Write([]byte(time.Now().Format("2006-01-02 15:04:05") + " " + logLeveltoStr(loglevel) + ": " + fmt.Sprint(text...) + "\n"))
			if err != nil {
				fmt.Println(err)
			}
		}
	}
	if loglevel&Panic != 0 {
		panic(fmt.Sprint(text...))
	}
}

// LogLevel flags
const (
	Quiet = 0
	Panic = 1 << iota
	Error
	Warning
	Notice
	Info
	Debug
)

func logLevelLeq(loglevel int) int {
	return loglevel - 1 | loglevel
}

func logLeveltoStr(loglevel int) string {
	s := []string{}
	if loglevel&Panic != 0 {
		s = append(s, "PNC")
	}
	if loglevel&Error != 0 {
		s = append(s, "ERR")
	}
	if loglevel&Warning != 0 {
		s = append(s, "WRN")
	}
	if loglevel&Notice != 0 {
		s = append(s, "NTC")
	}
	if loglevel&Info != 0 {
		s = append(s, "INF")
	}
	if loglevel&Debug != 0 {
		s = append(s, "DBG")
	}
	return strings.Join(s, "|")
}

func newFileWriter(path string) *os.File {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0755)
	if err != nil {
		log.Panicln(err)
	}
	return file
}

type TMailWriter struct {
	msg []string
}

func NewMailWriter() *TMailWriter {
	return &TMailWriter{}
}

func (m *TMailWriter) Write(p []byte) (n int, err error) {
	m.msg = append(m.msg, string(p))
	return 0, nil
}

func (m *TMailWriter) Send() error {
	if len(m.msg) != 0 {
		body := strings.Join(m.msg, "")
		err := pochta.SendMail(smtpserver, auth, from, to, subject, body)
		if err != nil {
			return err
		}
		m.msg = []string{}
	}
	return nil
}

// Global variables are set in private file.
// Ftp server address with port.
// var addr = ""
// var user = ""
// var pass = ""

// Mail config
// var smtpserver = "" // with port
// var auth = pochta.LoginAuth("", "")
// var from = mail.Address{Name: "", Address: ""}
// var to = mail.Address{Name: "", Address: ""}
// var subject = ""

var regExpLine = regexp.MustCompile(`\?\{(.*)\?\}(.*)\?\|(\d+)\?\|(.*)$`)
var logFilePath = "shuher.log"
var fileListPath = "shuherFileList.txt"
var watcherRootPath = "/AMEDIATEKA"
var fileMask = regexp.MustCompile(`^.*\.mxf$`)
var lastLine string
var longSleepTime = 30 * time.Minute
var shortSleepTime = 1 * time.Minute

func main() {
	// Create objects.
	fileWriter := newFileWriter(logFilePath)
	defer fileWriter.Close()
	mailWriter := NewMailWriter()
	logger := newLogger()
	logger.addLogger(logLevelLeq(Debug), fileWriter)
	logger.addLogger(Notice, mailWriter)
	logger.addLogger(logLevelLeq(Info), os.Stdout)
	ftpConn := newFtpConn()
	ftpConn.SetLogger(logger)
	fileList := newFileList()
	fileList.SetLogger(logger)
	// Load file list.
	fileList.load(fileListPath)
	// Properly close the connection on exit.
	defer ftpConn.quit()

	for {
		// Initialize the connection to the specified ftp server address.
		ftpConn.dial(addr)
		// Authenticate the client with specified user and password.
		ftpConn.login(user, pass)
		// Change directory to watcherRootPath.
		ftpConn.cd(watcherRootPath)
		// Walk the directory tree.
		if ftpConn.GetError() == nil {
			logger.Log(Debug, "Looking for new files...")
			fileList.unfind()
			ftpConn.walk(fileList.files)
			fmt.Print(pad("", len(lastLine)) + "\r")
		}
		// Terminate the FTP connection.
		ftpConn.quit()
		// Remove deleted files from the fileList.
		fileList.clean()
		err := mailWriter.Send()
		if err != nil {
			logger.Log(Error, err)
		}
		if ftpConn.GetError() == nil {
			// Save new fileList.
			fileList.save(fileListPath)
			// Wait for sleepTime before checking again.
			time.Sleep(longSleepTime)
		} else {
			// Wait for sleepTime before checking again.
			time.Sleep(shortSleepTime)
		}
	}
}
