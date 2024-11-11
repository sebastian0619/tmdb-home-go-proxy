package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// 环境变量获取
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// 全局变量
var (
	role         = getEnv("ROLE", "backend")
	targetURL    = getEnv("TARGET_URL", "https://www.themoviedb.org")
	port         = getEnv("PORT", "3666")
	backendHosts = strings.Split(getEnv("BACKEND_HOSTS", "203.0.113.10:3666,203.0.113.11:3666,203.0.113.12:3666"), ",")
	hostWeights  = make(map[string]int)
	weightMutex  sync.Mutex
	logFilePath  = "proxy_service.log"
)

// 初始化权重
func initWeights() {
	for _, host := range backendHosts {
		hostWeights[host] = 1 // 初始权重设为 1
	}
}

// 写日志
func writeLog(entry string) {
	logFileMutex := &sync.Mutex{}
	logFileMutex.Lock()
	defer logFileMutex.Unlock()

	logFile, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer logFile.Close()

	log.SetOutput(logFile)
	log.Println(entry)
}

// 测速函数
func measureLatency(host string) time.Duration {
	start := time.Now()
	resp, err := http.Get(fmt.Sprintf("http://%s/logs", host)) // 使用 /logs 接口测试响应速度
	if err != nil {
		log.Printf("Error measuring latency for %s: %v", host, err)
		return time.Duration(time.Hour) // 如果失败，返回较长时间
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body) // 读取响应以计算完整时间
	return time.Since(start)
}

// 更新权重
func updateWeights() {
	weightMutex.Lock()
	defer weightMutex.Unlock()

	for _, host := range backendHosts {
		latency := measureLatency(host)
		log.Printf("Latency for %s: %v", host, latency)

		// 更新权重：将权重与响应速度成反比
		if latency > 0 {
			hostWeights[host] = int(time.Second / latency)
		} else {
			hostWeights[host] = 1
		}
		log.Printf("Updated weight for %s: %d", host, hostWeights[host])
	}
}

// 定时器每半小时更新权重
func startLatencyMeasurement() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("Starting latency measurement...")
		updateWeights()
	}
}

// 选择后台机，按权重随机选择
func selectBackend() string {
	weightMutex.Lock()
	defer weightMutex.Unlock()

	totalWeight := 0
	for _, weight := range hostWeights {
		totalWeight += weight
	}

	randValue := rand.Intn(totalWeight)
	for host, weight := range hostWeights {
		if randValue < weight {
			return host
		}
		randValue -= weight
	}
	return backendHosts[0] // 默认返回第一个后台机
}

// 主机的代理处理函数，转发请求到后台机并将响应返回给客户端
func handleHostProxy(w http.ResponseWriter, r *http.Request) {
	backend := selectBackend()
	targetURL := fmt.Sprintf("http://%s%s", backend, r.URL.Path)

	// 创建转发请求
	proxyReq, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	// 复制请求头
	for header, values := range r.Header {
		for _, value := range values {
			proxyReq.Header.Add(header, value)
		}
	}

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	// 执行请求并获取响应
	resp, err := client.Do(proxyReq)
	if err != nil {
		http.Error(w, "Backend request failed", http.StatusBadGateway)
		log.Printf("Error requesting backend %s: %v", backend, err)
		return
	}
	defer resp.Body.Close()

	// 将响应头和状态码复制到客户端
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)

	// 将响应内容复制到客户端
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		log.Printf("Error copying response to client: %v", err)
	}
}

// 后台机的代理处理函数，转发请求到目标 URL
func handleBackendProxy(w http.ResponseWriter, r *http.Request) {
	target, err := url.Parse(targetURL)
	if err != nil {
		http.Error(w, "Invalid target URL", http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	// 设置代理请求头
	proxy.Director = func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = r.URL.Path
		req.URL.RawQuery = r.URL.RawQuery
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/85.0.4183.121 Safari/537.36")
		req.Header.Set("Accept-Language", "en-US,en;q=0.9")
		req.Header.Set("Accept-Encoding", "gzip, deflate, br")
		req.Header.Set("Connection", "keep-alive")
		req.Header.Set("Referer", targetURL)
		req.Header.Set("Origin", targetURL)
	}

	proxy.ServeHTTP(w, r)
}

// 后台机的日志处理函数
func handleLogs(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, logFilePath)
}

func main() {
	if role == "host" {
		// 主机启动：初始化权重和测速定时任务
		initWeights()
		go startLatencyMeasurement()
		http.HandleFunc("/", handleHostProxy)
		fmt.Printf("Host server running on port %s\n", port)
	} else if role == "backend" {
		// 后台机启动：代理请求和日志接口
		http.HandleFunc("/", handleBackendProxy)
		http.HandleFunc("/logs", handleLogs)
		fmt.Printf("Backend server running on port %s, forwarding to %s\n", port, targetURL)
	} else {
		log.Fatalf("Unknown role: %s. Use 'host' or 'backend'.", role)
	}

	log.Fatal(http.ListenAndServe(":"+port, nil))
}
