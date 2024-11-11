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
)

// 后台机设置
var (
	targetURL   = getEnv("TARGET_URL", "https://www.themoviedb.org")
	logFilePath = "proxy_service.log"
)

// 初始化后台机设置
func initBackend() {
	fmt.Println("Initializing backend service...")
}

// 代理到目标 URL 的处理函数
func handleBackendProxy(w http.ResponseWriter, r *http.Request) {
	// 解析目标 URL
	target, err := url.Parse(targetURL)
	if err != nil {
		http.Error(w, "Invalid target URL", http.StatusInternalServerError)
		return
	}

	// 使用默认的 SingleHostReverseProxy 进行代理配置
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	// 设置代理的 Director 方法
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host
		req.URL.Path = r.URL.Path
		req.URL.RawQuery = r.URL.RawQuery

		// 设置请求头以模拟浏览器请求
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/85.0.4183.121 Safari/537.36")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		req.Header.Set("Accept-Encoding", "gzip, deflate, br")
		req.Header.Set("Connection", "keep-alive")
		req.Header.Set("Referer", targetURL)
		req.Header.Set("Origin", targetURL)
	}

	// ModifyResponse 用于捕获 HTML 响应并替换资源链接
	proxy.ModifyResponse = func(resp *http.Response) error {
		// 检查内容类型是否为 HTML
		if strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			// 关闭响应体并替换为新的内容
			resp.Body.Close()

			// 将目标 URL 替换为代理服务器地址
			updatedBody := strings.ReplaceAll(
				string(body),
				"https://www.themoviedb.org",
				fmt.Sprintf("http://%s", r.Host),
			)
			// 替换静态资源地址（如图片、CSS、JavaScript）
			updatedBody = strings.ReplaceAll(
				updatedBody,
				"https://image.tmdb.org",
				fmt.Sprintf("http://%s/static", r.Host),
			)

			// 将更新后的内容写回到响应体
			resp.Body = ioutil.NopCloser(strings.NewReader(updatedBody))
			resp.ContentLength = int64(len(updatedBody))
			resp.Header.Set("Content-Length", fmt.Sprint(len(updatedBody)))
		}
		return nil
	}
	
	// 写日志到文件
func writeLog(entry string) {
	logFile, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer logFile.Close()

	log.SetOutput(logFile)
	log.Println(entry)
}

// 后台机日志接口
func handleLogs(w http.ResponseWriter, r *http.Request) {
	logFile, err := os.Open(logFilePath)
	if err != nil {
		http.Error(w, "Failed to open log file", http.StatusInternalServerError)
		return
	}
	defer logFile.Close()

	data, err := io.ReadAll(logFile)
	if err != nil {
		http.Error(w, "Failed to read log file", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write(data)
}

// 获取环境变量
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
