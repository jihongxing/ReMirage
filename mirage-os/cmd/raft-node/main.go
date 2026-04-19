// Mirage OS - Raft 节点启动程序
package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"mirage-os/pkg/raft"
)

func main() {
	// 命令行参数
	nodeID := flag.String("node-id", "", "节点 ID")
	bindAddr := flag.String("bind-addr", "0.0.0.0:7000", "绑定地址")
	dataDir := flag.String("data-dir", "./data", "数据目录")
	jurisdiction := flag.String("jurisdiction", "IS", "司法管辖区")
	peers := flag.String("peers", "", "Peer 节点列表（逗号分隔）")
	flag.Parse()
	
	// 从环境变量读取（优先级更高）
	if envNodeID := os.Getenv("NODE_ID"); envNodeID != "" {
		*nodeID = envNodeID
	}
	if envBindAddr := os.Getenv("BIND_ADDR"); envBindAddr != "" {
		*bindAddr = envBindAddr
	}
	if envJurisdiction := os.Getenv("JURISDICTION"); envJurisdiction != "" {
		*jurisdiction = envJurisdiction
	}
	if envPeers := os.Getenv("RAFT_PEERS"); envPeers != "" {
		*peers = envPeers
	}
	
	// 验证参数
	if *nodeID == "" {
		log.Fatal("❌ 必须指定节点 ID")
	}
	
	// 解析 Peer 列表
	var peerList []string
	if *peers != "" {
		peerList = strings.Split(*peers, ",")
	}
	
	log.Println("========================================")
	log.Println("🌍 Mirage OS - Raft 节点")
	log.Println("========================================")
	log.Printf("节点 ID:      %s", *nodeID)
	log.Printf("绑定地址:     %s", *bindAddr)
	log.Printf("数据目录:     %s", *dataDir)
	log.Printf("司法管辖区:   %s", *jurisdiction)
	log.Printf("Peer 节点:    %v", peerList)
	log.Println("========================================")
	
	// 创建集群配置
	config := &raft.ClusterConfig{
		NodeID:       *nodeID,
		BindAddr:     *bindAddr,
		DataDir:      *dataDir,
		Jurisdiction: raft.Jurisdiction(*jurisdiction),
		Peers:        peerList,
	}
	
	// 创建集群
	cluster, err := raft.NewCluster(config)
	if err != nil {
		log.Fatalf("❌ 创建集群失败: %v", err)
	}
	
	// 启动集群
	if err := cluster.Start(); err != nil {
		log.Fatalf("❌ 启动集群失败: %v", err)
	}
	
	// 创建地理围栏
	geoFence := raft.NewGeoFence(cluster)
	geoFence.Start()
	
	log.Println("✅ Raft 节点已启动")
	
	// 等待信号
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	
	log.Println("🛑 收到停止信号，正在关闭...")
	
	// 停止地理围栏
	geoFence.Stop()
	
	// 停止集群
	if err := cluster.Stop(); err != nil {
		log.Printf("❌ 停止集群失败: %v", err)
	}
	
	log.Println("✅ Raft 节点已停止")
}
