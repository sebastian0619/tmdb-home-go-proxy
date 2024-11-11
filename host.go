package main

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"
)

// 主机设置
var (
	backendHosts = strings.Split(getEnv("BACKEND_HOSTS", "192.168.1.10:3666,192.168.1.11:3666,192.168.1.12:3666"), ",")
	hostWeights  = make(map[string]int)
	weightMutex  sync.Mutex
)

// 初始化主机配置和权重
func initHost() {
	fmt.Println("Initializing host service...")
	for _, host := range backendHosts {
		hostWeights[host] = 1
	}
	go startLatencyMeasurement()
}

// 定时测速每个后台机并更新权重
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
	return backendHosts[0]
}

// 测试后台机延迟并更新权重
func updateWeights() {
	weightMutex.Lock()
	defer weightMutex.Unlock()

	for _, host := range backendHosts {
		latency := measureLatency(host)
		log.Printf("Latency for %s: %v", host, latency)

		// 根据延迟时间调整权重
		if latency > 0 {
			hostWeights[host] = int(time.Second / latency)
		} else {
			hostWeights[host] = 1
		}
		log.Printf("Updated weight for %s: %d", host, hostWeights[host])
	}
}

// 测速函数
func measureLatency(host string) time.Duration {
	start := time.Now()
	resp, err := http.Get(fmt.Sprintf("http://%s/logs", host))
	if err != nil {
		log.Printf("Error measuring latency for %s: %v", host, err)
		return time.Hour
	}
	defer resp.Body.Close()
	return time.Since(start)
}

// 主机的代理处理函数
func handleHostProxy(w http.ResponseWriter, r *http.Request) {
	backend := selectBackend()
	log.Printf("Proxying request to backend: %s", backend)

	resp, err := http.Get(fmt.Sprintf("http://%s%s", backend, r.URL.Path))
	if err != nil {
		http.Error(w, "Backend request failed", http.StatusInternalServerError)
		log.Printf("Error requesting backend %s: %v", backend, err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "Failed to read backend response", http.StatusInternalServerError)
		log.Printf("Error reading response from backend %s: %v", backend, err)
		return
	}

	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}
