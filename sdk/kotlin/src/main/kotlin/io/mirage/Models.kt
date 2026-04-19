package io.mirage

// Gateway
data class HeartbeatRequest(
    val gatewayId: String,
    val version: String = "1.0.0",
    val threatLevel: UInt = 0u
)

data class HeartbeatResponse(
    val success: Boolean,
    val message: String,
    val remainingQuota: ULong,
    val defenseLevel: UInt,
    val nextHeartbeatInterval: Long
)

data class TrafficReport(
    val gatewayId: String,
    val baseTrafficBytes: ULong,
    val defenseTrafficBytes: ULong,
    val cellLevel: String = "standard"
)

data class TrafficResponse(
    val success: Boolean,
    val remainingQuota: ULong,
    val currentCostUsd: Float = 0f,
    val quotaWarning: Boolean
)

enum class ThreatType {
    UNKNOWN, ACTIVE_PROBING, JA4_SCAN, SNI_PROBE, DPI_INSPECTION, TIMING_ATTACK, REPLAY_ATTACK
}

enum class ThreatAction {
    NONE, INCREASE_DEFENSE, BLOCK_IP, SWITCH_CELL, EMERGENCY_SHUTDOWN
}

data class ThreatReport(
    val gatewayId: String,
    val threatType: ThreatType,
    val sourceIp: String,
    val severity: UInt
)

data class ThreatResponse(
    val success: Boolean,
    val action: ThreatAction,
    val newDefenseLevel: UInt
)

data class QuotaResponse(
    val success: Boolean,
    val remainingBytes: ULong,
    val totalBytes: ULong,
    val expiresAt: Long = 0
)

// Billing
data class CreateAccountResponse(
    val success: Boolean,
    val accountId: String,
    val createdAt: Long = 0
)

data class BalanceResponse(
    val success: Boolean,
    val balanceUsd: ULong,
    val totalBytes: ULong = 0u,
    val usedBytes: ULong = 0u,
    val remainingBytes: ULong
)

enum class PackageType {
    PACKAGE_10GB, PACKAGE_50GB, PACKAGE_100GB, PACKAGE_500GB, PACKAGE_1TB
}

data class PurchaseRequest(
    val accountId: String,
    val packageType: PackageType,
    val cellLevel: String = "standard",
    val quantity: UInt = 1u
)

data class PurchaseResponse(
    val success: Boolean,
    val costUsd: ULong,
    val remainingBalance: ULong = 0u,
    val quotaAdded: ULong
)

// Cell
enum class CellLevel {
    STANDARD, PLATINUM, DIAMOND
}

data class ListCellsRequest(
    val level: CellLevel? = null,
    val country: String? = null,
    val onlineOnly: Boolean = true
)

data class CellInfo(
    val cellId: String,
    val cellName: String,
    val level: CellLevel,
    val country: String,
    val region: String,
    val loadPercent: Float,
    val gatewayCount: UInt,
    val maxGateways: UInt
)

data class ListCellsResponse(
    val success: Boolean,
    val cells: List<CellInfo>
)

data class AllocateRequest(
    val userId: String,
    val gatewayId: String,
    val preferredLevel: CellLevel? = null,
    val preferredCountry: String? = null
)

data class AllocateResponse(
    val success: Boolean,
    val cellId: String,
    val connectionToken: String
)

enum class SwitchReason {
    USER_REQUEST, THREAT_DETECTED, CELL_OVERLOAD, CELL_OFFLINE
}

data class SwitchCellRequest(
    val userId: String,
    val gatewayId: String,
    val currentCellId: String,
    val targetCellId: String? = null,
    val reason: SwitchReason = SwitchReason.USER_REQUEST
)

data class SwitchCellResponse(
    val success: Boolean,
    val newCellId: String,
    val connectionToken: String
)
