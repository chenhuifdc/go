package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"
)

func main() {
	start := time.Now()
	webDir := "web"

	// 读取并压缩静态文件
	files := make(map[string]string)
	err := filepath.WalkDir(webDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(webDir, path)
		relPath = strings.ReplaceAll(relPath, "\\", "/")

		// 压缩文本类型
		ext := filepath.Ext(relPath)
		compressible := []string{".html", ".css", ".js", ".json", ".svg", ".txt", ".xml", ".yaml", ".yml"}
		shouldCompress := false
		for _, e := range compressible {
			if ext == e {
				shouldCompress = true
				break
			}
		}

		var encoded string
		if shouldCompress {
			var buf bytes.Buffer
			gz := gzip.NewWriter(&buf)
			gz.Write(data)
			gz.Close()
			encoded = base64.StdEncoding.EncodeToString(buf.Bytes())
			files[relPath] = "gz:" + encoded
		} else {
			encoded = base64.StdEncoding.EncodeToString(data)
			files[relPath] = "raw:" + encoded
		}
		return nil
	})
	if err != nil {
		panic(err)
	}

	// 生成 Go 文件
	funcMap := template.FuncMap{
		"hasPrefix":  strings.HasPrefix,
		"trimPrefix": strings.TrimPrefix,
		"sha256": func(data string) string {
			h := sha256.Sum256([]byte(data))
			return base64.URLEncoding.EncodeToString(h[:12])
		},
	}
	tmpl := template.Must(template.New("").Funcs(funcMap).Parse(buildTemplate))

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, map[string]interface{}{
		"Files":       files,
		"BuildTime":   time.Now().Format(time.RFC3339),
		"GitCommit":   getGitCommit(),
	})
	if err != nil {
		panic(err)
	}

	// 写入文件
	outputDir := "internal/static"
	os.MkdirAll(outputDir, 0755)
	err = os.WriteFile(filepath.Join(outputDir, "files.go"), buf.Bytes(), 0644)
	if err != nil {
		panic(err)
	}

	fmt.Printf("构建完成: %d 个文件, 耗时 %v\n", len(files), time.Since(start))
}

func getGitCommit() string {
	// 简单返回空，实际可执行 git 命令
	return "unknown"
}

const buildTemplate = `package static

import (
	"bytes"
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
{{- range $path, $encoded := .Files }}
	"{{ $path }}": {
		Path:        "{{ $path }}",
		ContentType: getContentType("{{ $path }}"),
		Compressed:  {{ if hasPrefix $encoded "gz:" }}true{{ else }}false{{ end }},
		Data:        "{{ trimPrefix $encoded "gz:" | trimPrefix "raw:" }}",
		ETag:        "{{ sha256 $encoded }}",
	},
{{- end }}
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
`