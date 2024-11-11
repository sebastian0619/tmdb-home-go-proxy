package main

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
)

// 获取环境变量的工具函数
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// 全局配置
var (
	targetURL      = getEnv("TARGET_URL", "https://www.themoviedb.org")
	logFilePath    = "proxy_service.log"
	staticMode     = getEnv("STATIC_MODE", "false")             // 环境变量，是否启用静态模式
	imageProxyURL  = getEnv("IMAGE_PROXY_URL", "https://image.tmdb.org") // 静态资源代理 URL
)

// 初始化后台机设置
func initBackend() {
	fmt.Println("Initializing backend service...")
}

// 后台机的代理处理函数
func handleBackendProxy(w http.ResponseWriter, r *http.Request) {
	// 解析目标 URL
	target, err := url.Parse(targetURL)
	if err != nil {
		http.Error(w, "Invalid target URL", http.StatusInternalServerError)
		return
	}

	// 使用 httputil.NewSingleHostReverseProxy 创建代理配置
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	// 设置代理的 Director 方法，修改请求以指向目标服务器
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host
		req.URL.Path = r.URL.Path
		req.URL.RawQuery = r.URL.RawQuery

		// 设置必要的请求头以模仿真实浏览器
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
			// 关闭原始响应体并替换为新的内容
			resp.Body.Close()

			// 将目标 URL 替换为代理服务器地址
			updatedBody := strings.ReplaceAll(
				string(body),
				"https://www.themoviedb.org",
				fmt.Sprintf("http://%s", r.Host),
			)

			// 根据环境变量设置静态资源地址
			if staticMode == "true" {
				// 静态模式：所有静态资源指向本地代理
				updatedBody = strings.ReplaceAll(
					updatedBody,
					"https://image.tmdb.org",
					fmt.Sprintf("http://%s/static", r.Host),
				)
			} else {
				// 非静态模式：静态资源指向指定的代理地址
				updatedBody = strings.ReplaceAll(
					updatedBody,
					"https://image.tmdb.org",
					imageProxyURL,
				)
			}

			// 将更新后的内容写回到响应体
			resp.Body = ioutil.NopCloser(strings.NewReader(updatedBody))
			resp.ContentLength = int64(len(updatedBody))
			resp.Header.Set("Content-Length", fmt.Sprint(len(updatedBody)))
		}
		return nil
	}

	// 使用代理将请求转发到目标服务器
	proxy.ServeHTTP(w, r)
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

// 后台机日志接口，用于获取日志内容
func handleLogs(w http.ResponseWriter, r *http.Request) {
	logFile, err := os.Open(logFilePath)
	if err != nil {
		http.Error(w, "Failed to open log file", http.StatusInternalServerError)
		return
	}
	defer logFile.Close()

	data, err := ioutil.ReadAll(logFile)
	if err != nil {
		http.Error(w, "Failed to read log file", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write(data)
}
