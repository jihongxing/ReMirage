type EventHandler = (data: Record<string, unknown>) => void;

export interface WebSocketOptions {
  token: string;
}

export class MirageWebSocket {
  private url: string;
  private token: string;
  private ws: WebSocket | null = null;
  private handlers: Map<string, EventHandler> = new Map();

  constructor(url: string, options: WebSocketOptions) {
    this.url = url;
    this.token = options.token;
  }

  on(event: string, handler: EventHandler): void {
    this.handlers.set(event, handler);
  }

  off(event: string): void {
    this.handlers.delete(event);
  }

  async connect(): Promise<void> {
    return new Promise((resolve, reject) => {
      this.ws = new WebSocket(this.url, ['mirage-v1']);
      
      this.ws.onopen = () => {
        this.ws?.send(JSON.stringify({ type: 'auth', token: this.token }));
        resolve();
      };

      this.ws.onmessage = (event) => {
        try {
          const msg = JSON.parse(event.data);
          const handler = this.handlers.get(msg.event);
          if (handler) {
            handler(msg.data);
          }
        } catch (e) {
          // ignore parse errors
        }
      };

      this.ws.onerror = (error) => {
        const handler = this.handlers.get('error');
        if (handler) handler({ error });
        reject(error);
      };

      this.ws.onclose = () => {
        const handler = this.handlers.get('disconnected');
        if (handler) handler({});
      };
    });
  }

  send(event: string, data: Record<string, unknown>): void {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify({ event, data }));
    }
  }

  disconnect(): void {
    this.ws?.close();
    this.ws = null;
  }
}
