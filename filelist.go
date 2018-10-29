package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/malashin/cpbftpchk/xftp"
)

func Pad(s string, n int) string {
	if n > len(s) {
		return s + strings.Repeat(" ", n-len(s))
	}
	return s
}

func TruncPad(s string, n int, side byte) string {
	if len(s) > n {
		if n >= 1 {
			return "…" + s[len(s)-n+1:len(s)]
		}
		return s[len(s)-n : len(s)]
	}
	if side == 'r' {
		return strings.Repeat(" ", n-len(s)) + s
	}
	return s + strings.Repeat(" ", n-len(s))
}

func AcceptFileName(fileName string) bool {
	if fileMask.MatchString(fileName) {
		return true
	}
	return false
}

func NewFileEntry(entry xftp.TEntry) FileEntry {
	file := FileEntry{}
	file.Name = entry.Name
	file.Size = entry.Size
	file.Time = entry.Time
	file.Found = true
	return file
}

type FileEntry struct {
	Name  string
	Size  int64
	Time  time.Time
	Found bool
}

func (fe *FileEntry) Pack() (string, error) {
	time, err := fe.Time.MarshalText()
	if err != nil {
		return "", err
	}
	return fe.Name + "?|" + fmt.Sprintf("%v", fe.Size) + "?|" + string(time), nil
}

type TFileList struct {
	Loggerer
	files map[string]FileEntry
}

func NewFileList() *TFileList {
	return &TFileList{files: map[string]FileEntry{}}
}

func (fl *TFileList) Pack() (string, error) {
	output := []string{}
	for key, value := range fl.files {
		valueString, err := value.Pack()
		if err != nil {
			return "", err
		}
		output = append(output, "?{"+key+"?}"+valueString+"\n")
	}
	sort.Strings(output)
	return strings.Join(output, ""), nil
}

func (fl *TFileList) Clean() {
	for key, value := range fl.files {
		if !value.Found {
			delete(fl.files, key)
			fl.Log(Info, "- "+TruncPad(key, 64, 'l')+" deleted")
		} else {
			value.Found = false
			fl.files[key] = value
		}
	}
}

func (fl TFileList) String() (string, error) {
	return fl.Pack()
}

func (fl *TFileList) Load(filepath string) {
	fl.Log(Debug, "Loading \""+filepath+"\"...")
	file, err := os.Open(filepath)
	if err != nil {
		fl.Log(Error, "\""+filepath+"\" not found")
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, entry := fl.ParseLine(scanner.Text())
		if key != "" {
			fl.files[key] = entry
		}
	}

	err = scanner.Err()
	if err != nil {
		log.Panicln(err)
	}
	fl.Log(Debug, "FileList loaded. ", fl.files)
}

func (fl *TFileList) Save(filepath string) {
	file, err := os.Create(filepath)
	if err != nil {
		log.Panicln(err)
	}
	defer file.Close()

	fileListValue, err := fl.Pack()
	if err != nil {
		log.Panicln(err)
	}
	_, err = io.Copy(file, strings.NewReader(fileListValue))
	if err != nil {
		log.Panicln(err)
	}
	list := []string{}
	for key := range fl.files {
		list = append(list, key)
	}
	sort.Strings(list)
	fl.Log(Debug, "FileList saved. ", list)
}

func (fl *TFileList) ParseLine(line string) (string, FileEntry) {
	// "?{/AMEDIATEKA/ANIMALS_2/SER_05620.mxf?}SER_05620.mxf?|13114515508?|2017-03-17 14:39:39 +0000 UTC"
	if !regExpLine.MatchString(line) {
		fl.Log(Error, "Wrong input in file list ("+line+")")
		return "", FileEntry{}
	}
	matches := regExpLine.FindStringSubmatch(line)
	key := matches[1]
	entry := FileEntry{}
	entry.Name = matches[2]
	entrySize, err := strconv.ParseInt(matches[3], 0, 64)
	entry.Size = int64(entrySize)
	err = entry.Time.UnmarshalText([]byte(matches[4]))
	if err != nil {
		fl.Log(Error, "Wrong input in file list ("+line+")")
		return "", FileEntry{}
	}
	if err != nil {
		fl.Log(Error, err)
		return "", FileEntry{}
	}
	return key, entry
}
