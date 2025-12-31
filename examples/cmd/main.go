// cmd/client/main.go 中修改
package main

import (
	"flag"
	"fmt"
	"github.com/sweemingdow/sdim/examples/client"
	"os"
)

func main() {
	// 命令行参数
	serverAddr := flag.String("server", "127.0.0.1:5077", "Server address (host:port)")
	userID := flag.String("uid", "default-user", "User ID")
	clientType := flag.Int("type", 1, "Client type")
	flag.Parse()

	// 显示欢迎信息
	fmt.Println("Starting Interactive Client")
	fmt.Printf("Server: %s\n", *serverAddr)
	fmt.Printf("User ID: %s\n", *userID)
	fmt.Printf("Client Type: %d\n", *clientType)
	fmt.Println()

	// 创建交互式客户端
	var runner interface {
		Run() error
	}

	runner = client.NewSimpleInteractiveClient(*serverAddr, *userID, *clientType)

	// 运行客户端
	if err := runner.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Client exited normally")
}
