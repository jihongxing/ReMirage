import { useEffect, useRef, useState } from 'react';

export interface MirageEvent {
  type: 'threat' | 'traffic' | 'heartbeat' | 'quota' | 'billing' | 'cell_lifecycle';
  timestamp: number;
  data: any;
}

export interface ThreatEvent {
  lat: number;
  lng: number;
  intensity: number;
  label: string;
  srcIp: string;
  threatType: string;
}

export interface TrafficEvent {
  gatewayId: string;
  lat: number;
  lng: number;
  businessBytes: number;
  defenseBytes: number;
}

export interface QuotaEvent {
  userId: string;
  remainingBytes: number;
  totalBytes: number;
  usagePercent: number;
}

export interface CellLifecycleEvent {
  gatewayId: string;
  phase: number; // 0=潜伏, 1=校准, 2=服役
  lat: number;
  lng: number;
  networkQuality?: number;
  progress?: number; // 校准进度 0-100
}

export const useMirageSocket = (url: string = 'ws://localhost:8080/ws') => {
  const [connected, setConnected] = useState(false);
  const [events, setEvents] = useState<MirageEvent[]>([]);
  const [lastThreat, setLastThreat] = useState<ThreatEvent | null>(null);
  const [lastTraffic, setLastTraffic] = useState<TrafficEvent | null>(null);
  const [lastQuota, setLastQuota] = useState<QuotaEvent | null>(null);
  const [lastCellLifecycle, setLastCellLifecycle] = useState<CellLifecycleEvent | null>(null);
  
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout>>();

  // 发送命令到后端
  const sendCommand = (type: string, data: any) => {
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({ type, data, timestamp: Date.now() }));
    }
  };

  const connect = () => {
    try {
      const ws = new WebSocket(url);
      
      ws.onopen = () => {
        console.log('🔌 [WebSocket] 已连接到 Mirage-OS');
        setConnected(true);
      };

      ws.onmessage = (event) => {
        try {
          const message: MirageEvent = JSON.parse(event.data);
          
          // 更新事件列表（保留最近 100 条）
          setEvents((prev) => [...prev.slice(-99), message]);

          // 根据事件类型更新状态
          switch (message.type) {
            case 'threat':
              setLastThreat(message.data as ThreatEvent);
              break;
            case 'traffic':
              setLastTraffic(message.data as TrafficEvent);
              break;
            case 'quota':
              setLastQuota(message.data as QuotaEvent);
              break;
            case 'cell_lifecycle':
              setLastCellLifecycle(message.data as CellLifecycleEvent);
              break;
          }
        } catch (err) {
          console.error('❌ [WebSocket] 解析消息失败:', err);
        }
      };

      ws.onerror = (error) => {
        console.error('❌ [WebSocket] 连接错误:', error);
      };

      ws.onclose = () => {
        console.log('🔌 [WebSocket] 连接已关闭，5 秒后重连...');
        setConnected(false);
        
        // 5 秒后自动重连
        reconnectTimeoutRef.current = setTimeout(() => {
          connect();
        }, 5000);
      };

      wsRef.current = ws;
    } catch (err) {
      console.error('❌ [WebSocket] 创建连接失败:', err);
    }
  };

  useEffect(() => {
    connect();

    return () => {
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
      if (wsRef.current) {
        wsRef.current.close();
      }
    };
  }, [url]);

  return {
    connected,
    events,
    lastThreat,
    lastTraffic,
    lastQuota,
    lastCellLifecycle,
    sendCommand,
  };
};
