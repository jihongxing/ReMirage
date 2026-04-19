import * as grpc from '@grpc/grpc-js';
import {
  HeartbeatRequest, HeartbeatResponse,
  TrafficReport, TrafficResponse,
  ThreatReport, ThreatResponse,
  QuotaResponse,
  CreateAccountResponse, DepositRequest, DepositResponse,
  BalanceResponse, PurchaseRequest, PurchaseResponse,
  ListCellsRequest, ListCellsResponse,
  AllocateRequest, AllocateResponse,
  SwitchCellRequest, SwitchCellResponse,
} from './types';

export interface ClientOptions {
  endpoint: string;
  token: string;
  secure?: boolean;
  timeout?: number;
}

class GatewayService {
  constructor(private client: MirageClient) {}

  async syncHeartbeat(req: HeartbeatRequest): Promise<HeartbeatResponse> {
    // 实际实现需要 protobuf 生成的代码
    return {
      success: true,
      message: 'OK',
      remainingQuota: 1073741824,
      defenseLevel: 0,
      nextHeartbeatInterval: 30,
    };
  }

  async reportTraffic(req: TrafficReport): Promise<TrafficResponse> {
    return {
      success: true,
      remainingQuota: 1073741824,
      currentCostUsd: 0,
      quotaWarning: false,
    };
  }

  async reportThreat(req: ThreatReport): Promise<ThreatResponse> {
    return {
      success: true,
      action: 1,
      newDefenseLevel: 2,
    };
  }

  async getQuota(gatewayId: string, userId: string): Promise<QuotaResponse> {
    return {
      success: true,
      remainingBytes: 1073741824,
      totalBytes: 10737418240,
      expiresAt: Math.floor(Date.now() / 1000) + 86400 * 30,
    };
  }
}

class BillingService {
  constructor(private client: MirageClient) {}

  async createAccount(userId: string, publicKey: string): Promise<CreateAccountResponse> {
    return {
      success: true,
      accountId: `acc-${userId.slice(0, 8)}`,
      createdAt: Math.floor(Date.now() / 1000),
    };
  }

  async deposit(req: DepositRequest): Promise<DepositResponse> {
    return {
      success: true,
      balanceUsd: 10000,
      exchangeRate: 150.0,
      confirmedAt: Math.floor(Date.now() / 1000),
    };
  }

  async getBalance(accountId: string): Promise<BalanceResponse> {
    return {
      success: true,
      balanceUsd: 10000,
      totalBytes: 10737418240,
      usedBytes: 1073741824,
      remainingBytes: 9663676416,
    };
  }

  async purchaseQuota(req: PurchaseRequest): Promise<PurchaseResponse> {
    return {
      success: true,
      costUsd: 1000,
      remainingBalance: 9000,
      quotaAdded: 10737418240,
    };
  }
}

class CellService {
  constructor(private client: MirageClient) {}

  async listCells(req: ListCellsRequest): Promise<ListCellsResponse> {
    return { success: true, cells: [] };
  }

  async allocateGateway(req: AllocateRequest): Promise<AllocateResponse> {
    return {
      success: true,
      cellId: 'cell-001',
      connectionToken: 'token_xxx',
    };
  }

  async switchCell(req: SwitchCellRequest): Promise<SwitchCellResponse> {
    return {
      success: true,
      newCellId: 'cell-002',
      connectionToken: 'token_yyy',
    };
  }
}

export class MirageClient {
  private endpoint: string;
  private token: string;
  private metadata: grpc.Metadata;

  public gateway: GatewayService;
  public billing: BillingService;
  public cell: CellService;

  constructor(options: ClientOptions) {
    this.endpoint = options.endpoint;
    this.token = options.token;
    this.metadata = new grpc.Metadata();
    this.metadata.set('authorization', `Bearer ${this.token}`);

    this.gateway = new GatewayService(this);
    this.billing = new BillingService(this);
    this.cell = new CellService(this);
  }

  close(): void {
    // 关闭连接
  }
}
