// proxy_server.go
package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil" // 添加这个导入
	"net/url"
	"os"
)

var targetURL = "https://www.themoviedb.org"

func handleProxy(w http.ResponseWriter, r *http.Request) {
	target, _ := url.Parse(targetURL)
	r.Host = target.Host

	// 创建转发请求
	proxy := httputil.ReverseProxy{ // 修改为 httputil.ReverseProxy
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.URL.Path = r.URL.Path
			req.URL.RawQuery = r.URL.RawQuery
			req.Header.Set("User-Agent", "Your-User-Agent")
		},
	}

	// 代理请求到目标服务器
	proxy.ServeHTTP(w, r)
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3666"
	}
	http.HandleFunc("/", handleProxy)
	fmt.Printf("Proxy server running on port %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

