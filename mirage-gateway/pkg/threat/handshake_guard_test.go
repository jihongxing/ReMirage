package threat

import (
	"net"
	"testing"
	"time"
)

func TestHandshakeGuard_WrapListener_TimeoutTriggersCallback(t *testing.T) {
	bm := NewBlacklistManager(nil, 65536)
	guard := NewHandshakeGuard(50*time.Millisecond, bm, nil)

	// 创建 TCP listener
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	wrapped := guard.WrapListener(ln)

	// 客户端连接但不发送数据 → 触发握手超时
	go func() {
		conn, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			return
		}
		// 等待超时后关闭
		time.Sleep(200 * time.Millisecond)
		conn.Close()
	}()

	conn, err := wrapped.Accept()
	if err != nil {
		t.Fatalf("accept: %v", err)
	}

	// 尝试读取 → 应超时
	buf := make([]byte, 1)
	_, err = conn.Read(buf)
	if err == nil {
		t.Fatal("应触发超时错误")
	}

	// 关闭连接 → 触发 onTimeout
	conn.Close()

	// 验证：多次超时后 IP 应被加入黑名单
	for i := 0; i < 6; i++ {
		guard.onTimeout("127.0.0.1")
	}

	entry := bm.Get("127.0.0.1/32")
	if entry == nil {
		t.Fatal("超时 6 次后应加入黑名单")
	}
}

func TestHandshakeGuard_WrapListener_NormalConnNotBlocked(t *testing.T) {
	bm := NewBlacklistManager(nil, 65536)
	guard := NewHandshakeGuard(2*time.Second, bm, nil)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	wrapped := guard.WrapListener(ln)

	// 客户端正常连接并发送数据
	go func() {
		conn, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			return
		}
		conn.Write([]byte("hello"))
		time.Sleep(100 * time.Millisecond)
		conn.Close()
	}()

	conn, err := wrapped.Accept()
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	defer conn.Close()

	buf := make([]byte, 5)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("正常连接读取不应失败: %v", err)
	}
	if string(buf[:n]) != "hello" {
		t.Fatalf("数据不匹配: %s", string(buf[:n]))
	}
}
