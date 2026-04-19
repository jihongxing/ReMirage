# Quick Start

## 1. Install SDK

### Python
```bash
pip install mirage-sdk
```

### Go
```bash
go get github.com/mirage/sdk-go
```

### JavaScript/TypeScript
```bash
npm install @mirage/sdk
```

### Java
```xml
<dependency>
    <groupId>io.mirage</groupId>
    <artifactId>mirage-sdk</artifactId>
    <version>1.0.0</version>
</dependency>
```

### Rust
```toml
[dependencies]
mirage-sdk = "1.0"
```

### C#
```bash
dotnet add package MirageSDK
```

### PHP
```bash
composer require mirage/sdk
```

### Swift
```swift
.package(url: "https://github.com/mirage/sdk-swift.git", from: "1.0.0")
```

### Kotlin
```kotlin
implementation("io.mirage:mirage-sdk:1.0.0")
```

---

## 2. Generate Keypair (First Time)

Keypair is used for authentication. Keep your private key safe.

```python
from mirage import MirageClient

# Generate keypair
keypair = MirageClient.generate_keypair()

# Save locally (private key encrypted)
keypair.save("~/.mirage/keys")

# View public key (for registration)
print(keypair.public_key)
```

Or use CLI:

```bash
# Generate keypair
mirage-cli keygen --output ~/.mirage/keys

# View public key
mirage-cli pubkey --keyfile ~/.mirage/keys
```

---

## 3. Register Account (First Time)

Register an anonymous account with your public key.

```python
from mirage import MirageClient

# Load keypair
keypair = MirageClient.load_keypair("~/.mirage/keys")

# Connect (anonymous mode)
client = MirageClient.anonymous(endpoint="grpc.mirage.example:50847")

# Register account
account = client.register(public_key=keypair.public_key)

print(f"Account ID: {account.account_id}")
print(f"Created at: {account.created_at}")

# Save account ID
account.save("~/.mirage/account")
```

---

## 4. Authenticate

Sign with private key for each session.

```python
from mirage import MirageClient

# Option 1: Auto-load keypair and account
client = MirageClient.connect(
    endpoint="grpc.mirage.example:50847",
    keyfile="~/.mirage/keys"
)

# Option 2: Manual
keypair = MirageClient.load_keypair("~/.mirage/keys")
client = MirageClient.authenticate(
    endpoint="grpc.mirage.example:50847",
    private_key=keypair.private_key,
    account_id="your_account_id"
)
```

---

## 5. Start Using

```python
# Query balance
balance = client.billing.get_balance()
print(f"Balance: ${balance.balance_usd / 100:.2f}")
print(f"Remaining: {balance.remaining_bytes / 1024**3:.2f} GB")

# List available cells
cells = client.cell.list_cells(online_only=True)
for cell in cells.cells:
    print(f"{cell.cell_name} ({cell.country}): {cell.load_percent:.1f}%")

# Purchase quota
result = client.billing.purchase_quota(
    package_type="PACKAGE_10GB",
    cell_level="standard"
)
print(f"Purchased {result.quota_added / 1024**3:.0f} GB")
```

---

## 6. Complete Example

```python
from mirage import MirageClient

# === First Time ===
# Generate and save keypair
keypair = MirageClient.generate_keypair()
keypair.save("~/.mirage/keys")

# Register account
client = MirageClient.anonymous(endpoint="grpc.mirage.example:50847")
account = client.register(public_key=keypair.public_key)
account.save("~/.mirage/account")

# === Daily Use ===
# One-line connect
client = MirageClient.connect(
    endpoint="grpc.mirage.example:50847",
    keyfile="~/.mirage/keys"
)

# Query balance
balance = client.billing.get_balance()
print(f"Balance: ${balance.balance_usd / 100:.2f}")

# Close connection
client.close()
```

---

## Next Steps

- [API Reference](./02-api-reference.md) - Complete API documentation
- [Authentication](./03-authentication.md) - Keypair authentication details
- [Error Handling](./04-error-handling.md) - Error codes and retry strategies
