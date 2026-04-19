package io.mirage;

import io.grpc.ManagedChannel;
import io.grpc.Metadata;

public class GatewayService {
    private final ManagedChannel channel;
    private final Metadata metadata;

    public GatewayService(ManagedChannel channel, Metadata metadata) {
        this.channel = channel;
        this.metadata = metadata;
    }

    public HeartbeatResponse syncHeartbeat(HeartbeatRequest request) {
        // 实际实现需要 protobuf 生成的代码
        return new HeartbeatResponse(true, "OK", 1073741824L, 0, 30);
    }

    public TrafficResponse reportTraffic(TrafficReport request) {
        return new TrafficResponse(true, 1073741824L, 0.0f, false);
    }

    public ThreatResponse reportThreat(ThreatReport request) {
        return new ThreatResponse(true, ThreatAction.INCREASE_DEFENSE, 2);
    }

    public QuotaResponse getQuota(String gatewayId, String userId) {
        return new QuotaResponse(true, 1073741824L, 10737418240L, 
            System.currentTimeMillis() / 1000 + 86400 * 30);
    }

    // Request/Response classes
    public static class HeartbeatRequest {
        private String gatewayId;
        private String version;
        private int threatLevel;

        public static Builder newBuilder() { return new Builder(); }

        public static class Builder {
            private HeartbeatRequest req = new HeartbeatRequest();
            public Builder setGatewayId(String v) { req.gatewayId = v; return this; }
            public Builder setVersion(String v) { req.version = v; return this; }
            public Builder setThreatLevel(int v) { req.threatLevel = v; return this; }
            public HeartbeatRequest build() { return req; }
        }
    }

    public static class HeartbeatResponse {
        private boolean success;
        private String message;
        private long remainingQuota;
        private int defenseLevel;
        private int nextHeartbeatInterval;

        public HeartbeatResponse(boolean success, String message, long remainingQuota, 
                                  int defenseLevel, int nextHeartbeatInterval) {
            this.success = success;
            this.message = message;
            this.remainingQuota = remainingQuota;
            this.defenseLevel = defenseLevel;
            this.nextHeartbeatInterval = nextHeartbeatInterval;
        }

        public boolean isSuccess() { return success; }
        public String getMessage() { return message; }
        public long getRemainingQuota() { return remainingQuota; }
        public int getDefenseLevel() { return defenseLevel; }
        public int getNextHeartbeatInterval() { return nextHeartbeatInterval; }
    }

    public static class TrafficReport {
        private String gatewayId;
        private long baseTrafficBytes;
        private long defenseTrafficBytes;
        private String cellLevel;

        public static Builder newBuilder() { return new Builder(); }

        public static class Builder {
            private TrafficReport req = new TrafficReport();
            public Builder setGatewayId(String v) { req.gatewayId = v; return this; }
            public Builder setBaseTrafficBytes(long v) { req.baseTrafficBytes = v; return this; }
            public Builder setDefenseTrafficBytes(long v) { req.defenseTrafficBytes = v; return this; }
            public Builder setCellLevel(String v) { req.cellLevel = v; return this; }
            public TrafficReport build() { return req; }
        }
    }

    public static class TrafficResponse {
        private boolean success;
        private long remainingQuota;
        private float currentCostUsd;
        private boolean quotaWarning;

        public TrafficResponse(boolean success, long remainingQuota, float currentCostUsd, boolean quotaWarning) {
            this.success = success;
            this.remainingQuota = remainingQuota;
            this.currentCostUsd = currentCostUsd;
            this.quotaWarning = quotaWarning;
        }

        public boolean isSuccess() { return success; }
        public long getRemainingQuota() { return remainingQuota; }
        public float getCurrentCostUsd() { return currentCostUsd; }
        public boolean isQuotaWarning() { return quotaWarning; }
    }

    public static class ThreatReport {
        private String gatewayId;
        private ThreatType threatType;
        private String sourceIp;
        private int severity;

        public static Builder newBuilder() { return new Builder(); }

        public static class Builder {
            private ThreatReport req = new ThreatReport();
            public Builder setGatewayId(String v) { req.gatewayId = v; return this; }
            public Builder setThreatType(ThreatType v) { req.threatType = v; return this; }
            public Builder setSourceIp(String v) { req.sourceIp = v; return this; }
            public Builder setSeverity(int v) { req.severity = v; return this; }
            public ThreatReport build() { return req; }
        }
    }

    public static class ThreatResponse {
        private boolean success;
        private ThreatAction action;
        private int newDefenseLevel;

        public ThreatResponse(boolean success, ThreatAction action, int newDefenseLevel) {
            this.success = success;
            this.action = action;
            this.newDefenseLevel = newDefenseLevel;
        }

        public boolean isSuccess() { return success; }
        public ThreatAction getAction() { return action; }
        public int getNewDefenseLevel() { return newDefenseLevel; }
    }

    public static class QuotaResponse {
        private boolean success;
        private long remainingBytes;
        private long totalBytes;
        private long expiresAt;

        public QuotaResponse(boolean success, long remainingBytes, long totalBytes, long expiresAt) {
            this.success = success;
            this.remainingBytes = remainingBytes;
            this.totalBytes = totalBytes;
            this.expiresAt = expiresAt;
        }

        public boolean isSuccess() { return success; }
        public long getRemainingBytes() { return remainingBytes; }
        public long getTotalBytes() { return totalBytes; }
        public long getExpiresAt() { return expiresAt; }
    }

    public enum ThreatType {
        UNKNOWN, ACTIVE_PROBING, JA4_SCAN, SNI_PROBE, DPI_INSPECTION, TIMING_ATTACK, REPLAY_ATTACK
    }

    public enum ThreatAction {
        NONE, INCREASE_DEFENSE, BLOCK_IP, SWITCH_CELL, EMERGENCY_SHUTDOWN
    }
}
