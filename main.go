package main

import (
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"strconv"
	"sync"
	"time"
	"os"
)

// Define the number of chunks to download by default
const defaultNumChunks = 10

// confirmSupportAndFileChunkSize tests to see if "Accept-Ranges" is part of the HTTP Response header
// If HTTP Range requests are not supported, return server not supported error
// If supported, return the filesize and anticipated chunkSize
func confirmSupportAndFileChunkSize(dwLink string) (int64, int64, error) {
	// Set DisableCompression to true (default is false) 
	// This ensures Go's internal transport behavior does not mess with our logic
	tr := &http.Transport{
		DisableCompression: true,
	}
	client := &http.Client{Transport: tr}
	response, err := client.Get(dwLink)
	if err != nil {
    	log.Fatalln(err)
		return 0, 0, errors.New("HTTP error: GET request failed")
	}
	acceptRanges := response.Header["Accept-Ranges"]
	if acceptRanges[0] == "none" {
		return 0, 0, errors.New("Server Error: Accept-Ranges Header does not exist in HTTP Response")
	}
	filesize, err := strconv.ParseInt(response.Header["Content-Length"][0], 10, 64)
	return filesize, (filesize/defaultNumChunks), err
}

// getDownloadFileName returns the filename of the file hosted at the URL to download
func getDownloadFileName(dwLink string) string {
	filename, err := url.Parse(dwLink)
	if err != nil {
		log.Fatalln(err)
	}
	urlParts := strings.Split(filename.Path, "/")
	filePart := strings.Split(urlParts[len(urlParts)-1], "?")
	return filePart[0]
}

// getObjectRange obtains the range of bytes from rangeStart to rangeEnd from the server using the Range HTTP request header
// returns the HTTP response
func getObjectRange(dwLink string, rangeStart int64, rangeEnd int64) (http.Response, error) {
	// Set DisableCompression manually to true, same reason as in confirmSupportAndFileChunkSize
	tr := &http.Transport{
		DisableCompression: true,
	}
	client := &http.Client{Transport: tr}
	craftRequest, err := http.NewRequest("GET", dwLink, nil)
	if err != nil {
		return http.Response{}, err
	}
	craftRequest.Header.Add("Range", fmt.Sprintf("bytes=%d-%d", rangeStart, rangeEnd))
	response, err := client.Do(craftRequest)
	if err != nil {
		return http.Response{}, err
	}
	return *response, err
}

// writeChunks writes the obtained object to the right position in the file
func writeChunks(response http.Response, fileToWrite *os.File, currChunk int64, rangeStart int64, downloaderWg *sync.WaitGroup) {
	var writeRangeStart = rangeStart
	// Obtain size of response to compare the bytes read from the object
	responseSize, _ := strconv.ParseInt(response.Header["Content-Length"][0], 10, 64)

	obj := response.Body
	defer obj.Close()
	defer downloaderWg.Done()
	
	// make a temporary buffer to read chunks from the response
	buff := make([]byte, 8*1024)
	for {
		bytesRead, readErr := obj.Read(buff)
		if bytesRead > 0 {
			bytesWritten, writeErr := fileToWrite.WriteAt(buff[0:bytesRead], writeRangeStart)
			writeRangeStart += int64(bytesWritten)
			if writeErr != nil {
				log.Fatalf("Error: %s, at chunk: %d.\n", writeErr.Error(), currChunk)
			}
			if bytesRead != bytesWritten {
				log.Fatalln("Error occurred during writing, bytes read and bytes written do not match. At chunk: ", currChunk)
			}
		}
		if readErr != nil && readErr.Error() == "EOF" {
			if responseSize == (writeRangeStart-rangeStart) {
				fmt.Println("Downloaded chunk ", currChunk+1, " successfully!")
			} else {
				log.Fatalf("Error during READ, but reached EOF : %s\n", readErr.Error())
			}
			break
		} else if readErr != nil {
			log.Fatalf("Error during READ: %s, in chunk: %d.\n", readErr.Error(), currChunk)
		}
	}
}

// isFlagPassed checks if the input flag string was passed explicitly by user
func isFlagPassed(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func main() {
	// Get URL to download and desired output file name
	var resultFile, dwLink string
	// SHA256 Checksum for https://go.dev/dl/go1.20.3.linux-amd64.tar.gz file from https://go.dev/dl/ is 979694c2c25c735755bf26f4f45e19e64e4811d661dd07b8c010f7a8e18adfca (4/5/23)
	flag.StringVar(&dwLink, "url", "https://go.dev/dl/go1.20.3.linux-amd64.tar.gz", "URL of the file to download (default: latest go release for linux as of 4/5/23)")
	flag.StringVar(&resultFile, "output", "", "Path and filename to save output file (default: current directory with filename obtained through the URL)")
	flag.Parse()

	// Check hosting server's support for HTTP Range requests, if yes, get fileSize and anticipated chunkSize
	fileSize, chunkSize, err := confirmSupportAndFileChunkSize(dwLink)
	if err != nil {
		log.Fatalln("Fatal error in checking support for multi-source downloads: ", err)
	}

	if !isFlagPassed("output") {
		resultFile = getDownloadFileName(dwLink)
		if resultFile == "" {
			log.Fatalln("Bad Input: No object to download")
		}
	}
	file, err := os.OpenFile(resultFile, os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatalln(err)
	}

	var rangeStart, rangeEnd int64
	var downloaderWg sync.WaitGroup
	startTime := time.Now()
	fmt.Println("Downloading ", resultFile, " in ", defaultNumChunks, " chunks...")
	for i := int64(0); i < defaultNumChunks; i++ {
		if i == defaultNumChunks-1 {
			// For the last chunk, ensure rangeEnd is up to fileSize
			rangeEnd = fileSize 
		} else {
			// rangeStart is 0 indexed, so rangeEnd is adjusted
			rangeEnd = rangeStart + chunkSize - 1 
		}
		downloaderWg.Add(1)
		go func(i int64, dwLink string, rangeStart int64, rangeEnd int64, file *os.File, downloaderWg *sync.WaitGroup) {
			response, err := getObjectRange(dwLink, rangeStart, rangeEnd)
			if err != nil {
				log.Fatalf("Request error in chunk: %d, Error: %s\n", i, err.Error())
			}
			writeChunks(response, file, i, rangeStart, downloaderWg)
		}(i, dwLink, rangeStart, rangeEnd, file, &downloaderWg)
		rangeStart =  rangeEnd + 1
	}
	downloaderWg.Wait()
	elapsed := time.Since(startTime)
	fmt.Println("Time to download was: ", elapsed)
	file.Close()
	writtenFile, err := os.OpenFile(resultFile, os.O_RDONLY, 0666)
	h := sha256.New()
	if _, err := io.Copy(h, writtenFile); err != nil {
		log.Fatal("Error while calculating SHA256 checksum: ", err)
	}
	fmt.Printf("SHA256 Checksum: %x\n", h.Sum(nil))

}
