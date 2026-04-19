# API संदर्भ

## सेवा अवलोकन

| सेवा | पोर्ट | प्रोटोकॉल | उद्देश्य |
|------|-------|----------|---------|
| Gateway | 50847 | gRPC | हार्टबीट/ट्रैफ़िक/खतरा रिपोर्टिंग |
| Cell | 50847 | gRPC | सेल प्रबंधन |
| Billing | 50847 | gRPC | बिलिंग/जमा |
| WebSocket | 18443 | WSS | रीयल-टाइम पुश |

---

## GatewayService

### SyncHeartbeat - हार्टबीट सिंक

**अनुरोध पैरामीटर**

| फ़ील्ड | प्रकार | आवश्यक | विवरण |
|--------|--------|--------|-------|
| gateway_id | string | ✅ | Gateway अद्वितीय पहचानकर्ता |
| version | string | ✅ | Gateway संस्करण |
| threat_level | uint32 | ❌ | वर्तमान खतरा स्तर (0-5) |

**प्रतिक्रिया पैरामीटर**

| फ़ील्ड | प्रकार | विवरण |
|--------|--------|-------|
| success | bool | सफलता स्थिति |
| remaining_quota | uint64 | शेष कोटा (बाइट्स) |
| next_heartbeat_interval | int64 | अगला हार्टबीट अंतराल (सेकंड) |

---

## BillingService

### पैकेज प्रकार (PackageType)

| मान | क्षमता |
|-----|--------|
| PACKAGE_10GB | 10 GB |
| PACKAGE_50GB | 50 GB |
| PACKAGE_100GB | 100 GB |
| PACKAGE_500GB | 500 GB |
| PACKAGE_1TB | 1 TB |

---

## CellService

### सेल स्तर (CellLevel)

| मान | विवरण | लागत गुणक |
|-----|-------|-----------|
| STANDARD | मानक सेल | 1.0x |
| PLATINUM | प्लैटिनम सेल | 1.5x |
| DIAMOND | डायमंड सेल | 2.0x |
