package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/malashin/cpbftpchk/xftp"
)

var c xftp.IFtp
var err error

var regExpLine = regexp.MustCompile(`\?\{(.*)\?\}(.*)\?\|(\d+)\?\|(.*)$`)
var logFilePath = "shuher.log"
var fileListPath = "shuherFileList.txt"
var watcherRootPath = "/"
var ignoreFoldersMask = regexp.MustCompile(`^/AMEDIATEKA/PROMO$`)
var fileMask = regexp.MustCompile(`(?:\.mxf|\.mp4)$`)
var lastLine string
var longSleepTime = 30 * time.Minute
var shortSleepTime = 1 * time.Minute

func main() {
	// Get filepath to executable.
	bin, err := os.Executable()
	if err != nil {
		panic(err)
	}
	binPath := filepath.Dir(bin)

	// Create objects.
	fileWriter := NewFileWriter(filepath.Join(binPath, logFilePath))
	defer fileWriter.Close()
	mailWriter := NewMailWriter()
	logger := NewLogger()
	logger.AddLogger(LogLevelLeq(Debug), fileWriter)
	logger.AddLogger(Notice, mailWriter)
	logger.AddLogger(LogLevelLeq(Info), os.Stdout)
	ftpConn := NewFtpConn()
	ftpConn.SetLogger(logger)
	fileList := NewFileList()
	fileList.SetLogger(logger)
	// Load file list.
	fileList.Load(filepath.Join(binPath, fileListPath))
	// Properly close the connection on exit.
	defer ftpConn.Quit()

	for {
		// Initialize the connection to the specified ftp server address.
		ftpConn.DialAndLogin(ftpLogin)
		// Change directory to watcherRootPath.
		ftpConn.Cd(watcherRootPath)
		// Walk the directory tree.
		if ftpConn.GetError() == nil {
			logger.Log(Debug, "Looking for new files...")
			ftpConn.Walk(fileList.files)
			fmt.Print(Pad("", len(lastLine)) + "\r")
			// Terminate the FTP connection.
			ftpConn.Quit()
			if ftpConn.GetError() == nil {
				// Remove deleted files from the fileList.
				fileList.Clean()
				err := mailWriter.Send()
				if err != nil {
					logger.Log(Error, err)
				}
				// Save new fileList.
				fileList.Save(filepath.Join(binPath, fileListPath))
			}
		}
		if ftpConn.GetError() == nil {
			// Wait for sleepTime before checking again.
			time.Sleep(longSleepTime)
		} else {
			// Wait for sleepTime before checking again.
			time.Sleep(shortSleepTime)
		}
	}
}
