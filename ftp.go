package main

import (
	"fmt"
	"sort"

	"github.com/malashin/cpbftpchk/xftp"
)

type FtpConn struct {
	Loggerer
	conn      xftp.IFtp
	connected bool
}

func NewFtpConn() *FtpConn {
	return &FtpConn{}
}

func (f *FtpConn) Quit() {
	if !f.connected {
		return
	}
	f.conn.Quit()
	f.connected = false
	f.Log(Debug, "QUIT: Connection closed correctly")
}

func (f *FtpConn) DialAndLogin(addr string) {
	f.ResetError()
	conn, err := xftp.New(addr)
	if err != nil {
		f.Error(err)
		return
	}
	f.conn = conn
	f.Log(Debug, "DIAL_AND_LOGIN: Connected to"+addr)
}

func (f *FtpConn) Cwd() string {
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

func (f *FtpConn) Cd(path string) {
	if f.GetError() != nil {
		return
	}
	err := f.conn.ChangeDir(path)
	if err != nil {
		f.Error(err)
		return
	}
	f.Log(Debug, "CWD: "+f.Cwd())
}

func (f *FtpConn) CdUp() {
	if f.GetError() != nil {
		return
	}
	f.conn.ChangeDirToParent()
	f.Log(Debug, "CWD: "+f.Cwd())
}

func (f *FtpConn) Ls(path string) (entries []xftp.TEntry) {
	if f.GetError() != nil {
		return
	}
	entries, err := f.conn.List(path)
	if err != nil {
		f.Error(err)
		return
	}
	list := []string{}
	for _, file := range entries {
		if !(file.Name == "." || file.Name == "..") {
			list = append(list, file.Name)
		}
	}
	sort.Strings(list)
	f.Log(Debug, "LIST: ", list)
	return entries
}

func (f *FtpConn) Walk(fl map[string]FileEntry) {
	if f.GetError() != nil {
		return
	}
	entries := f.Ls("")
	cwd := f.Cwd()
	// Add "/" to cwd path
	if cwd != "/" {
		cwd = cwd + "/"
	}
	newLine := Pad(cwd, len(lastLine))
	fmt.Print(newLine + "\r")
	lastLine = cwd
	for _, element := range entries {
		switch element.Type {
		case xftp.File:
			if AcceptFileName(element.Name) {
				key := cwd + element.Name
				entry, fileExists := fl[key]
				if fileExists {
					if !entry.Time.Equal(element.Time) {
						// Old file with new date
						f.Log(Notice, "~ "+TruncPad(key, 64, 'l')+" datetime changed")
						fl[key] = NewFileEntry(element)
					} else if entry.Size != element.Size {
						// Old file with new size
						f.Log(Notice, "~ "+TruncPad(key, 64, 'l')+" size changed")
						fl[key] = NewFileEntry(element)
					} else {
						// Old file
						entry.Found = true
						fl[key] = entry
					}
				} else {
					// New file
					f.Log(Notice, "+ "+TruncPad(key, 64, 'l')+" new file")
					fl[key] = NewFileEntry(element)
				}
			}
		case xftp.Folder:
			if ignoreFoldersMask.MatchString(cwd + element.Name) {
				f.Log(Debug, "WALK: Ignoring folder \"", cwd+element.Name, "\"")
			} else if !(element.Name == "." || element.Name == "..") {
				f.Cd(element.Name)
				f.Walk(fl)
				f.CdUp()
			}
		}
	}
}
