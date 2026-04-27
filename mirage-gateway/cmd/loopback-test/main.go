// loopback-test: 本地冒烟测试用 QUIC Datagram 回显服务器
// 接收 Phantom Client 发来的 Datagram，打印 IP 包信息
package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"fmt"
	"log"
	"math/big"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/quic-go/quic-go"
)

func main() {
	listenAddr := "0.0.0.0:443"
	if len(os.Args) > 1 {
		listenAddr = os.Args[1]
	}

	log.Printf("🧪 Loopback Test Server: %s (QUIC Datagram)", listenAddr)

	tlsCert := generateTLSCert()
	tlsConf := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		NextProtos:   []string{"mirage-gtunnel"},
	}
	quicConf := &quic.Config{
		EnableDatagrams: true,
	}

	listener, err := quic.ListenAddr(listenAddr, tlsConf, quicConf)
	if err != nil {
		log.Fatalf("❌ 监听失败: %v", err)
	}
	defer listener.Close()
	log.Println("✅ 等待 Phantom Client 连接...")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch
		cancel()
	}()

	for {
		conn, err := listener.Accept(ctx)
		if err != nil {
			break
		}
		log.Printf("🔗 客户端: %s", "***")
		go serve(ctx, conn)
	}
}

func serve(ctx context.Context, conn *quic.Conn) {
	var n uint64
	for {
		msg, err := conn.ReceiveDatagram(ctx)
		if err != nil {
			log.Printf("🔌 断开: %v", err)
			return
		}
		n++
		printPacket(n, msg)
	}
}

func printPacket(seq uint64, msg []byte) {
	if len(msg) < 1 {
		log.Printf("📦 #%d: empty", seq)
		return
	}

	// 检测是否为 IPv4 包（版本号 = 4）
	if (msg[0]>>4) == 4 && len(msg) >= 20 {
		proto := msg[9]
		totalLen := binary.BigEndian.Uint16(msg[2:4])
		src := fmt.Sprintf("%d.%d.%d.%d", msg[12], msg[13], msg[14], msg[15])
		dst := fmt.Sprintf("%d.%d.%d.%d", msg[16], msg[17], msg[18], msg[19])
		name := protoName(proto)
		log.Printf("📦 #%d: %s %s → %s (%d bytes)", seq, name, src, dst, totalLen)
	} else {
		// 加密数据（正常模式）
		log.Printf("📦 #%d: encrypted datagram (%d bytes)", seq, len(msg))
	}
}

func protoName(p uint8) string {
	switch p {
	case 1:
		return "ICMP"
	case 6:
		return "TCP"
	case 17:
		return "UDP"
	default:
		return fmt.Sprintf("PROTO_%d", p)
	}
}

func generateTLSCert() tls.Certificate {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     []string{"localhost"},
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	return tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
	}
}
