package main

import (
	"compress/gzip"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"
)

var mimes = map[string]string {
	// common web types
	"html" : "text/html",
	"js"   : "application/javascript",
	"css"  : "text/css",
	"xml"  : "text/xml",
	"xhtml": "application/xhtml+xml",
	"txt"  : "text/plain",
	"json" : "application/json",

	// images
	"png"  : "image/png",
	"jpg"  : "image/jpeg",
	"jpeg" : "image/jpeg",
	"gif"  : "image/gif",
	"svg"  : "image/svg+xml",
	"webp" : "image/webp",
	"ico"  : "image/x-icon",

	// media
	"mp3"  : "audio/mpeg",
	"ogg"  : "audio/ogg",
	"m4a"  : "audio/x-m4a",
	"avi"  : "video/x-msvideo",
	"mp4"  : "video/mp4",
	"mov"  : "video/quicktime",
	"ts"   : "video/mp2t",
	"webm" : "video/webm",
	"m3u8" : "application/vnd.apple.mpegurl",

	// fonts
	"eot"  : "application/vnd.ms-fontobject",
	"ttf"  : "font/ttf",
	"woff" : "font/woff",
	"woff2": "font/woff2",
	"otf"  : "font/otf",

	// documents
	"pdf"  : "application/pdf",
	"csv"  : "text/csv",

	// archives
	"7z"   : "application/x-ms-compressed",
	"zip"  : "application/zip",
	"rar"  : "application/x-rar-compressed",
}

var compressExts = []string {
	"css",
	"csv",
	"eot",
	"html",
	"js",
	"json",
	"otf",
	"svg",
	"ttf",
	"txt",
	"xhtml",
	"xml",
}

var indexFiles = []string {
	"index.html",
	"index.xhtml",
}

type listTemplateInfo struct {
	Path string
	Files []os.FileInfo
}

var listTemplate = `
<!DOCTYPE html>
<html>
<head>
  <title>Index of {{ .Path }}</title>
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <style>
    html, body, table, tr {
      width: 100%;
    }
    .main {
      max-width: 992px;
      margin: 0 auto;
    }
    h2 {
      margin-top: 5px;
      margin-bottom: 5px;
    }
    tr {
      vertical-align: top;
    }
    a {
      text-decoration: none;
    }
    a:hover {
      text-decoration: underline;
    }
    td.name {
      width: 60%;
    }
    td.size, td.last-modified {
      width: 20%;
    }
  </style>
</head>
<body>
  <div class="main">
    <h2>Index of {{ .Path }}</h2>
    <table>
      <tr>
        <td class="name"><b>Name</b></td>
        <td class="size"><b>Size (bytes)</b></td>
        <td class="last-modified"><b>Last Modified</b></td>
      </tr>
      <tr>
      {{ range .Files }}
        {{ if (ne (index .Name 0) 46) }}
        <tr>
         <td class="name">
           <a href="{{ .Name }}{{ if .IsDir }}/{{ end }}">
             {{ .Name }}{{ if .IsDir }}/{{ end }}
           </a>
         </td>
         <td class="size">
           {{ if .IsDir }}
             -
           {{ else }}
             {{ .Size }}
           {{ end }}
         </td>
         <td class="last-modified">
           {{ if .IsDir }}
             -
           {{ else }}
             {{ .ModTime.Format "2 Jan 2006 15:04" }}
           {{ end }}
         </td>
        </tr>
        {{ end }}
      {{ end }}
    </table>
  </div>
</body>
</html>`

var gzPool = sync.Pool {
	New: func() interface{} {
		w := gzip.NewWriter(ioutil.Discard)
		return w
	},
}

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w *gzipResponseWriter) WriteHeader(status int) {
	w.ResponseWriter.WriteHeader(status)
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}

	return false
}

func isHiddenPath(path string) bool {
	return len(path) > 1 && path[0] == '.' || strings.Index(path, "/.") != -1
}

func showListing(writer http.ResponseWriter, path string) {
	files, err := ioutil.ReadDir(path)
	if err != nil {
		http.Error(writer, "File not found", 404)
		return
	}

	t, err := template.New("listTemplate").Parse(listTemplate)
	if err != nil {
		panic(err)
	}

	err = t.Execute(writer, listTemplateInfo{
		Path: path,
		Files: files,
	})

	if err != nil {
		panic(err)
	}
}

