use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::RwLock;
use tokio_tungstenite::{connect_async, tungstenite::Message};
use futures_util::{StreamExt, SinkExt};
use serde_json::{json, Value};

type EventHandler = Box<dyn Fn(Value) + Send + Sync>;

pub struct MirageWebSocket {
    url: String,
    token: String,
    handlers: Arc<RwLock<HashMap<String, EventHandler>>>,
}

impl MirageWebSocket {
    pub fn builder() -> WebSocketBuilder {
        WebSocketBuilder::default()
    }

    pub fn on<F>(&mut self, event: &str, handler: F)
    where
        F: Fn(Value) + Send + Sync + 'static,
    {
        let handlers = self.handlers.clone();
        tokio::spawn(async move {
            handlers.write().await.insert(event.to_string(), Box::new(handler));
        });
    }

    pub async fn connect(&self) -> Result<(), Box<dyn std::error::Error>> {
        let url = format!("{}?token={}", self.url, self.token);
        let (ws_stream, _) = connect_async(&url).await?;
        let (mut write, mut read) = ws_stream.split();

        // Auth
        let auth_msg = json!({ "type": "auth", "token": self.token });
        write.send(Message::Text(auth_msg.to_string())).await?;

        let handlers = self.handlers.clone();
        while let Some(msg) = read.next().await {
            if let Ok(Message::Text(text)) = msg {
                if let Ok(parsed) = serde_json::from_str::<Value>(&text) {
                    if let Some(event) = parsed.get("event").and_then(|e| e.as_str()) {
                        let data = parsed.get("data").cloned().unwrap_or(Value::Null);
                        let handlers = handlers.read().await;
                        if let Some(handler) = handlers.get(event) {
                            handler(data);
                        }
                    }
                }
            }
        }

        Ok(())
    }

    pub async fn send(&self, event: &str, data: Value) -> Result<(), Box<dyn std::error::Error>> {
        // 实际实现需要保存 write half
        Ok(())
    }
}

#[derive(Default)]
pub struct WebSocketBuilder {
    url: String,
    token: String,
}

impl WebSocketBuilder {
    pub fn url(mut self, url: &str) -> Self {
        self.url = url.to_string();
        self
    }

    pub fn token(mut self, token: &str) -> Self {
        self.token = token.to_string();
        self
    }

    pub fn build(self) -> Result<MirageWebSocket, Box<dyn std::error::Error>> {
        Ok(MirageWebSocket {
            url: self.url,
            token: self.token,
            handlers: Arc::new(RwLock::new(HashMap::new())),
        })
    }
}
