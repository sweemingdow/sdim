package client

import (
	"bufio"
	"fmt"
	"github.com/sweemingdow/gmicro_pkg/pkg/parser/json"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// SimpleInteractiveClient 简单版交互式客户端
type SimpleInteractiveClient struct {
	*Client
	reader   *bufio.Reader
	quitChan chan struct{}
	wg       sync.WaitGroup
}

// NewSimpleInteractiveClient 创建简单版交互式客户端
func NewSimpleInteractiveClient(addr string, uid string, cType int) *SimpleInteractiveClient {
	return &SimpleInteractiveClient{
		Client:   NewClient(addr, uid, cType),
		reader:   bufio.NewReader(os.Stdin),
		quitChan: make(chan struct{}),
	}
}

// Run 运行简单版交互式客户端
func (sic *SimpleInteractiveClient) Run() error {
	// 连接服务器
	fmt.Printf("Connecting to %s...\n", sic.addr)
	if err := sic.Connect(); err != nil {
		return fmt.Errorf("connection failed: %v", err)
	}

	fmt.Println("✓ Connected successfully!")

	// 启动读取循环
	if err := sic.StartReadLoop(); err != nil {
		return fmt.Errorf("failed to start read loop: %v", err)
	}

	// 启动帧处理 goroutine
	sic.wg.Add(1)
	go sic.frameHandler()

	// 显示帮助
	sic.showHelp()

	// 处理退出信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 启动输入处理 goroutine
	sic.wg.Add(1)
	go sic.inputHandler()

	var shutdownReason string
	// 等待退出信号或 quitChan
	select {
	case <-sigChan:
		fmt.Println("\nReceived shutdown signal...")
		shutdownReason = "signal"
	case <-sic.quitChan:
		// 正常退出
		shutdownReason = "quit"
	}

	// 触发关闭
	fmt.Printf("Shutting down (%s)...\n", shutdownReason)
	sic.Close()

	// 等待所有 goroutine 优雅退出
	sic.wg.Wait()

	return nil
}

// frameHandler 处理接收到的帧
func (sic *SimpleInteractiveClient) frameHandler() {
	defer sic.wg.Done()

	frameChan := sic.GetFrameChan()
	for {
		select {
		case frame, ok := <-frameChan:
			if !ok {
				// channel 已关闭（如 Close() 调用后）
				return
			}
			sic.handleIncomingFrame(frame)
		case <-sic.quitChan:
			return
		}
	}
}

// handleIncomingFrame 处理接收到的帧
func (sic *SimpleInteractiveClient) handleIncomingFrame(frame *Frame) {
	frameType := frame.Header.Ftype
	frameDesc := GetFrameTypeDesc(frameType)

	fmt.Printf("[DEBUG] Received frame: type=%d (%s)\n", frameType, frameDesc)

	switch frameType {
	case Pong:
		fmt.Println("🏓 Received pong from server")
	case ConnAck:
		fmt.Println("✅ Connection acknowledged by server")

	case SendAck:
		var saf SendFrameAck
		err := json.Parse(frame.Payload.Body, &saf)
		if err != nil {
			fmt.Printf("❌ Message parsed faild:%v\n", err)
			return
		}

		if saf.ErrCode != 0 {
			fmt.Printf("❌ Message sent faild:%s\n", saf.ErrDesc)
			return
		}

		fmt.Printf("✅ Message sent successfully, saf:%+v\n", saf)
	case Recv:
		var msgData map[string]interface{}
		if err := ParseFrameBody(frame, &msgData); err == nil {
			fmt.Printf("📨 Received message: %v\n", msgData)
		} else {
			fmt.Printf("📨 Received raw message: %s\n", string(frame.Payload.Body))
		}
	case Disconnect:
		fmt.Println("❌ Server closed the connection")
		close(sic.quitChan)
	default:
		fmt.Printf("📦 Received %s frame (type=%d)\n", frameDesc, frameType)
	}
}

// inputHandler 处理用户输入（修复：移除内部 goroutine，避免 EOF 死循环）
func (sic *SimpleInteractiveClient) inputHandler() {
	defer sic.wg.Done()

	for {
		// 检查是否已请求退出
		select {
		case <-sic.quitChan:
			return
		default:
		}

		// 检查连接状态
		if !sic.IsConnected() {
			fmt.Println("Connection lost. Exiting...")
			close(sic.quitChan)
			return
		}

		fmt.Print("> ")

		// 直接读取输入（不再用 goroutine）
		input, err := sic.reader.ReadString('\n')
		if err != nil {
			// 处理 EOF（如 Ctrl+D）或读取错误
			if err.Error() == "EOF" {
				fmt.Println("\nInput stream closed (EOF). Exiting...")
			} else {
				fmt.Printf("Input error: %v. Exiting...\n", err)
			}
			close(sic.quitChan)
			return
		}

		cmd := strings.TrimSpace(input)
		if cmd == "" {
			continue
		}

		sic.handleCommand(cmd)

		// 检查命令是否触发了退出
		select {
		case <-sic.quitChan:
			return
		default:
		}
	}
}

// handleCommand 处理用户命令
func (sic *SimpleInteractiveClient) handleCommand(cmd string) {
	parts := strings.SplitN(cmd, " ", 4)
	command := strings.ToLower(parts[0])

	switch command {
	case "ping":
		sic.handlePing()
	case "send":
		if len(parts) > 1 {
			sic.handleSend(parts[1])
		} else {
			fmt.Println("Usage: send <message>")
		}
	case "json":
		// json chatType receiveID `fdfdsf fdsfs  fdsf`
		content := parts[3][1 : len(parts[3])-1]
		ct, _ := strconv.Atoi(parts[1])
		sic.handleJSON(ChatType(ct), parts[2], content)
	case "status":
		sic.handleStatus()
	case "quit", "exit":
		fmt.Println("Goodbye!")
		close(sic.quitChan)
	case "help":
		sic.showHelp()
	case "clear":
		sic.handleClear()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		fmt.Println("Type 'help' for available commands")
	}
}

// handlePing 处理ping命令
func (sic *SimpleInteractiveClient) handlePing() {
	if err := sic.SendPing(); err != nil {
		fmt.Printf("Failed to send ping: %v\n", err)
		return
	}
	fmt.Println("Ping sent")
}

// handleSend 处理发送文本消息
func (sic *SimpleInteractiveClient) handleSend(message string) {
	fmt.Printf("Sending: %s\n", message)

	if err := sic.SendMessage(message); err != nil {
		fmt.Printf("Failed to send message: %v\n", err)
		return
	}

	fmt.Println("Message sent to server")
}

// handleJSON 处理发送JSON消息
func (sic *SimpleInteractiveClient) handleJSON(ct ChatType, receiveId string, content string) {
	sendFrb := &SendFrameBody{
		Sender:    sic.uid,
		Receiver:  receiveId,
		ChatType:  ct,
		SendMills: time.Now().UnixMilli(),
		Sign:      "",
		Ttl:       0,
		MsgBody: map[string]any{
			"content": map[string]any{
				"text": content,
			},
			"type": 1,
		},
	}

	fmt.Printf("Sending JSON: %v\n", sendFrb)

	if err := sic.SendJSON(sendFrb); err != nil {
		fmt.Printf("Failed to send JSON: %v\n", err)
		return
	}

	fmt.Println("JSON sent to server")
}

// handleStatus 处理状态命令
func (sic *SimpleInteractiveClient) handleStatus() {
	status := "Connected"
	if !sic.IsConnected() {
		status = "Disconnected"
	}

	fmt.Printf("Status: %s\n", status)
	fmt.Printf("Server: %s\n", sic.addr)
	fmt.Printf("User ID: %s\n", sic.uid)
	fmt.Printf("Client Type: %d\n", sic.cType)
}

// handleClear 处理清屏命令
func (sic *SimpleInteractiveClient) handleClear() {
	// ANSI escape sequence to clear screen
	fmt.Print("\033[2J\033[H")
}

// showHelp 显示帮助信息
func (sic *SimpleInteractiveClient) showHelp() {
	separator := strings.Repeat("=", 50)
	fmt.Println(separator)
	fmt.Println("Commands:")
	fmt.Println("  ping          - Send ping frame")
	fmt.Println("  send <msg>    - Send message")
	fmt.Println("  json <json>   - Send JSON message")
	fmt.Println("  status        - Show connection status")
	fmt.Println("  clear         - Clear the screen")
	fmt.Println("  quit          - Exit the client")
	fmt.Println("  help          - Show this help message")
	fmt.Println(separator)
}
