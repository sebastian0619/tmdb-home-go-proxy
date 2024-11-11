package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
)

func main() {
	// 根据环境变量决定角色
	role := os.Getenv("ROLE")
	port := os.Getenv("PORT")
	if port == "" {
		port = "3666"
	}

	if role == "host" {
		// 初始化并启动主机
		initHost()
		http.HandleFunc("/", handleHostProxy)
		fmt.Printf("Host server running on port %s\n", port)
	} else if role == "backend" {
		// 初始化并启动后台机
		initBackend()
		http.HandleFunc("/", handleBackendProxy)
		http.HandleFunc("/logs", handleLogs)
		fmt.Printf("Backend server running on port %s\n", port)
	} else {
		log.Fatalf("Unknown role: %s. Use 'host' or 'backend' for the ROLE environment variable.", role)
	}

	log.Fatal(http.ListenAndServe(":"+port, nil))
}
