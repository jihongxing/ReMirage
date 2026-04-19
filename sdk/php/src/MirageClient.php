<?php

namespace Mirage;

class MirageClient
{
    private string $endpoint;
    private string $token;
    public GatewayService $gateway;
    public BillingService $billing;
    public CellService $cell;

    public function __construct(string $endpoint, string $token)
    {
        $this->endpoint = $endpoint;
        $this->token = $token;
        $this->gateway = new GatewayService($this);
        $this->billing = new BillingService($this);
        $this->cell = new CellService($this);
    }

    public function getEndpoint(): string { return $this->endpoint; }
    public function getToken(): string { return $this->token; }
}

class GatewayService
{
    private MirageClient $client;

    public function __construct(MirageClient $client)
    {
        $this->client = $client;
    }

    public function syncHeartbeat(array $request): array
    {
        // 实际实现需要 gRPC PHP 扩展
        return [
            'success' => true,
            'message' => 'OK',
            'remaining_quota' => 1073741824,
            'defense_level' => 0,
            'next_heartbeat_interval' => 30,
        ];
    }

    public function reportTraffic(array $request): array
    {
        return ['success' => true, 'remaining_quota' => 1073741824, 'quota_warning' => false];
    }

    public function reportThreat(array $request): array
    {
        return ['success' => true, 'action' => 'INCREASE_DEFENSE', 'new_defense_level' => 2];
    }

    public function getQuota(string $gatewayId, string $userId): array
    {
        return ['success' => true, 'remaining_bytes' => 1073741824, 'total_bytes' => 10737418240];
    }
}

class BillingService
{
    private MirageClient $client;

    public function __construct(MirageClient $client)
    {
        $this->client = $client;
    }

    public function createAccount(string $userId, string $publicKey): array
    {
        return ['success' => true, 'account_id' => 'acc-' . substr($userId, 0, 8)];
    }

    public function getBalance(string $accountId): array
    {
        return ['success' => true, 'balance_usd' => 10000, 'remaining_bytes' => 9663676416];
    }

    public function purchaseQuota(array $request): array
    {
        return ['success' => true, 'cost_usd' => 1000, 'quota_added' => 10737418240];
    }
}

class CellService
{
    private MirageClient $client;

    public function __construct(MirageClient $client)
    {
        $this->client = $client;
    }

    public function listCells(array $request = []): array
    {
        return ['success' => true, 'cells' => []];
    }

    public function allocateGateway(array $request): array
    {
        return ['success' => true, 'cell_id' => 'cell-001', 'connection_token' => 'token_xxx'];
    }

    public function switchCell(array $request): array
    {
        return ['success' => true, 'new_cell_id' => 'cell-002', 'connection_token' => 'token_yyy'];
    }
}