func requestHandler(
	writer http.ResponseWriter,
	request *http.Request,
	listDir bool,
) {
	if request.Method != "GET" && request.Method != "HEAD" {
		http.Error(writer, "Method not allowed", 405)
		return
	}

	path := filepath.Clean(request.URL.Path[1:])
	if isHiddenPath(path) {
		http.Error(writer, "File not found", 404)
		return
	}

	stat, err := os.Stat(path)
	if err != nil {
		http.Error(writer, "File not found", 404)
		return
	}

	if stat.IsDir() {
		lastChar := request.URL.Path[len(request.URL.Path) - 1]

		// redirect to the directory URL with '/' at end.
		if path != "." && lastChar != '/' {
			writer.Header().Set("Location", fmt.Sprintf("/%s/", path))
			writer.WriteHeader(301)
			return
		}

		found := false

		for _, i := range indexFiles {
			indexPath := fmt.Sprintf("%s/%s", path, i)
			stat, err = os.Stat(indexPath)

			if err == nil && !stat.IsDir() {
				found = true
				path = indexPath
				break
			}
		}

		if !found {
			if listDir {
				showListing(writer, path)
			} else {
				http.Error(writer, "File not found", 404)
			}

			return
		}
	}

	file, err := os.Open(path)
	defer file.Close()

	if err != nil {
		http.Error(writer, "File not found", 404)
		return
	}

	extension := filepath.Ext(path)
	if extension != "" {
		extension = extension[1:]
	}

	mimeType, ok := mimes[extension]
	if !ok {
		mimeType = "application/octet-stream"
	}

	// truncate time to seconds to prevent caching issues
	// because the resolution of the If-Modified-Since header
	// is only precise upto a second.
	lastModified := stat.ModTime().UTC().Truncate(time.Second)
	lastModifiedStr := lastModified.Format(http.TimeFormat)

	writer.Header().Set("Last-Modified", lastModifiedStr)
	writer.Header().Set("Content-Type", mimeType)

	ifModifiedSince := request.Header.Get("If-Modified-Since")
	since, err := time.Parse(http.TimeFormat, ifModifiedSince)

	if err == nil {
		if lastModified.Before(since) || lastModified.Equal(since) {
			writer.WriteHeader(304)
			return
		}
	}

	if request.Method == "HEAD" {
		return
	}

	acceptEnc := request.Header.Get("Accept-Encoding")

	if stat.Size() > 1024 && strings.Contains(acceptEnc, "gzip") &&
	   extension != "" && stringInSlice(extension, compressExts) {
		writer.Header().Set("Content-Encoding", "gzip")

		gz := gzPool.Get().(*gzip.Writer)
		gz.Reset(writer)

		defer gzPool.Put(gz)
		defer gz.Close()

		io.Copy(&gzipResponseWriter{ResponseWriter: writer, Writer: gz}, file)
	} else {
		io.Copy(writer, file)
	}
}

func handlerWrap(
	handler func(http.ResponseWriter, *http.Request, bool),
	context bool,
) http.HandlerFunc {
	return (func(writer http.ResponseWriter, request *http.Request) {
		requestTime := time.Now()
		handler(writer, request, context)

		portIndex := strings.LastIndex(request.RemoteAddr, ":")
		clientIP := request.RemoteAddr[:portIndex]

		reflectWriter := reflect.ValueOf(writer)
		statusCode := reflectWriter.Elem().FieldByName("status")

		fmt.Printf(
			"%v %#v %v %#v %v %#v %#v\n",
			clientIP,
			requestTime.Format(time.RFC822Z),
			request.Method,
			request.RequestURI,
			statusCode,
			request.Header.Get("Referer"),
			request.Header.Get("User-Agent"),
		)
	})
}

func mainWithExitCode() int {
	port := flag.Int("port", 8080, "port number to bind")
	home := flag.String("home", ".", "web server home directory")
	listDir := flag.Bool("listdir", false, "enable directory listing")

	flag.Parse()

	if *port < 1 || *port > 65535 {
		fmt.Println("invalid port number: ", port)
		flag.PrintDefaults()
		return 1
	}

	if err := os.Chdir(*home); err != nil {
		fmt.Println("unable to chdir: ", err)
		flag.PrintDefaults()
		return 1
	}

	fmt.Println("* Serving on port", *port, "from", *home)
	http.Handle("/", handlerWrap(requestHandler, *listDir))

	bindPort := fmt.Sprintf(":%d", *port)
	err := http.ListenAndServe(bindPort, nil)

	if err != nil && err != http.ErrServerClosed {
		fmt.Println("unable to start server", err)
		return 1
	}

	return 0
}

func main() {
	os.Exit(mainWithExitCode())
}