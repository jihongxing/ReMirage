import Foundation

// MARK: - Gateway Models

public struct HeartbeatRequest {
    public let gatewayId: String
    public let version: String
    public let threatLevel: UInt32
    
    public init(gatewayId: String, version: String = "1.0.0", threatLevel: UInt32 = 0) {
        self.gatewayId = gatewayId
        self.version = version
        self.threatLevel = threatLevel
    }
}

public struct HeartbeatResponse {
    public let success: Bool
    public let message: String
    public let remainingQuota: UInt64
    public let defenseLevel: UInt32
    public let nextHeartbeatInterval: Int64
}

public struct TrafficReport {
    public let gatewayId: String
    public let baseTrafficBytes: UInt64
    public let defenseTrafficBytes: UInt64
    public let cellLevel: String
}

public struct TrafficResponse {
    public let success: Bool
    public let remainingQuota: UInt64
    public let quotaWarning: Bool
}

public enum ThreatType: Int {
    case unknown = 0, activeProbing, ja4Scan, sniProbe, dpiInspection, timingAttack, replayAttack
}

public enum ThreatAction: Int {
    case none = 0, increaseDefense, blockIp, switchCell, emergencyShutdown
}

public struct ThreatReport {
    public let gatewayId: String
    public let threatType: ThreatType
    public let sourceIp: String
    public let severity: UInt32
}

public struct ThreatResponse {
    public let success: Bool
    public let action: ThreatAction
    public let newDefenseLevel: UInt32
}

public struct QuotaResponse {
    public let success: Bool
    public let remainingBytes: UInt64
    public let totalBytes: UInt64
}

// MARK: - Billing Models

public struct CreateAccountResponse {
    public let success: Bool
    public let accountId: String
}

public struct BalanceResponse {
    public let success: Bool
    public let balanceUsd: UInt64
    public let remainingBytes: UInt64
}

public enum PackageType: Int {
    case package10Gb = 1, package50Gb, package100Gb, package500Gb, package1Tb
}

public struct PurchaseRequest {
    public let accountId: String
    public let packageType: PackageType
    public let cellLevel: String
    public let quantity: UInt32
}

public struct PurchaseResponse {
    public let success: Bool
    public let costUsd: UInt64
    public let quotaAdded: UInt64
}

// MARK: - Cell Models

public enum CellLevel: Int {
    case standard = 1, platinum, diamond
}

public struct ListCellsRequest {
    public let level: CellLevel?
    public let country: String?
    public let onlineOnly: Bool
    
    public init(level: CellLevel? = nil, country: String? = nil, onlineOnly: Bool = true) {
        self.level = level
        self.country = country
        self.onlineOnly = onlineOnly
    }
}

public struct CellInfo {
    public let cellId: String
    public let cellName: String
    public let level: CellLevel
    public let country: String
    public let region: String
    public let loadPercent: Float
}

public struct ListCellsResponse {
    public let success: Bool
    public let cells: [CellInfo]
}

public struct AllocateRequest {
    public let userId: String
    public let gatewayId: String
    public let preferredLevel: CellLevel?
    public let preferredCountry: String?
}

public struct AllocateResponse {
    public let success: Bool
    public let cellId: String
    public let connectionToken: String
}

public enum SwitchReason: Int {
    case userRequest = 1, threatDetected, cellOverload, cellOffline
}

public struct SwitchCellRequest {
    public let userId: String
    public let gatewayId: String
    public let currentCellId: String
    public let targetCellId: String?
    public let reason: SwitchReason
}

public struct SwitchCellResponse {
    public let success: Bool
    public let newCellId: String
    public let connectionToken: String
}
