# Mirage Java SDK

## 安装

Maven:
```xml
<dependency>
    <groupId>io.mirage</groupId>
    <artifactId>mirage-sdk</artifactId>
    <version>1.0.0</version>
</dependency>
```

Gradle:
```groovy
implementation 'io.mirage:mirage-sdk:1.0.0'
```

## 快速开始

```java
import io.mirage.MirageClient;
import io.mirage.gateway.*;
import io.mirage.billing.*;
import io.mirage.cell.*;

public class Example {
    public static void main(String[] args) {
        // 初始化
        MirageClient client = MirageClient.builder()
            .endpoint("grpc.mirage.example:50847")
            .token("your_jwt_token")
            .useTls(true)
            .build();

        try {
            // 心跳同步
            HeartbeatResponse resp = client.gateway().syncHeartbeat(
                HeartbeatRequest.newBuilder()
                    .setGatewayId("gw-001")
                    .setVersion("1.0.0")
                    .setThreatLevel(0)
                    .build()
            );
            System.out.printf("剩余配额: %d bytes%n", resp.getRemainingQuota());

            // 查询余额
            BalanceResponse balance = client.billing().getBalance("acc-001");
            System.out.printf("余额: $%.2f%n", balance.getBalanceUsd() / 100.0);

            // 查询蜂窝
            ListCellsResponse cells = client.cell().listCells(
                ListCellsRequest.newBuilder()
                    .setOnlineOnly(true)
                    .build()
            );
            for (CellInfo cell : cells.getCellsList()) {
                System.out.printf("%s: %.1f%%%n", cell.getCellName(), cell.getLoadPercent());
            }
        } finally {
            client.close();
        }
    }
}
```

## WebSocket 实时推送

```java
import io.mirage.MirageWebSocket;

MirageWebSocket ws = MirageWebSocket.builder()
    .url("wss://ws.mirage.example:18443")
    .token("your_token")
    .build();

ws.on("threat", data -> {
    System.out.println("威胁告警: " + data);
});

ws.on("quota_warning", data -> {
    System.out.println("配额告警: " + data);
});

ws.connect();
```

## API 参考

### GatewayService

| 方法 | 说明 |
|------|------|
| `syncHeartbeat(req)` | 心跳同步 |
| `reportTraffic(req)` | 流量上报 |
| `reportThreat(req)` | 威胁上报 |
| `getQuota(gatewayId, userId)` | 配额查询 |

### BillingService

| 方法 | 说明 |
|------|------|
| `createAccount(userId, publicKey)` | 创建账户 |
| `deposit(req)` | 充值 |
| `getBalance(accountId)` | 查询余额 |
| `purchaseQuota(req)` | 购买流量包 |

### CellService

| 方法 | 说明 |
|------|------|
| `listCells(req)` | 查询蜂窝 |
| `allocateGateway(req)` | 分配 Gateway |
| `switchCell(req)` | 切换蜂窝 |
