// client_test.go
package client

//
//import (
//	"encoding/binary"
//	"fmt"
//	"testing"
//	"time"
//)
//
//// TestClient_Connect 测试连接功能
//func TestClient_Connect(t *testing.T) {
//	// 启动测试服务器
//	testAddr := "127.0.0.1:5077"
//	//server := NewTestServer(testAddr)
//	//err := server.Start()
//	//if err != nil {
//	//	t.Fatalf("Failed to start test server: %v", err)
//	//}
//	//defer server.Stop()
//	//
//	//// 等待服务器启动
//	//time.Sleep(100 * time.Millisecond)
//
//	// 创建客户端
//	client := NewClient(testAddr, "test-user-001", 1)
//	defer client.Close()
//
//	// 测试连接
//	err := client.Connect()
//	if err != nil {
//		t.Fatalf("Client connect failed: %v", err)
//	}
//
//	// 验证连接状态
//	if !client.IsConnected() {
//		t.Error("Client should be connected")
//	}
//
//	fmt.Println("✓ Connect test passed")
//}
//
//// TestClient_SendPing 测试发送ping功能
//func TestClient_SendPing(t *testing.T) {
//	// 启动测试服务器
//	testAddr := "127.0.0.1:5077"
//	//server := NewTestServer(testAddr)
//	//err := server.Start()
//	//if err != nil {
//	//	t.Fatalf("Failed to start test server: %v", err)
//	//}
//	//defer server.Stop()
//
//	// 等待服务器启动
//	//time.Sleep(100 * time.Millisecond)
//
//	// 创建并连接客户端
//	client := NewClient(testAddr, "test-user-002", 1)
//	defer client.Close()
//
//	err := client.Connect()
//	if err != nil {
//		t.Fatalf("Client connect failed: %v", err)
//	}
//
//	// 启动读取循环
//	err = client.StartReadLoop()
//	if err != nil {
//		t.Fatalf("Start read loop failed: %v", err)
//	}
//
//	// 发送ping
//	err = client.SendPing()
//	if err != nil {
//		t.Fatalf("Send ping failed: %v", err)
//	}
//
//	// 等待响应
//	time.Sleep(200 * time.Millisecond)
//
//	fmt.Println("✓ SendPing test passed")
//}
//
//// TestClient_ReadFrame 测试读取帧功能
//func TestClient_ReadFrame(t *testing.T) {
//	// 启动测试服务器
//	testAddr := "127.0.0.1:5077"
//	//server := NewTestServer(testAddr)
//	//err := server.Start()
//	//if err != nil {
//	//	t.Fatalf("Failed to start test server: %v", err)
//	//}
//	//defer server.Stop()
//	//
//	//// 等待服务器启动
//	//time.Sleep(100 * time.Millisecond)
//
//	// 创建并连接客户端
//	client := NewClient(testAddr, "test-user-003", 1)
//	defer client.Close()
//
//	err := client.Connect()
//	if err != nil {
//		t.Fatalf("Client connect failed: %v", err)
//	}
//
//	// 手动读取一帧（应该是conn_ack）
//	frame, err := client.ReadFrame()
//	if err != nil {
//		t.Fatalf("Read frame failed: %v", err)
//	}
//
//	// 验证帧类型
//	if frame.Header.Ftype != ConnAck {
//		t.Errorf("Expected frame type ConnAck(%d), got %d", ConnAck, frame.Header.Ftype)
//	}
//
//	// 验证payload协议
//	if frame.MsgContent.PayloadProtocol != JsonPayload {
//		t.Errorf("Expected payload protocol JsonPayload(%d), got %d", JsonPayload, frame.MsgContent.PayloadProtocol)
//	}
//
//	fmt.Println("✓ ReadFrame test passed")
//}
//
//// TestClient_Reconnect 测试重连（应失败，因为不包含重连逻辑）
//func TestClient_Reconnect(t *testing.T) {
//	testAddr := "127.0.0.1:9093"
//
//	// 创建客户端但不启动服务器
//	client := NewClient(testAddr, "test-user-004", 1)
//	defer client.Close()
//
//	// 尝试连接应该失败
//	err := client.Connect()
//	if err == nil {
//		t.Error("Connect should fail when server is not running")
//	}
//
//	if client.IsConnected() {
//		t.Error("Client should not be connected")
//	}
//
//	fmt.Println("✓ Reconnect test passed (correctly failed as expected)")
//}
//
//// TestClient_FrameEncoding 测试帧编码解码
//func TestClient_FrameEncoding(t *testing.T) {
//	client := NewClient("dummy", "test", 1)
//
//	// 测试数据
//	var reqId [ReqIdSize]byte
//	for i := range reqId {
//		reqId[i] = byte(i)
//	}
//
//	payload := MsgContent{
//		PayloadProtocol: JsonPayload,
//		ReqId:           reqId,
//		Body:            []byte(`{"test": "data"}`),
//	}
//
//	frame := Frame{
//		Header: FrameHeader{
//			Magic:      MagicNumber,
//			Version:    Version,
//			Ftype:      Send,
//			PayloadLen: uint32(1 + ReqIdSize + len(payload.Body)),
//			CheckSum:   0,
//		},
//		MsgContent: payload,
//	}
//
//	// 编码
//	frameBytes, err := client.encodeFrame(frame)
//	if err != nil {
//		t.Fatalf("Encode frame failed: %v", err)
//	}
//
//	// 验证帧头
//	if len(frameBytes) < HeaderFrameSize {
//		t.Fatal("Encoded frame too short")
//	}
//
//	// 验证魔数
//	magic := binary.BigEndian.Uint16(frameBytes[0:2])
//	if magic != MagicNumber {
//		t.Errorf("Magic number mismatch: expected %d, got %d", MagicNumber, magic)
//	}
//
//	// 验证版本
//	if frameBytes[2] != Version {
//		t.Errorf("Version mismatch: expected %d, got %d", Version, frameBytes[2])
//	}
//
//	fmt.Println("✓ Frame encoding test passed")
//}
//
//// BenchmarkClient_EncodeFrame 性能测试
//func BenchmarkClient_EncodeFrame(b *testing.B) {
//	client := NewClient("dummy", "test", 1)
//	var reqId [ReqIdSize]byte
//
//	payload := MsgContent{
//		PayloadProtocol: JsonPayload,
//		ReqId:           reqId,
//		Body:            []byte(`{"data": "benchmark test data for encoding performance"}`),
//	}
//
//	frame := Frame{
//		Header: FrameHeader{
//			Magic:      MagicNumber,
//			Version:    Version,
//			Ftype:      Send,
//			PayloadLen: uint32(1 + ReqIdSize + len(payload.Body)),
//			CheckSum:   0,
//		},
//		MsgContent: payload,
//	}
//
//	b.ResetTimer()
//	for i := 0; i < b.N; i++ {
//		_, err := client.encodeFrame(frame)
//		if err != nil {
//			b.Fatal(err)
//		}
//	}
//}
