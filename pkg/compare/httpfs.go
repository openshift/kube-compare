package compare

import (
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultHttpGetAttempts = 5

// isURL checks if the given path is a URL by verifying if it starts with "http://" or "https://".
func isURL(path string) bool {
	return strings.Index(path, "http://") == 0 || strings.Index(path, "https://") == 0
}

// HTTPFS represents a file system that retrieves files from a http server by returning the http response body,
// ideal for http servers that return raw files
type HTTPFS struct {
	baseURL string
	httpGet httpget
}

// httpget is a function type that defines the signature of functions used to retrieve HTTP resources.
type httpget func(url string) (int, string, io.ReadCloser, int64, error)

// Open creates a http request and returns a http body reader object representing a file for reading.
func (fs HTTPFS) Open(name string) (fs.File, error) {
	fullURL, err := url.JoinPath(fs.baseURL, name)
	if err != nil {
		return HTTPFile{}, err
	}
	body, contentLength, err := readHttpWithRetries(fs.httpGet, 5*time.Millisecond, fullURL, defaultHttpGetAttempts)
	if err != nil {
		return HTTPFile{}, err
	}
	file := HTTPFile{data: body, fi: HTTPFileInfo{name: name, size: contentLength, modTime: time.Now()}}
	return file, err
}

// httpgetImpl Implements a function to retrieve a url and return the results.
func httpgetImpl(url string) (int, string, io.ReadCloser, int64, error) {
	resp, err := http.Get(url)
	if err != nil {
		return 0, "", nil, 0, err
	}
	return resp.StatusCode, resp.Status, resp.Body, resp.ContentLength, nil
}

// readHttpWithRetries tries to http.Get the v.URL retries times before giving up.
func readHttpWithRetries(get httpget, duration time.Duration, u string, attempts int) (io.ReadCloser, int64, error) {
	var err error
	if attempts <= 0 {
		return nil, 0, fmt.Errorf("http attempts must be greater than 0, was %d", attempts)
	}
	for i := 0; i < attempts; i++ {
		var (
			statusCode    int
			status        string
			body          io.ReadCloser
			contentLength int64
		)
		if i > 0 {
			time.Sleep(duration)
		}

		// Try to get the URL
		statusCode, status, body, contentLength, err = get(u)

		// Retry Errors
		if err != nil {
			continue
		}

		if statusCode == http.StatusOK {
			return body, contentLength, nil
		}
		err := body.Close()
		if err != nil {
			return nil, 0, err
		}
		// Error - Set the error condition from the StatusCode
		err = fmt.Errorf("unable to read URL %q, server reported %s, status code=%d", u, status, statusCode)

		if statusCode >= 500 && statusCode < 600 {
			// Retry 500's
			continue
		} else {
			// Don't retry other StatusCodes
			break
		}
	}
	return nil, 0, err
}

// HTTPFile represents a file obtained from an HTTP response body.
type HTTPFile struct {
	fi   HTTPFileInfo
	data io.ReadCloser
}

// Stat returns the HTTP file information.
func (f HTTPFile) Stat() (fs.FileInfo, error) {
	return f.fi, nil
}

// Read returns the http body.
func (f HTTPFile) Read(b []byte) (int, error) {
	return f.data.Read(b)
}

// Close closes the http body reader.
func (f HTTPFile) Close() error {
	return f.data.Close()
}

// HTTPFileInfo represents information about the http raw resource
type HTTPFileInfo struct {
	name    string
	size    int64
	modTime time.Time
}

// Name returns the uri of the file from the requested base URL
func (f HTTPFileInfo) Name() string {
	return f.name
}

// Size returns the length of the http body
func (f HTTPFileInfo) Size() int64 {
	return f.size
}

// Mode returns the file mode bits - always returns fs.ModeTemporary because file isn't in local file system
// and is a http resource
func (f HTTPFileInfo) Mode() fs.FileMode {
	return fs.ModeTemporary
}

// ModTime returns the time of the http response
func (f HTTPFileInfo) ModTime() time.Time {
	return f.modTime
}

// IsDir abbreviation for Mode().IsDir()
func (f HTTPFileInfo) IsDir() bool {
	return f.Mode().IsDir()
}

// Sys underlying data source - returns nil
func (f HTTPFileInfo) Sys() any {
	return nil
}
