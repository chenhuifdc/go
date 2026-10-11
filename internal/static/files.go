package static

import (
	"compress/gzip"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
)

// FileInfo 静态文件信息
type FileInfo struct {
	Path        string
	ContentType string
	Compressed  bool
	Data        string
	ETag        string
}

// Files 内嵌的静态文件映射
var Files = map[string]FileInfo{
	"css/app.css": {
		Path:        "css/app.css",
		ContentType: getContentType("css/app.css"),
		Compressed:  true,
		Data:        "raw:",
		ETag:        "VkauX16b342IYPIB",
	},
	"index.html": {
		Path:        "index.html",
		ContentType: getContentType("index.html"),
		Compressed:  true,
		Data:        "raw:",
		ETag:        "q50jXTIbjD3mpRyT",
	},
	"js/api.js": {
		Path:        "js/api.js",
		ContentType: getContentType("js/api.js"),
		Compressed:  true,
		Data:        "raw:",
		ETag:        "cqEoa2LCoczhlKxm",
	},
	"js/app.js": {
		Path:        "js/app.js",
		ContentType: getContentType("js/app.js"),
		Compressed:  true,
		Data:        "raw:",
		ETag:        "JdTmnldzIAm5tWns",
	},
	"js/pages/accounts.js": {
		Path:        "js/pages/accounts.js",
		ContentType: getContentType("js/pages/accounts.js"),
		Compressed:  true,
		Data:        "raw:",
		ETag:        "2q2X6G2nGgsjaAHH",
	},
	"js/pages/broadcast.js": {
		Path:        "js/pages/broadcast.js",
		ContentType: getContentType("js/pages/broadcast.js"),
		Compressed:  true,
		Data:        "raw:",
		ETag:        "PSWkeDIcMYABu_Ni",
	},
	"js/pages/contacts.js": {
		Path:        "js/pages/contacts.js",
		ContentType: getContentType("js/pages/contacts.js"),
		Compressed:  true,
		Data:        "raw:",
		ETag:        "yJ5U4Ce1xKIYvaF-",
	},
	"js/pages/dashboard.js": {
		Path:        "js/pages/dashboard.js",
		ContentType: getContentType("js/pages/dashboard.js"),
		Compressed:  true,
		Data:        "raw:",
		ETag:        "E3efwFnaYUHj_eoS",
	},
	"js/pages/groups.js": {
		Path:        "js/pages/groups.js",
		ContentType: getContentType("js/pages/groups.js"),
		Compressed:  true,
		Data:        "raw:",
		ETag:        "ea27taCZaig2dFD7",
	},
	"js/pages/risk.js": {
		Path:        "js/pages/risk.js",
		ContentType: getContentType("js/pages/risk.js"),
		Compressed:  true,
		Data:        "raw:",
		ETag:        "7bze4IuZZo5_D0vC",
	},
	"js/pages/settings.js": {
		Path:        "js/pages/settings.js",
		ContentType: getContentType("js/pages/settings.js"),
		Compressed:  true,
		Data:        "raw:",
		ETag:        "wTDvRcBJrQ4VZeHj",
	},
	"js/store.js": {
		Path:        "js/store.js",
		ContentType: getContentType("js/store.js"),
		Compressed:  true,
		Data:        "raw:",
		ETag:        "FfeqdjyOSwEq4MV0",
	},
}

// ServeStatic 处理静态文件请求
func ServeStatic(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}

	file, ok := Files[path]
	if !ok {
		// SPA 路由回退
		if _, ok := Files["index.html"]; ok {
			file = Files["index.html"]
		} else {
			http.NotFound(w, r)
			return
		}
	}

	w.Header().Set("Content-Type", file.ContentType)
	w.Header().Set("ETag", file.ETag)
	w.Header().Set("Cache-Control", "public, max-age=31536000")

	// 解码
	data, err := base64.StdEncoding.DecodeString(file.Data)
	if err != nil {
		http.Error(w, "Internal Error", 500)
		return
	}

	// 解压
	if file.Compressed {
		gr, err := gzip.NewReader(strings.NewReader(string(data)))
		if err != nil {
			http.Error(w, "Internal Error", 500)
			return
		}
		defer gr.Close()
		data, err = io.ReadAll(gr)
		if err != nil {
			http.Error(w, "Internal Error", 500)
			return
		}
	}

	// 检查 ETag
	if r.Header.Get("If-None-Match") == file.ETag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Write(data)
}

func getContentType(path string) string {
	ext := path[strings.LastIndex(path, "."):]
	switch ext {
	case ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "application/javascript; charset=utf-8"
	case ".json":
		return "application/json; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".woff":
		return "font/woff"
	case ".woff2":
		return "font/woff2"
	case ".ttf":
		return "font/ttf"
	case ".eot":
		return "application/vnd.ms-fontobject"
	default:
		return "application/octet-stream"
	}
}
