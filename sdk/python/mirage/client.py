"""Mirage gRPC Client"""

import grpc
from dataclasses import dataclass
from typing import Optional
import time


@dataclass
class GatewayStatus:
    online: bool = True
    active_connections: int = 0
    uptime_seconds: int = 0
    cell_id: str = ""
    region: str = ""


@dataclass
class ResourceUsage:
    cpu_percent: float = 0.0
    memory_bytes: int = 0
    bandwidth_bps: int = 0


@dataclass
class HeartbeatResponse:
    success: bool
    message: str
    remaining_quota: int
    defense_level: int
    next_heartbeat_interval: int


@dataclass
class BalanceResponse:
    success: bool
    message: str
    balance_usd: int
    total_bytes: int
    used_bytes: int
    remaining_bytes: int


@dataclass
class CellInfo:
    cell_id: str
    cell_name: str
    level: str
    country: str
    region: str
    load_percent: float
    gateway_count: int
    max_gateways: int


@dataclass
class ListCellsResponse:
    success: bool
    message: str
    cells: list


class GatewayService:
    """Gateway 服务客户端"""
    
    def __init__(self, channel, metadata):
        self._channel = channel
        self._metadata = metadata
    
    def sync_heartbeat(
        self,
        gateway_id: str,
        version: str = "1.0.0",
        threat_level: int = 0,
        status: Optional[GatewayStatus] = None,
        resource: Optional[ResourceUsage] = None
    ) -> HeartbeatResponse:
        """
        心跳同步
        
        Args:
            gateway_id: Gateway 唯一标识
            version: Gateway 版本
            threat_level: 当前威胁等级 (0-5)
            status: Gateway 状态
            resource: 资源使用情况
        
        Returns:
            HeartbeatResponse: 心跳响应
        """
        # 实际实现需要 protobuf 生成的代码
        # 这里是示例结构
        return HeartbeatResponse(
            success=True,
            message="OK",
            remaining_quota=1073741824,
            defense_level=0,
            next_heartbeat_interval=30
        )
    
    def report_traffic(
        self,
        gateway_id: str,
        base_traffic_bytes: int,
        defense_traffic_bytes: int,
        cell_level: str = "standard"
    ) -> dict:
        """
        流量上报
        
        Args:
            gateway_id: Gateway ID
            base_traffic_bytes: 业务流量（字节）
            defense_traffic_bytes: 防御流量（字节）
            cell_level: 蜂窝等级
        
        Returns:
            dict: 响应结果
        """
        return {
            "success": True,
            "remaining_quota": 1073741824,
            "current_cost_usd": 0.0,
            "quota_warning": False
        }
    
    def report_threat(
        self,
        gateway_id: str,
        threat_type: str,
        source_ip: str,
        severity: int
    ) -> dict:
        """
        威胁上报
        
        Args:
            gateway_id: Gateway ID
            threat_type: 威胁类型
            source_ip: 源 IP
            severity: 严重程度 (0-10)
        
        Returns:
            dict: 响应结果
        """
        return {
            "success": True,
            "action": "INCREASE_DEFENSE",
            "new_defense_level": 2
        }
    
    def get_quota(self, gateway_id: str, user_id: str) -> dict:
        """配额查询"""
        return {
            "success": True,
            "remaining_bytes": 1073741824,
            "total_bytes": 10737418240,
            "expires_at": int(time.time()) + 86400 * 30
        }


class BillingService:
    """计费服务客户端"""
    
    def __init__(self, channel, metadata):
        self._channel = channel
        self._metadata = metadata
    
    def create_account(self, user_id: str, public_key: str) -> dict:
        """创建账户"""
        return {
            "success": True,
            "account_id": f"acc-{user_id[:8]}",
            "created_at": int(time.time())
        }
    
    def deposit(self, account_id: str, tx_hash: str, amount_xmr: int) -> dict:
        """充值（Monero）"""
        return {
            "success": True,
            "balance_usd": 10000,
            "exchange_rate": 150.0,
            "confirmed_at": int(time.time())
        }
    
    def get_balance(self, account_id: str) -> BalanceResponse:
        """查询余额"""
        return BalanceResponse(
            success=True,
            message="OK",
            balance_usd=10000,
            total_bytes=10737418240,
            used_bytes=1073741824,
            remaining_bytes=9663676416
        )
    
    def purchase_quota(
        self,
        account_id: str,
        package_type: str,
        cell_level: str = "standard",
        quantity: int = 1
    ) -> dict:
        """购买流量包"""
        return {
            "success": True,
            "cost_usd": 1000,
            "remaining_balance": 9000,
            "quota_added": 10737418240
        }


class CellService:
    """蜂窝服务客户端"""
    
    def __init__(self, channel, metadata):
        self._channel = channel
        self._metadata = metadata
    
    def list_cells(
        self,
        level: Optional[str] = None,
        country: Optional[str] = None,
        online_only: bool = True
    ) -> ListCellsResponse:
        """查询可用蜂窝"""
        return ListCellsResponse(
            success=True,
            message="OK",
            cells=[]
        )
    
    def allocate_gateway(
        self,
        user_id: str,
        gateway_id: str,
        preferred_level: str = "standard",
        preferred_country: Optional[str] = None
    ) -> dict:
        """分配 Gateway 到蜂窝"""
        return {
            "success": True,
            "cell_id": "cell-001",
            "connection_token": "token_xxx"
        }
    
    def switch_cell(
        self,
        user_id: str,
        gateway_id: str,
        current_cell_id: str,
        target_cell_id: Optional[str] = None,
        reason: str = "USER_REQUEST"
    ) -> dict:
        """切换蜂窝"""
        return {
            "success": True,
            "new_cell_id": "cell-002",
            "connection_token": "token_yyy"
        }


class MirageClient:
    """Mirage 客户端"""
    
    def __init__(
        self,
        endpoint: str,
        token: str,
        secure: bool = True,
        timeout: int = 30
    ):
        """
        初始化客户端
        
        Args:
            endpoint: gRPC 服务地址 (host:port)
            token: JWT Token
            secure: 是否使用 TLS
            timeout: 超时时间（秒）
        """
        self._endpoint = endpoint
        self._token = token
        self._timeout = timeout
        
        # 创建 channel
        if secure:
            credentials = grpc.ssl_channel_credentials()
            self._channel = grpc.secure_channel(endpoint, credentials)
        else:
            self._channel = grpc.insecure_channel(endpoint)
        
        # 认证 metadata
        self._metadata = [("authorization", f"Bearer {token}")]
        
        # 初始化服务
        self.gateway = GatewayService(self._channel, self._metadata)
        self.billing = BillingService(self._channel, self._metadata)
        self.cell = CellService(self._channel, self._metadata)
    
    def close(self):
        """关闭连接"""
        self._channel.close()
    
    def __enter__(self):
        return self
    
    def __exit__(self, exc_type, exc_val, exc_tb):
        self.close()
