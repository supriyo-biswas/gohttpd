# gohttpd

Yet another HTTP web server written in Go (using net/http).

## Features

* Supports gzip compression
* Supports If-Modified-Since headers
* Supports GET and HEAD requests
* Blocks access to hidden files/directories
* Directory listing (turned off by default)
* Request logging
* No dependencies on external libraries

## Getting started

Build the binary and start it up:

```bash
go build -o httpd httpd.go
./httpd
```

By default it serves content on port 8080 and from the current directory,
although you can change this behaviour with the built-in flags (use
`./httpd -help` for details).

You can also build a static binary. As an example, on Linux/amd64, use:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -tags netgo -ldflags '-w' -o httpd_amd64 httpd.go
```

## License

[MIT](https://opensource.org/licenses/MIT)
