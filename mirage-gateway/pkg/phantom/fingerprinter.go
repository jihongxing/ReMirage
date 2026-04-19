// Package phantom 客户端指纹采集器
// 通过浏览器特征建立攻击者设备画像
package phantom

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Fingerprint 设备指纹
type Fingerprint struct {
	ID           string    `json:"id"`
	Hash         string    `json:"hash"`
	FirstSeen    time.Time `json:"firstSeen"`
	LastSeen     time.Time `json:"lastSeen"`
	SeenCount    int       `json:"seenCount"`
	AssociatedIPs []string `json:"associatedIPs"`

	// 浏览器特征
	UserAgent    string `json:"userAgent"`
	Language     string `json:"language"`
	Platform     string `json:"platform"`
	ScreenRes    string `json:"screenRes"`
	Timezone     int    `json:"timezone"`
	ColorDepth   int    `json:"colorDepth"`

	// 高级指纹
	CanvasHash   string `json:"canvasHash"`
	WebGLHash    string `json:"webglHash"`
	AudioHash    string `json:"audioHash"`
	FontsHash    string `json:"fontsHash"`

	// 风险评估
	RiskScore    int    `json:"riskScore"`
	IsSuspicious bool   `json:"isSuspicious"`
}

// FingerprintCollector 指纹采集器
type FingerprintCollector struct {
	mu sync.RWMutex

	// 指纹库
	fingerprints map[string]*Fingerprint // hash -> fingerprint

	// IP 关联
	ipToFingerprint map[string]string // IP -> fingerprint hash

	// 自动拉黑阈值
	suspiciousIPThreshold int

	// 回调
	onNewFingerprint func(fp *Fingerprint)
	onSuspicious     func(fp *Fingerprint)
}

// ClientFingerprint 客户端上报的指纹数据
type ClientFingerprint struct {
	UserAgent  string `json:"userAgent"`
	Language   string `json:"language"`
	Platform   string `json:"platform"`
	ScreenRes  string `json:"screenRes"`
	Timezone   int    `json:"timezone"`
	ColorDepth int    `json:"colorDepth"`
	CanvasHash string `json:"canvasHash"`
	WebGLHash  string `json:"webglHash"`
	AudioHash  string `json:"audioHash"`
	FontsHash  string `json:"fontsHash"`
}

// NewFingerprintCollector 创建指纹采集器
func NewFingerprintCollector() *FingerprintCollector {
	return &FingerprintCollector{
		fingerprints:          make(map[string]*Fingerprint),
		ipToFingerprint:       make(map[string]string),
		suspiciousIPThreshold: 3, // 同一指纹出现在 3 个以上 IP 则标记可疑
	}
}

// Handler 返回指纹采集 HTTP 处理器
func (fc *FingerprintCollector) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/fp/collect", fc.handleCollect)
	mux.HandleFunc("/fp/beacon", fc.handleBeacon)
	return mux
}

// handleCollect 处理指纹上报
func (fc *FingerprintCollector) handleCollect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var clientFP ClientFingerprint
	if err := json.NewDecoder(r.Body).Decode(&clientFP); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	ip := extractIP(r.RemoteAddr)
	fp := fc.processFingerprint(ip, &clientFP)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"id":     fp.ID,
	})
}

// handleBeacon 处理信标请求（用于持续追踪）
func (fc *FingerprintCollector) handleBeacon(w http.ResponseWriter, r *http.Request) {
	ip := extractIP(r.RemoteAddr)

	fc.mu.Lock()
	if hash, ok := fc.ipToFingerprint[ip]; ok {
		if fp, ok := fc.fingerprints[hash]; ok {
			fp.LastSeen = time.Now()
			fp.SeenCount++
		}
	}
	fc.mu.Unlock()

	// 返回 1x1 透明 GIF
	w.Header().Set("Content-Type", "image/gif")
	w.Header().Set("Cache-Control", "no-store")
	w.Write([]byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x21, 0xf9, 0x04, 0x01, 0x00, 0x00, 0x00, 0x00, 0x2c, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x01, 0x00, 0x00})
}

