# Multi-source-downloader implemented in Golang

This program helps you download files in multiple chunks to help parallelize the download. This only works if the server supports partial requests from the client for file downloads. Typically, servers advertise this using the `Accept-Ranges` HTTP response header. Clients can use the `Range` HTTP request header to indicate what part of the object it wishes to fetch.
The code checks for such support, and then follows up with requests for multiple different chunks in parallel and rearranges them locally to reconstitute the file.

## Build
Build the program using `go build main.go`

## Run 
By default, the program downloads the recent golang binary release for linux (`https://go.dev/dl/go1.20.3.linux-amd64.tar.gz`).
But it can take a URL (`--url`) as an input and also an optional `--output` to specify the file path.


Running the program:
- Provide your own URL: `./main --url="https://go.dev/dl/go1.20.3.windows-amd64.zip"`
- Provide your own URL and desired output file path: `./main --url="https://go.dev/dl/go1.20.3.windows-amd64.zip" --output=downloads/windows-amd64-archive.zip`
