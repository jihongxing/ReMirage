# Referencia API

## Resumen de Servicios

| Servicio | Puerto | Protocolo | Propósito |
|----------|--------|-----------|-----------|
| Gateway | 50847 | gRPC | Heartbeat/Tráfico/Amenazas |
| Cell | 50847 | gRPC | Gestión de celdas |
| Billing | 50847 | gRPC | Facturación/Depósito |
| WebSocket | 18443 | WSS | Push en tiempo real |

---

## GatewayService

### SyncHeartbeat - Sincronización de Heartbeat

**Parámetros de Solicitud**

| Campo | Tipo | Requerido | Descripción |
|-------|------|-----------|-------------|
| gateway_id | string | ✅ | Identificador único del Gateway |
| version | string | ✅ | Versión del Gateway |
| threat_level | uint32 | ❌ | Nivel de amenaza actual (0-5) |

**Parámetros de Respuesta**

| Campo | Tipo | Descripción |
|-------|------|-------------|
| success | bool | Estado de éxito |
| remaining_quota | uint64 | Cuota restante (bytes) |
| next_heartbeat_interval | int64 | Intervalo del próximo heartbeat (segundos) |

**Frecuencia**: 30 segundos

---

### ReportThreat - Reporte de Amenazas

**Tipos de Amenaza (ThreatType)**

| Valor | Descripción |
|-------|-------------|
| ACTIVE_PROBING | Sondeo activo |
| JA4_SCAN | Escaneo de huella JA4 |
| SNI_PROBE | Sonda SNI |
| DPI_INSPECTION | Inspección profunda DPI |
| TIMING_ATTACK | Ataque de temporización |
| REPLAY_ATTACK | Ataque de repetición |

---

## BillingService

### PurchaseQuota - Comprar Cuota

**Tipos de Paquete (PackageType)**

| Valor | Capacidad |
|-------|-----------|
| PACKAGE_10GB | 10 GB |
| PACKAGE_50GB | 50 GB |
| PACKAGE_100GB | 100 GB |
| PACKAGE_500GB | 500 GB |
| PACKAGE_1TB | 1 TB |

---

## CellService

### Niveles de Celda (CellLevel)

| Valor | Descripción | Multiplicador de Costo |
|-------|-------------|------------------------|
| STANDARD | Celda estándar | 1.0x |
| PLATINUM | Celda platino | 1.5x |
| DIAMOND | Celda diamante | 2.0x |
