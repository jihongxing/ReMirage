# Mirage Go SDK

## 安装

```bash
go get github.com/mirage/sdk-go
```

## 快速开始

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    mirage "github.com/mirage/sdk-go"
)

func main() {
    // 初始化客户端
    client, err := mirage.NewClient(
        "grpc.mirage.example:50847",
        mirage.WithToken("your_jwt_token"),
        mirage.WithTLS(true),
    )
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()
    
    ctx := context.Background()
    
    // 心跳同步
    resp, err := client.Gateway.SyncHeartbeat(ctx, &mirage.HeartbeatRequest{
        GatewayID:    "gw-001",
        Version:      "1.0.0",
        ThreatLevel:  0,
    })
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("剩余配额: %d bytes\n", resp.RemainingQuota)
    
    // 查询余额
    balance, err := client.Billing.GetBalance(ctx, "acc-001")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("余额: $%.2f\n", float64(balance.BalanceUSD)/100)
    
    // 查询蜂窝
    cells, err := client.Cell.ListCells(ctx, &mirage.ListCellsRequest{
        OnlineOnly: true,
    })
    if err != nil {
        log.Fatal(err)
    }
    for _, cell := range cells.Cells {
        fmt.Printf("%s: %.1f%%\n", cell.CellName, cell.LoadPercent)
    }
}
```

## WebSocket 实时推送

```go
package main

import (
    "fmt"
    "log"
    
    mirage "github.com/mirage/sdk-go"
)

func main() {
    ws, err := mirage.NewWebSocket(
        "wss://ws.mirage.example:18443",
        mirage.WithToken("your_token"),
    )
    if err != nil {
        log.Fatal(err)
    }
    
    ws.On("threat", func(data map[string]interface{}) {
        fmt.Printf("威胁告警: %v\n", data)
    })
    
    ws.On("quota_warning", func(data map[string]interface{}) {
        fmt.Printf("配额告警: %v\n", data)
    })
    
    // 阻塞连接
    ws.Connect()
}
```

## API 参考

### GatewayService

```go
SyncHeartbeat(ctx, req *HeartbeatRequest) (*HeartbeatResponse, error)
ReportTraffic(ctx, req *TrafficReport) (*TrafficResponse, error)
ReportThreat(ctx, req *ThreatReport) (*ThreatResponse, error)
GetQuota(ctx, gatewayID, userID string) (*QuotaResponse, error)
```

### BillingService

```go
CreateAccount(ctx, userID, publicKey string) (*CreateAccountResponse, error)
Deposit(ctx, req *DepositRequest) (*DepositResponse, error)
GetBalance(ctx, accountID string) (*BalanceResponse, error)
PurchaseQuota(ctx, req *PurchaseRequest) (*PurchaseResponse, error)
```

### CellService

```go
ListCells(ctx, req *ListCellsRequest) (*ListCellsResponse, error)
AllocateGateway(ctx, req *AllocateRequest) (*AllocateResponse, error)
SwitchCell(ctx, req *SwitchCellRequest) (*SwitchCellResponse, error)
```
