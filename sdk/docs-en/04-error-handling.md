# Error Handling

## Error Codes

| Code | Name | Description | Action |
|------|------|-------------|--------|
| 0 | OK | Success | - |
| 1 | UNAUTHENTICATED | Auth failed | Refresh token |
| 2 | QUOTA_EXCEEDED | Quota exceeded | Purchase quota |
| 3 | CELL_UNAVAILABLE | Cell unavailable | Switch cell |
| 4 | INVALID_ARGUMENT | Invalid argument | Check parameters |
| 5 | INTERNAL | Internal error | Retry or contact support |
| 6 | RATE_LIMITED | Rate limited | Reduce request frequency |
| 7 | PERMISSION_DENIED | Permission denied | Check token scope |

---

## gRPC Error Handling

### Python

```python
import grpc

try:
    response = client.gateway.sync_heartbeat(request)
except grpc.RpcError as e:
    if e.code() == grpc.StatusCode.UNAUTHENTICATED:
        # Refresh token
        pass
    elif e.code() == grpc.StatusCode.RESOURCE_EXHAUSTED:
        # Quota exceeded
        pass
    elif e.code() == grpc.StatusCode.UNAVAILABLE:
        # Service unavailable, retry
        pass
```

### Go

```go
resp, err := client.Gateway.SyncHeartbeat(ctx, req)
if err != nil {
    st, ok := status.FromError(err)
    if ok {
        switch st.Code() {
        case codes.Unauthenticated:
            // Refresh token
        case codes.ResourceExhausted:
            // Quota exceeded
        case codes.Unavailable:
            // Retry
        }
    }
}
```

---

## Retry Strategy

### Exponential Backoff

```python
import time
import random

def retry_with_backoff(func, max_retries=3):
    for attempt in range(max_retries):
        try:
            return func()
        except grpc.RpcError as e:
            if e.code() not in [grpc.StatusCode.UNAVAILABLE, grpc.StatusCode.DEADLINE_EXCEEDED]:
                raise
            if attempt == max_retries - 1:
                raise
            delay = (2 ** attempt) + random.uniform(0, 1)
            time.sleep(delay)
```

### Retryable Errors

| Error | Retryable | Suggested Delay |
|-------|-----------|-----------------|
| UNAVAILABLE | ✅ | 1-5s |
| DEADLINE_EXCEEDED | ✅ | 1-5s |
| RESOURCE_EXHAUSTED | ⚠️ | 30s+ |
| INTERNAL | ⚠️ | 5-10s |
| UNAUTHENTICATED | ❌ | - |
| INVALID_ARGUMENT | ❌ | - |
