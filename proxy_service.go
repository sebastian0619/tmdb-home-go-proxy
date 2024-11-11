package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
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

// 获取环境变量的函数
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// 全局变量定义
var (
	role         = getEnv("ROLE", "backend")
	targetURL    = getEnv("TARGET_URL", "https://www.themoviedb.org")
	port         = getEnv("PORT", "3666")
	backendHosts = strings.Split(getEnv("BACKEND_HOSTS", "203.0.113.10:3666,203.0.113.11:3666,203.0.113.12:3666"), ",")
	hostWeights  = make(map[string]int)
	weightMutex  sync.Mutex
	logFilePath  = "proxy_service.log"
)

// 初始化权重（默认所有后台机的初始权重为 1）
func initWeights() {
	for _, host := range backendHosts {
		hostWeights[host] = 1
	}
}

// 测速函数，用于计算每个后台机的延迟
func measureLatency(host string) time.Duration {
	start := time.Now()
	resp, err := http.Get(fmt.Sprintf("http://%s/logs", host))
	if err != nil {
		log.Printf("Error measuring latency for %s: %v", host, err)
		return time.Duration(time.Hour) // 若失败，返回较长的延迟
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)
	return time.Since(start)
}

// 更新权重：根据测速结果调整后台机的权重
func updateWeights() {
	weightMutex.Lock()
	defer weightMutex.Unlock()

	for _, host := range backendHosts {
		latency := measureLatency(host)
		log.Printf("Latency for %s: %v", host, latency)
		if latency > 0 {
			hostWeights[host] = int(time.Second / latency)
		} else {
			hostWeights[host] = 1
		}
		log.Printf("Updated weight for %s: %d", host, hostWeights[host])
	}
}

// 定时器：每半小时更新一次权重
func startLatencyMeasurement() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		log.Println("Starting latency measurement...")
		updateWeights()
	}
}

// 按权重随机选择一个后台机
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
	return backendHosts[0]
}

// 主机的代理处理函数
func handleHostProxy(w http.ResponseWriter, r *http.Request) {
	backend := selectBackend()
	targetURL := fmt.Sprintf("http://%s%s", backend, r.URL.Path)

	proxyReq, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, "Failed to create request", http.StatusInternalServerError)
		return
	}

	// 复制客户端的请求头
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

	// 发起请求并获取响应
	resp, err := client.Do(proxyReq)
	if err != nil {
		http.Error(w, "Backend request failed", http.StatusBadGateway)
		log.Printf("Error requesting backend %s: %v", backend, err)
		return
	}
	defer resp.Body.Close()

	// 将响应头和状态码传给客户端
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)

	// 将响应内容传给客户端
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		log.Printf("Error copying response to client: %v", err)
	}
}

// 后台机的代理处理函数
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

// 日志处理函数：返回日志内容
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

func main() {
	if role == "host" {
		initWeights()
		go startLatencyMeasurement()
		http.HandleFunc("/", handleHostProxy)
		fmt.Printf("Host server running on port %s\n", port)
	} else if role == "backend" {
		http.HandleFunc("/", handleBackendProxy)
		http.HandleFunc("/logs", handleLogs)
		fmt.Printf("Backend server running on port %s, forwarding to %s\n", port, targetURL)
	} else {
		log.Fatalf("Unknown role: %s. Use 'host' or 'backend'.", role)
	}

	log.Fatal(http.ListenAndServe(":"+port, nil))
}
