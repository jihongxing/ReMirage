package io.mirage

import io.grpc.ManagedChannel
import io.grpc.ManagedChannelBuilder
import io.grpc.Metadata

class MirageClient private constructor(
    private val channel: ManagedChannel,
    private val token: String
) : AutoCloseable {
    
    val gateway = GatewayService(channel, token)
    val billing = BillingService(channel, token)
    val cell = CellService(channel, token)
    
    override fun close() {
        channel.shutdown()
    }
    
    class Builder {
        private var endpoint: String = ""
        private var token: String = ""
        private var useTls: Boolean = true
        
        fun endpoint(endpoint: String) = apply { this.endpoint = endpoint }
        fun token(token: String) = apply { this.token = token }
        fun useTls(useTls: Boolean) = apply { this.useTls = useTls }
        
        fun build(): MirageClient {
            val builder = ManagedChannelBuilder.forTarget(endpoint)
            if (useTls) builder.useTransportSecurity() else builder.usePlaintext()
            return MirageClient(builder.build(), token)
        }
    }
    
    companion object {
        fun builder() = Builder()
    }
}

class GatewayService(private val channel: ManagedChannel, private val token: String) {
    
    suspend fun syncHeartbeat(request: HeartbeatRequest): HeartbeatResponse {
        // 实际实现需要 protobuf 生成的代码
        return HeartbeatResponse(
            success = true,
            message = "OK",
            remainingQuota = 1073741824,
            defenseLevel = 0,
            nextHeartbeatInterval = 30
        )
    }
    
    suspend fun reportTraffic(request: TrafficReport): TrafficResponse {
        return TrafficResponse(success = true, remainingQuota = 1073741824, quotaWarning = false)
    }
    
    suspend fun reportThreat(request: ThreatReport): ThreatResponse {
        return ThreatResponse(success = true, action = ThreatAction.INCREASE_DEFENSE, newDefenseLevel = 2)
    }
    
    suspend fun getQuota(gatewayId: String, userId: String): QuotaResponse {
        return QuotaResponse(success = true, remainingBytes = 1073741824, totalBytes = 10737418240)
    }
}

class BillingService(private val channel: ManagedChannel, private val token: String) {
    
    suspend fun createAccount(userId: String, publicKey: String): CreateAccountResponse {
        return CreateAccountResponse(success = true, accountId = "acc-${userId.take(8)}")
    }
    
    suspend fun getBalance(accountId: String): BalanceResponse {
        return BalanceResponse(success = true, balanceUsd = 10000, remainingBytes = 9663676416)
    }
    
    suspend fun purchaseQuota(request: PurchaseRequest): PurchaseResponse {
        return PurchaseResponse(success = true, costUsd = 1000, quotaAdded = 10737418240)
    }
}

class CellService(private val channel: ManagedChannel, private val token: String) {
    
    suspend fun listCells(request: ListCellsRequest): ListCellsResponse {
        return ListCellsResponse(success = true, cells = emptyList())
    }
    
    suspend fun allocateGateway(request: AllocateRequest): AllocateResponse {
        return AllocateResponse(success = true, cellId = "cell-001", connectionToken = "token_xxx")
    }
    
    suspend fun switchCell(request: SwitchCellRequest): SwitchCellResponse {
        return SwitchCellResponse(success = true, newCellId = "cell-002", connectionToken = "token_yyy")
    }
}
