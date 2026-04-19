package io.mirage;

import io.grpc.ManagedChannel;
import io.grpc.Metadata;
import java.util.Collections;
import java.util.List;

public class CellService {
    private final ManagedChannel channel;
    private final Metadata metadata;

    public CellService(ManagedChannel channel, Metadata metadata) {
        this.channel = channel;
        this.metadata = metadata;
    }

    public ListCellsResponse listCells(ListCellsRequest request) {
        return new ListCellsResponse(true, Collections.emptyList());
    }

    public AllocateResponse allocateGateway(AllocateRequest request) {
        return new AllocateResponse(true, "cell-001", "token_xxx");
    }

    public SwitchCellResponse switchCell(SwitchCellRequest request) {
        return new SwitchCellResponse(true, "cell-002", "token_yyy");
    }

    // Request/Response classes
    public static class ListCellsRequest {
        private CellLevel level;
        private String country;
        private boolean onlineOnly;

        public static Builder newBuilder() { return new Builder(); }

        public static class Builder {
            private ListCellsRequest req = new ListCellsRequest();
            public Builder setLevel(CellLevel v) { req.level = v; return this; }
            public Builder setCountry(String v) { req.country = v; return this; }
            public Builder setOnlineOnly(boolean v) { req.onlineOnly = v; return this; }
            public ListCellsRequest build() { return req; }
        }
    }

    public static class ListCellsResponse {
        private boolean success;
        private List<CellInfo> cells;

        public ListCellsResponse(boolean success, List<CellInfo> cells) {
            this.success = success;
            this.cells = cells;
        }

        public boolean isSuccess() { return success; }
        public List<CellInfo> getCellsList() { return cells; }
    }

    public static class CellInfo {
        private String cellId;
        private String cellName;
        private CellLevel level;
        private String country;
        private String region;
        private float loadPercent;
        private int gatewayCount;
        private int maxGateways;

        public String getCellId() { return cellId; }
        public String getCellName() { return cellName; }
        public CellLevel getLevel() { return level; }
        public String getCountry() { return country; }
        public String getRegion() { return region; }
        public float getLoadPercent() { return loadPercent; }
        public int getGatewayCount() { return gatewayCount; }
        public int getMaxGateways() { return maxGateways; }
    }

    public static class AllocateRequest {
        private String userId;
        private String gatewayId;
        private CellLevel preferredLevel;
        private String preferredCountry;

        public static Builder newBuilder() { return new Builder(); }

        public static class Builder {
            private AllocateRequest req = new AllocateRequest();
            public Builder setUserId(String v) { req.userId = v; return this; }
            public Builder setGatewayId(String v) { req.gatewayId = v; return this; }
            public Builder setPreferredLevel(CellLevel v) { req.preferredLevel = v; return this; }
            public Builder setPreferredCountry(String v) { req.preferredCountry = v; return this; }
            public AllocateRequest build() { return req; }
        }
    }

    public static class AllocateResponse {
        private boolean success;
        private String cellId;
        private String connectionToken;

        public AllocateResponse(boolean success, String cellId, String connectionToken) {
            this.success = success;
            this.cellId = cellId;
            this.connectionToken = connectionToken;
        }

        public boolean isSuccess() { return success; }
        public String getCellId() { return cellId; }
        public String getConnectionToken() { return connectionToken; }
    }

    public static class SwitchCellRequest {
        private String userId;
        private String gatewayId;
        private String currentCellId;
        private String targetCellId;
        private SwitchReason reason;

        public static Builder newBuilder() { return new Builder(); }

        public static class Builder {
            private SwitchCellRequest req = new SwitchCellRequest();
            public Builder setUserId(String v) { req.userId = v; return this; }
            public Builder setGatewayId(String v) { req.gatewayId = v; return this; }
            public Builder setCurrentCellId(String v) { req.currentCellId = v; return this; }
            public Builder setTargetCellId(String v) { req.targetCellId = v; return this; }
            public Builder setReason(SwitchReason v) { req.reason = v; return this; }
            public SwitchCellRequest build() { return req; }
        }
    }

    public static class SwitchCellResponse {
        private boolean success;
        private String newCellId;
        private String connectionToken;

        public SwitchCellResponse(boolean success, String newCellId, String connectionToken) {
            this.success = success;
            this.newCellId = newCellId;
            this.connectionToken = connectionToken;
        }

        public boolean isSuccess() { return success; }
        public String getNewCellId() { return newCellId; }
        public String getConnectionToken() { return connectionToken; }
    }

    public enum CellLevel {
        STANDARD, PLATINUM, DIAMOND
    }

    public enum SwitchReason {
        USER_REQUEST, THREAT_DETECTED, CELL_OVERLOAD, CELL_OFFLINE
    }
}
