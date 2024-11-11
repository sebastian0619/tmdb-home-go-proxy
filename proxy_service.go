// proxy_service.go
package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sync"
)

// 获取环境变量，如果没有设置则返回默认值
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

var (
	targetURL    = getEnv("TARGET_URL", "https://www.themoviedb.org")
	role         = getEnv("ROLE", "backend") // 默认角色为 backend
	port         = getEnv("PORT", "3666")
	logFilePath  = "proxy_service.log"
	logFileMutex sync.Mutex
)

// 初始化日志文件
func initLog() *os.File {
	logFile, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	log.SetOutput(logFile)
	log.Println("Starting proxy service...")
	return logFile
}

// 写日志
func writeLog(entry string) {
	logFileMutex.Lock()
	defer logFileMutex.Unlock()
	log.Println(entry)
}

// 代理处理函数
func handleProxy(w http.ResponseWriter, r *http.Request) {
	// 解析目标 URL
	target, err := url.Parse(targetURL)
	if err != nil {
		http.Error(w, "Invalid target URL", http.StatusInternalServerError)
		return
	}
	r.Host = target.Host

	// 创建一个带有忽略证书的自定义 HTTP 客户端
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	// 创建代理
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = target.Scheme
			req.URL.Host = target.Host
			req.URL.Path = r.URL.Path
			req.URL.RawQuery = r.URL.RawQuery

			// 设置常见请求头来模拟真实浏览器请求
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/85.0.4183.121 Safari/537.36")
			req.Header.Set("Accept-Language", "en-US,en;q=0.9")
			req.Header.Set("Accept-Encoding", "gzip, deflate, br")
			req.Header.Set("Connection", "keep-alive")
			req.Header.Set("Referer", targetURL)
			req.Header.Set("Origin", targetURL)
		},
		Transport: transport,
		ModifyResponse: func(resp *http.Response) error {
			// 可选：这里可以修改响应，例如替换 HTML 内容等
			if resp.StatusCode == http.StatusForbidden {
				body, _ := io.ReadAll(resp.Body)
				writeLog(fmt.Sprintf("Received 403 response: %s", string(body)))
			}
			return nil
		},
	}

	// 记录请求日志
	writeLog(fmt.Sprintf("Proxying request: %s %s", r.Method, r.URL.String()))

	// 代理请求到目标服务器
	proxy.ServeHTTP(w, r)
}

// 日志获取处理函数
func handleLogs(w http.ResponseWriter, r *http.Request) {
	logFileMutex.Lock()
	defer logFileMutex.Unlock()

	logFile, err := os.Open(logFilePath)
	if err != nil {
		http.Error(w, "Failed to open log file", http.StatusInternalServerError)
		return
	}
	defer logFile.Close()

	// 读取日志文件并返回内容
	data, err := io.ReadAll(logFile)
	if err != nil {
		http.Error(w, "Failed to read log file", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write(data)
}

func main() {
	// 初始化日志文件
	logFile := initLog()
	defer logFile.Close()

	http.HandleFunc("/", handleProxy)
	http.HandleFunc("/logs", handleLogs) // 提供获取日志的接口

	fmt.Printf("%s running on port %s, forwarding to %s\n", role, port, targetURL)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