// processFingerprint 处理指纹
func (fc *FingerprintCollector) processFingerprint(ip string, clientFP *ClientFingerprint) *Fingerprint {
	hash := fc.computeHash(clientFP)

	fc.mu.Lock()
	defer fc.mu.Unlock()

	fp, exists := fc.fingerprints[hash]
	if !exists {
		fp = &Fingerprint{
			ID:            fmt.Sprintf("fp_%s", hash[:12]),
			Hash:          hash,
			FirstSeen:     time.Now(),
			LastSeen:      time.Now(),
			SeenCount:     1,
			AssociatedIPs: []string{ip},
			UserAgent:     clientFP.UserAgent,
			Language:      clientFP.Language,
			Platform:      clientFP.Platform,
			ScreenRes:     clientFP.ScreenRes,
			Timezone:      clientFP.Timezone,
			ColorDepth:    clientFP.ColorDepth,
			CanvasHash:    clientFP.CanvasHash,
			WebGLHash:     clientFP.WebGLHash,
			AudioHash:     clientFP.AudioHash,
			FontsHash:     clientFP.FontsHash,
		}
		fc.fingerprints[hash] = fp

		if fc.onNewFingerprint != nil {
			go fc.onNewFingerprint(fp)
		}
	} else {
		fp.LastSeen = time.Now()
		fp.SeenCount++

		// 检查是否是新 IP
		isNewIP := true
		for _, existingIP := range fp.AssociatedIPs {
			if existingIP == ip {
				isNewIP = false
				break
			}
		}
		if isNewIP {
			fp.AssociatedIPs = append(fp.AssociatedIPs, ip)
		}

		// 检查是否可疑
		if len(fp.AssociatedIPs) >= fc.suspiciousIPThreshold && !fp.IsSuspicious {
			fp.IsSuspicious = true
			fp.RiskScore = len(fp.AssociatedIPs) * 20
			if fc.onSuspicious != nil {
				go fc.onSuspicious(fp)
			}
		}
	}

	fc.ipToFingerprint[ip] = hash
	return fp
}

// computeHash 计算指纹哈希
func (fc *FingerprintCollector) computeHash(clientFP *ClientFingerprint) string {
	data := fmt.Sprintf("%s|%s|%s|%s|%d|%d|%s|%s|%s|%s",
		clientFP.UserAgent,
		clientFP.Language,
		clientFP.Platform,
		clientFP.ScreenRes,
		clientFP.Timezone,
		clientFP.ColorDepth,
		clientFP.CanvasHash,
		clientFP.WebGLHash,
		clientFP.AudioHash,
		clientFP.FontsHash,
	)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// GetFingerprint 获取指纹
func (fc *FingerprintCollector) GetFingerprint(hash string) *Fingerprint {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	return fc.fingerprints[hash]
}

// GetFingerprintByIP 通过 IP 获取指纹
func (fc *FingerprintCollector) GetFingerprintByIP(ip string) *Fingerprint {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	if hash, ok := fc.ipToFingerprint[ip]; ok {
		return fc.fingerprints[hash]
	}
	return nil
}

// GetSuspiciousFingerprints 获取所有可疑指纹
func (fc *FingerprintCollector) GetSuspiciousFingerprints() []*Fingerprint {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	var suspicious []*Fingerprint
	for _, fp := range fc.fingerprints {
		if fp.IsSuspicious {
			suspicious = append(suspicious, fp)
		}
	}
	return suspicious
}

// GetAllIPsForFingerprint 获取指纹关联的所有 IP
func (fc *FingerprintCollector) GetAllIPsForFingerprint(hash string) []string {
	fc.mu.RLock()
	defer fc.mu.RUnlock()
	if fp, ok := fc.fingerprints[hash]; ok {
		result := make([]string, len(fp.AssociatedIPs))
		copy(result, fp.AssociatedIPs)
		return result
	}
	return nil
}

// OnNewFingerprint 设置新指纹回调
func (fc *FingerprintCollector) OnNewFingerprint(fn func(fp *Fingerprint)) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.onNewFingerprint = fn
}

