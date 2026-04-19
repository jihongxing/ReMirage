import Foundation
import GRPC
import NIO

public class MirageClient {
    private let channel: GRPCChannel
    private let token: String
    
    public let gateway: GatewayService
    public let billing: BillingService
    public let cell: CellService
    
    public init(endpoint: String, token: String, useTLS: Bool = true) throws {
        self.token = token
        let group = MultiThreadedEventLoopGroup(numberOfThreads: 1)
        
        if useTLS {
            channel = try GRPCChannelPool.with(
                target: .host(endpoint),
                transportSecurity: .tls(.makeClientConfigurationBackedByNIOSSL()),
                eventLoopGroup: group
            )
        } else {
            channel = try GRPCChannelPool.with(
                target: .host(endpoint),
                transportSecurity: .plaintext,
                eventLoopGroup: group
            )
        }
        
        gateway = GatewayService(channel: channel, token: token)
        billing = BillingService(channel: channel, token: token)
        cell = CellService(channel: channel, token: token)
    }
    
    public func close() throws {
        try channel.close().wait()
    }
}

// MARK: - Gateway Service

public class GatewayService {
    private let channel: GRPCChannel
    private let token: String
    
    init(channel: GRPCChannel, token: String) {
        self.channel = channel
        self.token = token
    }
    
    public func syncHeartbeat(_ request: HeartbeatRequest) async throws -> HeartbeatResponse {
        // 实际实现需要 protobuf 生成的代码
        return HeartbeatResponse(
            success: true,
            message: "OK",
            remainingQuota: 1073741824,
            defenseLevel: 0,
            nextHeartbeatInterval: 30
        )
    }
    
    public func reportTraffic(_ request: TrafficReport) async throws -> TrafficResponse {
        return TrafficResponse(success: true, remainingQuota: 1073741824, quotaWarning: false)
    }
    
    public func reportThreat(_ request: ThreatReport) async throws -> ThreatResponse {
        return ThreatResponse(success: true, action: .increaseDefense, newDefenseLevel: 2)
    }
    
    public func getQuota(gatewayId: String, userId: String) async throws -> QuotaResponse {
        return QuotaResponse(success: true, remainingBytes: 1073741824, totalBytes: 10737418240)
    }
}

// MARK: - Billing Service

public class BillingService {
    private let channel: GRPCChannel
    private let token: String
    
    init(channel: GRPCChannel, token: String) {
        self.channel = channel
        self.token = token
    }
    
    public func createAccount(userId: String, publicKey: String) async throws -> CreateAccountResponse {
        return CreateAccountResponse(success: true, accountId: "acc-\(userId.prefix(8))")
    }
    
    public func getBalance(accountId: String) async throws -> BalanceResponse {
        return BalanceResponse(success: true, balanceUsd: 10000, remainingBytes: 9663676416)
    }
    
    public func purchaseQuota(_ request: PurchaseRequest) async throws -> PurchaseResponse {
        return PurchaseResponse(success: true, costUsd: 1000, quotaAdded: 10737418240)
    }
}

// MARK: - Cell Service

public class CellService {
    private let channel: GRPCChannel
    private let token: String
    
    init(channel: GRPCChannel, token: String) {
        self.channel = channel
        self.token = token
    }
    
    public func listCells(_ request: ListCellsRequest) async throws -> ListCellsResponse {
        return ListCellsResponse(success: true, cells: [])
    }
    
    public func allocateGateway(_ request: AllocateRequest) async throws -> AllocateResponse {
        return AllocateResponse(success: true, cellId: "cell-001", connectionToken: "token_xxx")
    }
    
    public func switchCell(_ request: SwitchCellRequest) async throws -> SwitchCellResponse {
        return SwitchCellResponse(success: true, newCellId: "cell-002", connectionToken: "token_yyy")
    }
}
