# Authentication

## Overview

Mirage uses keypair authentication - no username/password, fully anonymous.

Authentication flow:
1. Generate keypair (secp256k1)
2. Register public key to get account
3. Sign challenge with private key to get Token
4. Use Token for subsequent requests

---

## Keypair Generation

### SDK Method

```python
from mirage import MirageClient

# Generate keypair
keypair = MirageClient.generate_keypair()

# Save (private key AES-256 encrypted)
keypair.save("~/.mirage/keys", password="optional_password")

# Load
keypair = MirageClient.load_keypair("~/.mirage/keys", password="optional_password")
```

### CLI Method

```bash
# Generate
mirage-cli keygen --output ~/.mirage/keys

# With password protection
mirage-cli keygen --output ~/.mirage/keys --password

# View public key
mirage-cli pubkey --keyfile ~/.mirage/keys
```

### Manual Method (OpenSSL)

```bash
# Generate private key
openssl ecparam -genkey -name secp256k1 -out private.pem

# Export public key
openssl ec -in private.pem -pubout -out public.pem
```

---

## Account Registration

First-time use requires registering public key:

```python
from mirage import MirageClient

# Anonymous connection
client = MirageClient.anonymous(endpoint="grpc.mirage.example:50847")

# Register
account = client.register(public_key=keypair.public_key)

# Save account info
account.save("~/.mirage/account")
```

Registration response:

```json
{
  "account_id": "acc-a1b2c3d4",
  "created_at": 1704067200,
  "deposit_address": "4xxx...xxx"
}
```

---

## Signature Authentication

### Authentication Flow

```
Client                              Server
   |                                   |
   |  1. Request challenge (account_id) |
   | --------------------------------> |
   |                                   |
   |  2. Return challenge (nonce+ts)    |
   | <-------------------------------- |
   |                                   |
   |  3. Sign challenge with private key|
   | --------------------------------> |
   |                                   |
   |  4. Verify signature, return JWT   |
   | <-------------------------------- |
```

### SDK Auto-handling

```python
# Automatic signature authentication
client = MirageClient.connect(
    endpoint="grpc.mirage.example:50847",
    keyfile="~/.mirage/keys"
)
# Token automatically obtained and refreshed
```

### Manual Authentication

```python
from mirage import MirageClient

# 1. Request challenge
client = MirageClient.anonymous(endpoint="grpc.mirage.example:50847")
challenge = client.auth.request_challenge(account_id="acc-a1b2c3d4")

# 2. Sign challenge
signature = keypair.sign(challenge.nonce + challenge.timestamp)

# 3. Verify and get Token
token_response = client.auth.verify_signature(
    account_id="acc-a1b2c3d4",
    signature=signature
)

# 4. Use Token
client = MirageClient(
    endpoint="grpc.mirage.example:50847",
    token=token_response.token
)
```

---

## JWT Token

### Token Structure

```json
{
  "sub": "acc-a1b2c3d4",
  "iat": 1704067200,
  "exp": 1704153600,
  "scope": ["gateway", "billing", "cell"]
}
```

### Token Validity

- Default validity: 24 hours
- SDK auto-refresh: Renews 1 hour before expiration
- Manual refresh: Call `client.auth.refresh_token()`

### Token Usage

All API requests automatically include:

```
Authorization: Bearer <token>
```

---

## gRPC Authentication

### Interceptor

```go
// Go
func authInterceptor(token string) grpc.UnaryClientInterceptor {
    return func(ctx context.Context, method string, req, reply interface{},
        cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
        ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
        return invoker(ctx, method, req, reply, cc, opts...)
    }
}
```

---

## WebSocket Authentication

```javascript
const ws = new WebSocket('wss://ws.mirage.example:18443');

ws.onopen = () => {
  ws.send(JSON.stringify({
    type: 'auth',
    token: 'your_jwt_token'
  }));
};
```

---

## Permission Scopes

| Scope | Permission |
|-------|------------|
| gateway | Heartbeat/Traffic/Threat reporting |
| billing | Balance/Deposit/Purchase |
| cell | Cell query/Allocate/Switch |
| admin | Admin operations |

---

## Security Recommendations

1. Store token in memory, do not persist
2. Use TLS encryption
3. Rotate tokens regularly
4. Monitor abnormal logins
