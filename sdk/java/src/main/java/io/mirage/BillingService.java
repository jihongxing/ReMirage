package io.mirage;

import io.grpc.ManagedChannel;
import io.grpc.Metadata;

public class BillingService {
    private final ManagedChannel channel;
    private final Metadata metadata;

    public BillingService(ManagedChannel channel, Metadata metadata) {
        this.channel = channel;
        this.metadata = metadata;
    }

    public CreateAccountResponse createAccount(String userId, String publicKey) {
        return new CreateAccountResponse(true, "acc-" + userId.substring(0, 8), 
            System.currentTimeMillis() / 1000);
    }

    public DepositResponse deposit(DepositRequest request) {
        return new DepositResponse(true, 10000, 150.0f, System.currentTimeMillis() / 1000);
    }

    public BalanceResponse getBalance(String accountId) {
        return new BalanceResponse(true, 10000, 10737418240L, 1073741824L, 9663676416L);
    }

    public PurchaseResponse purchaseQuota(PurchaseRequest request) {
        return new PurchaseResponse(true, 1000, 9000, 10737418240L);
    }

    // Request/Response classes
    public static class CreateAccountResponse {
        private boolean success;
        private String accountId;
        private long createdAt;

        public CreateAccountResponse(boolean success, String accountId, long createdAt) {
            this.success = success;
            this.accountId = accountId;
            this.createdAt = createdAt;
        }

        public boolean isSuccess() { return success; }
        public String getAccountId() { return accountId; }
        public long getCreatedAt() { return createdAt; }
    }

    public static class DepositRequest {
        private String accountId;
        private String txHash;
        private long amountXmr;

        public static Builder newBuilder() { return new Builder(); }

        public static class Builder {
            private DepositRequest req = new DepositRequest();
            public Builder setAccountId(String v) { req.accountId = v; return this; }
            public Builder setTxHash(String v) { req.txHash = v; return this; }
            public Builder setAmountXmr(long v) { req.amountXmr = v; return this; }
            public DepositRequest build() { return req; }
        }
    }

    public static class DepositResponse {
        private boolean success;
        private long balanceUsd;
        private float exchangeRate;
        private long confirmedAt;

        public DepositResponse(boolean success, long balanceUsd, float exchangeRate, long confirmedAt) {
            this.success = success;
            this.balanceUsd = balanceUsd;
            this.exchangeRate = exchangeRate;
            this.confirmedAt = confirmedAt;
        }

        public boolean isSuccess() { return success; }
        public long getBalanceUsd() { return balanceUsd; }
        public float getExchangeRate() { return exchangeRate; }
        public long getConfirmedAt() { return confirmedAt; }
    }

    public static class BalanceResponse {
        private boolean success;
        private long balanceUsd;
        private long totalBytes;
        private long usedBytes;
        private long remainingBytes;

        public BalanceResponse(boolean success, long balanceUsd, long totalBytes, 
                               long usedBytes, long remainingBytes) {
            this.success = success;
            this.balanceUsd = balanceUsd;
            this.totalBytes = totalBytes;
            this.usedBytes = usedBytes;
            this.remainingBytes = remainingBytes;
        }

        public boolean isSuccess() { return success; }
        public long getBalanceUsd() { return balanceUsd; }
        public long getTotalBytes() { return totalBytes; }
        public long getUsedBytes() { return usedBytes; }
        public long getRemainingBytes() { return remainingBytes; }
    }

    public static class PurchaseRequest {
        private String accountId;
        private PackageType packageType;
        private String cellLevel;
        private int quantity;

        public static Builder newBuilder() { return new Builder(); }

        public static class Builder {
            private PurchaseRequest req = new PurchaseRequest();
            public Builder setAccountId(String v) { req.accountId = v; return this; }
            public Builder setPackageType(PackageType v) { req.packageType = v; return this; }
            public Builder setCellLevel(String v) { req.cellLevel = v; return this; }
            public Builder setQuantity(int v) { req.quantity = v; return this; }
            public PurchaseRequest build() { return req; }
        }
    }

    public static class PurchaseResponse {
        private boolean success;
        private long costUsd;
        private long remainingBalance;
        private long quotaAdded;

        public PurchaseResponse(boolean success, long costUsd, long remainingBalance, long quotaAdded) {
            this.success = success;
            this.costUsd = costUsd;
            this.remainingBalance = remainingBalance;
            this.quotaAdded = quotaAdded;
        }

        public boolean isSuccess() { return success; }
        public long getCostUsd() { return costUsd; }
        public long getRemainingBalance() { return remainingBalance; }
        public long getQuotaAdded() { return quotaAdded; }
    }

    public enum PackageType {
        PACKAGE_10GB, PACKAGE_50GB, PACKAGE_100GB, PACKAGE_500GB, PACKAGE_1TB
    }
}