// OnSuspicious 设置可疑指纹回调
func (fc *FingerprintCollector) OnSuspicious(fn func(fp *Fingerprint)) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.onSuspicious = fn
}

// SetSuspiciousThreshold 设置可疑阈值
func (fc *FingerprintCollector) SetSuspiciousThreshold(threshold int) {
	fc.mu.Lock()
	defer fc.mu.Unlock()
	fc.suspiciousIPThreshold = threshold
}

// GenerateCollectorJS 生成客户端采集脚本
func (fc *FingerprintCollector) GenerateCollectorJS(endpoint string) string {
	return fmt.Sprintf(`(function(){
  var fp = {
    userAgent: navigator.userAgent,
    language: navigator.language,
    platform: navigator.platform,
    screenRes: screen.width + 'x' + screen.height,
    timezone: new Date().getTimezoneOffset(),
    colorDepth: screen.colorDepth,
    canvasHash: '',
    webglHash: '',
    audioHash: '',
    fontsHash: ''
  };

  // Canvas 指纹
  try {
    var canvas = document.createElement('canvas');
    var ctx = canvas.getContext('2d');
    ctx.textBaseline = 'top';
    ctx.font = '14px Arial';
    ctx.fillText('Mirage-FP-Canvas', 2, 2);
    fp.canvasHash = canvas.toDataURL().slice(-32);
  } catch(e) {}

  // WebGL 指纹
  try {
    var gl = document.createElement('canvas').getContext('webgl');
    var debugInfo = gl.getExtension('WEBGL_debug_renderer_info');
    if (debugInfo) {
      fp.webglHash = btoa(gl.getParameter(debugInfo.UNMASKED_RENDERER_WEBGL)).slice(0, 32);
    }
  } catch(e) {}

  // AudioContext 指纹
  try {
    var audioCtx = new (window.AudioContext || window.webkitAudioContext)();
    var oscillator = audioCtx.createOscillator();
    var analyser = audioCtx.createAnalyser();
    var gain = audioCtx.createGain();
    oscillator.connect(analyser);
    analyser.connect(gain);
    gain.connect(audioCtx.destination);
    oscillator.start(0);
    var dataArray = new Float32Array(analyser.frequencyBinCount);
    analyser.getFloatFrequencyData(dataArray);
    fp.audioHash = btoa(dataArray.slice(0, 10).join(',')).slice(0, 32);
    oscillator.stop();
    audioCtx.close();
  } catch(e) {}

  // 字体指纹
  try {
    var fonts = ['Arial', 'Verdana', 'Times New Roman', 'Courier New', 'Georgia'];
    var detected = [];
    var testString = 'mmmmmmmmmmlli';
    var testSize = '72px';
    var h = document.getElementsByTagName('body')[0];
    var s = document.createElement('span');
    s.style.fontSize = testSize;
    s.innerHTML = testString;
    var defaultWidth = {};
    var defaultHeight = {};
    for (var index in fonts) {
      s.style.fontFamily = fonts[index];
      h.appendChild(s);
      defaultWidth[fonts[index]] = s.offsetWidth;
      defaultHeight[fonts[index]] = s.offsetHeight;
      h.removeChild(s);
    }
    fp.fontsHash = btoa(Object.values(defaultWidth).join(',')).slice(0, 32);
  } catch(e) {}

  // 上报
  fetch('%s/fp/collect', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify(fp)
  });

  // 持续追踪
  setInterval(function() {
    new Image().src = '%s/fp/beacon?t=' + Date.now();
  }, 30000);
})();`, endpoint, endpoint)
}
