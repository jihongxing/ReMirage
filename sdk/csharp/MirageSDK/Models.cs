using System.Collections.Generic;

namespace MirageSDK
{
    // Gateway
    public class HeartbeatRequest
    {
        public string GatewayId { get; set; }
        public string Version { get; set; }
        public uint ThreatLevel { get; set; }
    }

    public class HeartbeatResponse
    {
        public bool Success { get; set; }
        public string Message { get; set; }
        public ulong RemainingQuota { get; set; }
        public uint DefenseLevel { get; set; }
        public long NextHeartbeatInterval { get; set; }
    }

    public class TrafficReport
    {
        public string GatewayId { get; set; }
        public ulong BaseTrafficBytes { get; set; }
        public ulong DefenseTrafficBytes { get; set; }
        public string CellLevel { get; set; }
    }

    public class TrafficResponse
    {
        public bool Success { get; set; }
        public ulong RemainingQuota { get; set; }
        public float CurrentCostUsd { get; set; }
        public bool QuotaWarning { get; set; }
    }

    public class ThreatReport
    {
        public string GatewayId { get; set; }
        public ThreatType ThreatType { get; set; }
        public string SourceIp { get; set; }
        public uint Severity { get; set; }
    }

    public enum ThreatType { Unknown, ActiveProbing, Ja4Scan, SniProbe, DpiInspection, TimingAttack, ReplayAttack }
    public enum ThreatAction { None, IncreaseDefense, BlockIp, SwitchCell, EmergencyShutdown }

    public class ThreatResponse
    {
        public bool Success { get; set; }
        public ThreatAction Action { get; set; }
        public uint NewDefenseLevel { get; set; }
    }

    public class QuotaResponse
    {
        public bool Success { get; set; }
        public ulong RemainingBytes { get; set; }
        public ulong TotalBytes { get; set; }
        public long ExpiresAt { get; set; }
    }

    // Billing
    public class CreateAccountResponse
    {
        public bool Success { get; set; }
        public string AccountId { get; set; }
        public long CreatedAt { get; set; }
    }

    public class BalanceResponse
    {
        public bool Success { get; set; }
        public ulong BalanceUsd { get; set; }
        public ulong TotalBytes { get; set; }
        public ulong UsedBytes { get; set; }
        public ulong RemainingBytes { get; set; }
    }

    public class PurchaseRequest
    {
        public string AccountId { get; set; }
        public PackageType PackageType { get; set; }
        public string CellLevel { get; set; }
        public uint Quantity { get; set; }
    }

    public enum PackageType { Package10Gb = 1, Package50Gb, Package100Gb, Package500Gb, Package1Tb }

    public class PurchaseResponse
    {
        public bool Success { get; set; }
        public ulong CostUsd { get; set; }
        public ulong RemainingBalance { get; set; }
        public ulong QuotaAdded { get; set; }
    }

    // Cell
    public class ListCellsRequest
    {
        public CellLevel? Level { get; set; }
        public string Country { get; set; }
        public bool OnlineOnly { get; set; }
    }

    public enum CellLevel { Standard = 1, Platinum, Diamond }

    public class CellInfo
    {
        public string CellId { get; set; }
        public string CellName { get; set; }
        public CellLevel Level { get; set; }
        public string Country { get; set; }
        public string Region { get; set; }
        public float LoadPercent { get; set; }
        public uint GatewayCount { get; set; }
        public uint MaxGateways { get; set; }
    }

    public class ListCellsResponse
    {
        public bool Success { get; set; }
        public List<CellInfo> Cells { get; set; }
    }

    public class AllocateRequest
    {
        public string UserId { get; set; }
        public string GatewayId { get; set; }
        public CellLevel? PreferredLevel { get; set; }
        public string PreferredCountry { get; set; }
    }

    public class AllocateResponse
    {
        public bool Success { get; set; }
        public string CellId { get; set; }
        public string ConnectionToken { get; set; }
    }

    public class SwitchCellRequest
    {
        public string UserId { get; set; }
        public string GatewayId { get; set; }
        public string CurrentCellId { get; set; }
        public string TargetCellId { get; set; }
        public SwitchReason Reason { get; set; }
    }

    public enum SwitchReason { UserRequest = 1, ThreatDetected, CellOverload, CellOffline }

    public class SwitchCellResponse
    {
        public bool Success { get; set; }
        public string NewCellId { get; set; }
        public string ConnectionToken { get; set; }
    }
}
