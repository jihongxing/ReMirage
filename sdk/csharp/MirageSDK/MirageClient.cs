using System;
using System.Threading.Tasks;
using Grpc.Core;
using Grpc.Net.Client;

namespace MirageSDK
{
    public class MirageClient : IDisposable
    {
        private readonly GrpcChannel _channel;
        private readonly string _token;
        
        public GatewayService Gateway { get; }
        public BillingService Billing { get; }
        public CellService Cell { get; }

        public MirageClient(string endpoint, string token, bool useTls = true)
        {
            _token = token;
            var uri = useTls ? $"https://{endpoint}" : $"http://{endpoint}";
            _channel = GrpcChannel.ForAddress(uri);
            
            Gateway = new GatewayService(_channel, _token);
            Billing = new BillingService(_channel, _token);
            Cell = new CellService(_channel, _token);
        }

        public void Dispose()
        {
            _channel?.Dispose();
        }
    }

    public class GatewayService
    {
        private readonly GrpcChannel _channel;
        private readonly Metadata _metadata;

        public GatewayService(GrpcChannel channel, string token)
        {
            _channel = channel;
            _metadata = new Metadata { { "authorization", $"Bearer {token}" } };
        }

        public async Task<HeartbeatResponse> SyncHeartbeatAsync(HeartbeatRequest request)
        {
            // 实际实现需要 protobuf 生成的代码
            return new HeartbeatResponse
            {
                Success = true,
                Message = "OK",
                RemainingQuota = 1073741824,
                DefenseLevel = 0,
                NextHeartbeatInterval = 30
            };
        }

        public async Task<TrafficResponse> ReportTrafficAsync(TrafficReport request)
        {
            return new TrafficResponse { Success = true, RemainingQuota = 1073741824 };
        }

        public async Task<ThreatResponse> ReportThreatAsync(ThreatReport request)
        {
            return new ThreatResponse { Success = true, Action = ThreatAction.IncreaseDefense, NewDefenseLevel = 2 };
        }

        public async Task<QuotaResponse> GetQuotaAsync(string gatewayId, string userId)
        {
            return new QuotaResponse { Success = true, RemainingBytes = 1073741824, TotalBytes = 10737418240 };
        }
    }

    public class BillingService
    {
        private readonly GrpcChannel _channel;
        private readonly Metadata _metadata;

        public BillingService(GrpcChannel channel, string token)
        {
            _channel = channel;
            _metadata = new Metadata { { "authorization", $"Bearer {token}" } };
        }

        public async Task<CreateAccountResponse> CreateAccountAsync(string userId, string publicKey)
        {
            return new CreateAccountResponse { Success = true, AccountId = $"acc-{userId[..8]}" };
        }

        public async Task<BalanceResponse> GetBalanceAsync(string accountId)
        {
            return new BalanceResponse { Success = true, BalanceUsd = 10000, RemainingBytes = 9663676416 };
        }

        public async Task<PurchaseResponse> PurchaseQuotaAsync(PurchaseRequest request)
        {
            return new PurchaseResponse { Success = true, CostUsd = 1000, QuotaAdded = 10737418240 };
        }
    }

    public class CellService
    {
        private readonly GrpcChannel _channel;
        private readonly Metadata _metadata;

        public CellService(GrpcChannel channel, string token)
        {
            _channel = channel;
            _metadata = new Metadata { { "authorization", $"Bearer {token}" } };
        }

        public async Task<ListCellsResponse> ListCellsAsync(ListCellsRequest request)
        {
            return new ListCellsResponse { Success = true, Cells = new List<CellInfo>() };
        }

        public async Task<AllocateResponse> AllocateGatewayAsync(AllocateRequest request)
        {
            return new AllocateResponse { Success = true, CellId = "cell-001", ConnectionToken = "token_xxx" };
        }

        public async Task<SwitchCellResponse> SwitchCellAsync(SwitchCellRequest request)
        {
            return new SwitchCellResponse { Success = true, NewCellId = "cell-002", ConnectionToken = "token_yyy" };
        }
    }
}
