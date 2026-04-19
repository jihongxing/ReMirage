# Inicio Rápido

## 1. Obtener Credenciales

Contacte al administrador para obtener:
- `endpoint`: Dirección del servicio gRPC
- `token`: Token de autenticación JWT

## 2. Instalar SDK

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

## 3. Inicializar Cliente

```python
# Ejemplo en Python
from mirage import MirageClient

client = MirageClient(
    endpoint="grpc.mirage.example:50847",
    token="your_jwt_token"
)
```

## 4. Primera Solicitud

```python
# Consultar saldo
balance = client.billing.get_balance(account_id="your_account_id")
print(f"Saldo: ${balance.balance_usd / 100:.2f}")
print(f"Restante: {balance.remaining_bytes / 1024**3:.2f} GB")
```

## 5. Siguientes Pasos

- [Referencia API](./02-referencia-api.md)
- [Autenticación](./03-autenticacion.md)
- [Manejo de Errores](./04-manejo-errores.md)
