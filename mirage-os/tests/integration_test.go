package tests

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	goredis "github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"mirage-os/gateway-bridge/pkg/config"
	"mirage-os/gateway-bridge/pkg/dispatch"
	grpcserver "mirage-os/gateway-bridge/pkg/grpc"
	"mirage-os/gateway-bridge/pkg/intel"
	"mirage-os/gateway-bridge/pkg/quota"
	pb "mirage-proto/gen"
)

var (
	testDB  *sql.DB
	testRDB *goredis.Client
	testDSN string
)

func TestMain(m *testing.M) {
	// 使用环境变量或默认测试数据库
	testDSN = os.Getenv("TEST_DATABASE_DSN")
	if testDSN == "" {
		testDSN = "postgres://mirage:mirage_dev@localhost:5432/mirage_os_test?sslmode=disable"
	}
	redisAddr := os.Getenv("TEST_REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	var err error
	testDB, err = sql.Open("postgres", testDSN)
	if err != nil {
		log.Printf("[WARN] cannot connect to test postgres: %v (DB-dependent tests will be skipped)", err)
	} else if err := testDB.Ping(); err != nil {
		log.Printf("[WARN] cannot ping test postgres: %v (DB-dependent tests will be skipped)", err)
		testDB = nil
	}

	testRDB = goredis.NewClient(&goredis.Options{Addr: redisAddr})
	if err := testRDB.Ping(context.Background()).Err(); err != nil {
		log.Printf("[WARN] cannot connect to test redis: %v (Redis-dependent tests will be skipped)", err)
		testRDB = nil
	}

	// 初始化 schema（如果数据库可用）
	if testDB != nil {
		initSchema()
	}

	code := m.Run()

	if testDB != nil {
		testDB.Close()
	}
	if testRDB != nil {
		testRDB.Close()
	}
	os.Exit(code)
}

func initSchema() {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY, cell_id TEXT, remaining_quota FLOAT8 DEFAULT 0,
		total_consumed FLOAT8 DEFAULT 0, total_recharged FLOAT8 DEFAULT 0,
		is_active BOOLEAN DEFAULT true, created_at TIMESTAMPTZ DEFAULT NOW(), updated_at TIMESTAMPTZ DEFAULT NOW()
	);
	CREATE TABLE IF NOT EXISTS cells (
		id TEXT PRIMARY KEY, name TEXT, region TEXT, level INT DEFAULT 0,
		cost_multiplier FLOAT8 DEFAULT 1.0, max_users INT DEFAULT 100,
		created_at TIMESTAMPTZ DEFAULT NOW()
	);
	CREATE TABLE IF NOT EXISTS gateways (
		id TEXT PRIMARY KEY, cell_id TEXT, status TEXT DEFAULT 'OFFLINE',
		last_heartbeat TIMESTAMPTZ, ebpf_loaded BOOLEAN DEFAULT false,
		threat_level INT DEFAULT 0, active_connections INT DEFAULT 0,
		memory_usage_mb FLOAT8 DEFAULT 0, updated_at TIMESTAMPTZ DEFAULT NOW()
	);
	CREATE TABLE IF NOT EXISTS billing_logs (
		id SERIAL PRIMARY KEY, user_id TEXT, gateway_id TEXT,
		business_bytes BIGINT, defense_bytes BIGINT,
		business_cost FLOAT8, defense_cost FLOAT8, total_cost FLOAT8,
		period_seconds INT, created_at TIMESTAMPTZ DEFAULT NOW()
	);
	CREATE TABLE IF NOT EXISTS threat_intel (
		id SERIAL, source_ip TEXT, source_port INT DEFAULT 0,
		threat_type TEXT, severity INT DEFAULT 0, hit_count INT DEFAULT 1,
		is_banned BOOLEAN DEFAULT false, reported_by_gateway TEXT,
		last_seen TIMESTAMPTZ DEFAULT NOW(),
		UNIQUE(source_ip, threat_type)
	);`
	if _, err := testDB.Exec(schema); err != nil {
		log.Printf("[WARN] init schema: %v", err)
	}
}

type bridgeEnv struct {
	srv         *grpcserver.Server
	enforcer    *quota.Enforcer
	distributor *intel.Distributor
	dispatcher  *dispatch.StrategyDispatcher
	conn        *grpc.ClientConn
	client      pb.GatewayUplinkClient
}

func setupBridge(t *testing.T) *bridgeEnv {
	t.Helper()

	if testDB == nil || testRDB == nil {
		t.Skip("requires PostgreSQL and Redis")
	}

	// 清理测试数据
	testDB.Exec("DELETE FROM billing_logs")
	testDB.Exec("DELETE FROM threat_intel")
	testDB.Exec("DELETE FROM gateways")
	testDB.Exec("DELETE FROM users")
	testDB.Exec("DELETE FROM cells")

	cfg := config.GRPCConfig{Port: 0} // 随机端口
	enforcer := quota.NewEnforcer(testDB, config.PricingConfig{BusinessPricePerGB: 0.10, DefensePricePerGB: 0.05})
	distributor := intel.NewDistributor(testDB, testRDB, config.IntelConfig{BanThreshold: 100})
	dispatcher := dispatch.NewStrategyDispatcher(testRDB)

	// 使用固定端口进行测试
	port := 50100 + time.Now().UnixNano()%100
	cfg.Port = int(port)

	srv := grpcserver.NewServer(cfg, enforcer, distributor, dispatcher, testDB, testRDB)
	if err := srv.Start(); err != nil {
		t.Fatalf("start grpc: %v", err)
	}

	// 连接 gRPC
	addr := fmt.Sprintf("localhost:%d", port)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial grpc: %v", err)
	}

	return &bridgeEnv{
		srv: srv, enforcer: enforcer, distributor: distributor, dispatcher: dispatcher,
		conn: conn, client: pb.NewGatewayUplinkClient(conn),
	}
}

func (e *bridgeEnv) cleanup() {
	e.conn.Close()
	e.srv.Stop()
}
