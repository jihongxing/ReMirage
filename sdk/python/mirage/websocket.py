"""Mirage WebSocket Client"""

import json
import threading
from typing import Callable, Dict, Optional
import websocket


class MirageWebSocket:
    """WebSocket 实时推送客户端"""
    
    def __init__(self, url: str, token: str):
        """
        初始化 WebSocket 客户端
        
        Args:
            url: WebSocket 地址 (wss://host:port)
            token: JWT Token
        """
        self._url = url
        self._token = token
        self._handlers: Dict[str, Callable] = {}
        self._ws: Optional[websocket.WebSocketApp] = None
        self._thread: Optional[threading.Thread] = None
        self._connected = False
    
    def on(self, event: str):
        """
        注册事件处理器
        
        Args:
            event: 事件类型 (threat, quota_warning, cell_switch, etc.)
        
        Example:
            @ws.on("threat")
            def handle_threat(data):
                print(data)
        """
        def decorator(func: Callable):
            self._handlers[event] = func
            return func
        return decorator
    
    def connect(self, blocking: bool = True):
        """
        连接 WebSocket
        
        Args:
            blocking: 是否阻塞当前线程
        """
        headers = {"Authorization": f"Bearer {self._token}"}
        
        self._ws = websocket.WebSocketApp(
            self._url,
            header=headers,
            on_open=self._on_open,
            on_message=self._on_message,
            on_error=self._on_error,
            on_close=self._on_close
        )
        
        if blocking:
            self._ws.run_forever()
        else:
            self._thread = threading.Thread(target=self._ws.run_forever)
            self._thread.daemon = True
            self._thread.start()
    
    def disconnect(self):
        """断开连接"""
        if self._ws:
            self._ws.close()
            self._connected = False
    
    def send(self, event: str, data: dict):
        """
        发送消息
        
        Args:
            event: 事件类型
            data: 数据
        """
        if self._ws and self._connected:
            message = json.dumps({"event": event, "data": data})
            self._ws.send(message)
    
    def _on_open(self, ws):
        self._connected = True
        if "connected" in self._handlers:
            self._handlers["connected"](None)
    
    def _on_message(self, ws, message):
        try:
            msg = json.loads(message)
            event = msg.get("event", "unknown")
            data = msg.get("data", {})
            
            if event in self._handlers:
                self._handlers[event](data)
            elif "message" in self._handlers:
                self._handlers["message"](msg)
        except json.JSONDecodeError:
            pass
    
    def _on_error(self, ws, error):
        if "error" in self._handlers:
            self._handlers["error"](error)
    
    def _on_close(self, ws, close_status_code, close_msg):
        self._connected = False
        if "disconnected" in self._handlers:
            self._handlers["disconnected"]({"code": close_status_code, "message": close_msg})
