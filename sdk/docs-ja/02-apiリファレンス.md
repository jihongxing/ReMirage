# APIリファレンス

## サービス概要

| サービス | ポート | プロトコル | 用途 |
|----------|--------|------------|------|
| Gateway | 50847 | gRPC | ハートビート/トラフィック/脅威レポート |
| Cell | 50847 | gRPC | セル管理 |
| Billing | 50847 | gRPC | 課金/入金 |
| WebSocket | 18443 | WSS | リアルタイムプッシュ |

---

## GatewayService

### SyncHeartbeat - ハートビート同期

**リクエストパラメータ**

| フィールド | 型 | 必須 | 説明 |
|------------|-----|------|------|
| gateway_id | string | ✅ | Gateway一意識別子 |
| version | string | ✅ | Gatewayバージョン |
| threat_level | uint32 | ❌ | 現在の脅威レベル (0-5) |

**レスポンスパラメータ**

| フィールド | 型 | 説明 |
|------------|-----|------|
| success | bool | 成功ステータス |
| remaining_quota | uint64 | 残りクォータ（バイト） |
| next_heartbeat_interval | int64 | 次のハートビート間隔（秒） |

---

### ReportThreat - 脅威レポート

**脅威タイプ (ThreatType)**

| 値 | 説明 |
|----|------|
| ACTIVE_PROBING | アクティブプロービング |
| JA4_SCAN | JA4フィンガープリントスキャン |
| SNI_PROBE | SNIプローブ |
| DPI_INSPECTION | DPI深層検査 |
| TIMING_ATTACK | タイミング攻撃 |
| REPLAY_ATTACK | リプレイ攻撃 |

---

## BillingService

### パッケージタイプ (PackageType)

| 値 | 容量 |
|----|------|
| PACKAGE_10GB | 10 GB |
| PACKAGE_50GB | 50 GB |
| PACKAGE_100GB | 100 GB |
| PACKAGE_500GB | 500 GB |
| PACKAGE_1TB | 1 TB |

---

## CellService

### セルレベル (CellLevel)

| 値 | 説明 | コスト倍率 |
|----|------|-----------|
| STANDARD | スタンダードセル | 1.0x |
| PLATINUM | プラチナセル | 1.5x |
| DIAMOND | ダイヤモンドセル | 2.0x |
