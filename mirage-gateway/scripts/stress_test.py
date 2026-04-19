#!/usr/bin/env python3
"""
Mirage Gateway 战损模拟器 (离线模式)
模拟 UniversalSensor -> AutonomousGSwitch -> HealthMonitor 全链路联动

无需后端 API，纯本地模拟干扰和响应

测试目标:
1. 特定 SNI 干扰 (TCP RST)
2. 内容劫持模拟 (HTML 注入)
3. App 环境模拟 (丢包/限速)

指标:
- 逃逸时长 (ms)
- 误判率 (%)
"""

import asyncio
import random
import time
import math
import argparse
from dataclasses import dataclass, field
from typing import List, Dict, Optional
from enum import Enum
from collections import deque

# ============================================
# 配置
# ============================================

class JammingLevel(Enum):
    NONE = 0
    LIGHT = 1           # 轻微干扰 (5% 丢包)
    MODERATE = 2        # 中度干扰 (15% 丢包)
    TACTICAL = 3        # 精准干扰 (特定 App)
    FULL_BLOCK = 4      # 全域封锁

@dataclass
class TestConfig:
    target_domain: str = "cdn-test.example.com"
    jamming_level: JammingLevel = JammingLevel.TACTICAL
    duration_seconds: int = 60
    report_interval: float = 1.0

# ============================================
# 模拟 HealthMonitor (战损自检引擎)
# ============================================

class AlertLevel(Enum):
    NONE = 0
    YELLOW = 1  # 丢包 > 10%
    RED = 2     # 丢包 > 25% 或 RST

@dataclass
class LinkQuality:
    loss_rate: float = 0.0
    rtt_mean: float = 50.0
    rtt_jitter: float = 5.0
    jitter_entropy: float = 2.0
    bandwidth_util: float = 50.0
    alert_level: AlertLevel = AlertLevel.NONE
    is_precise_jam: bool = False

class SimHealthMonitor:
    """模拟战损自检引擎"""
    
    def __init__(self):
        self.rtt_history: deque = deque(maxlen=100)
        self.loss_history: deque = deque(maxlen=100)
        self.rst_count: int = 0
        self.current_quality = LinkQuality()
        self.last_alert = AlertLevel.NONE
        
        # 回调
        self.on_yellow_alert = None
        self.on_red_alert = None
        
        # 基线 RTT
        self.baseline_rtt = 50.0
    
    def inject_jamming(self, loss_rate: float, rtt_delta: float, rst_burst: int = 0):
        """注入干扰"""
        # 模拟 RTT 波动
        rtt = self.baseline_rtt + rtt_delta + random.gauss(0, 10)
        self.rtt_history.append(max(1, rtt))
        
        # 模拟丢包
        self.loss_history.append(loss_rate)
        
        # RST 注入
        self.rst_count += rst_burst
    
    def sample_normal(self):
        """正常采样"""
        rtt = self.baseline_rtt + random.gauss(0, 3)
        self.rtt_history.append(max(1, rtt))
        self.loss_history.append(random.uniform(0, 2))

    def evaluate(self) -> LinkQuality:
        """评估链路质量"""
        quality = LinkQuality()
        
        # 计算丢包率均值
        if self.loss_history:
            quality.loss_rate = sum(self.loss_history) / len(self.loss_history)
        
        # 计算 RTT 均值和方差
        if self.rtt_history:
            quality.rtt_mean = sum(self.rtt_history) / len(self.rtt_history)
            variance = sum((x - quality.rtt_mean) ** 2 for x in self.rtt_history) / len(self.rtt_history)
            quality.rtt_jitter = math.sqrt(variance) if variance > 0 else 0
        
        # 计算抖动熵
        quality.jitter_entropy = self._calculate_jitter_entropy()
        
        # 检测精准干扰
        quality.is_precise_jam = self._detect_precise_jamming(quality)
        
        # 判定告警等级
        quality.alert_level = self._determine_alert_level(quality)
        
        self.current_quality = quality
        
        # 触发告警
        if quality.alert_level.value > self.last_alert.value:
            if quality.alert_level == AlertLevel.YELLOW and self.on_yellow_alert:
                self.on_yellow_alert()
            elif quality.alert_level == AlertLevel.RED and self.on_red_alert:
                self.on_red_alert()
        
        self.last_alert = quality.alert_level
        return quality
    
    def _calculate_jitter_entropy(self) -> float:
        """计算抖动熵"""
        if len(self.rtt_history) < 10:
            return 0
        
        rtt_list = list(self.rtt_history)
        diffs = [rtt_list[i] - rtt_list[i-1] for i in range(1, len(rtt_list))]
        
        # 量化到 bins
        bins = {}
        bin_size = 1.0
        for d in diffs:
            b = int(d / bin_size)
            bins[b] = bins.get(b, 0) + 1
        
        # 香农熵
        entropy = 0.0
        total = len(diffs)
        for count in bins.values():
            p = count / total
            if p > 0:
                entropy -= p * math.log2(p)
        
        return entropy

    def _detect_precise_jamming(self, quality: LinkQuality) -> bool:
        """检测精准干扰"""
        if quality.jitter_entropy < 5.0:
            return False
        if quality.loss_rate < 5.0:
            return False
        if quality.bandwidth_util < 70 and quality.loss_rate > 10:
            return True
        if quality.rtt_jitter > quality.rtt_mean * 0.5:
            return True
        return False
    
    def _determine_alert_level(self, quality: LinkQuality) -> AlertLevel:
        """判定告警等级"""
        if self.rst_count > 10:
            return AlertLevel.RED
        if quality.loss_rate > 25:
            return AlertLevel.RED
        if quality.loss_rate > 10:
            return AlertLevel.YELLOW
        if quality.is_precise_jam:
            return AlertLevel.RED
        return AlertLevel.NONE
    
    def reset(self):
        """重置状态"""
        self.rtt_history.clear()
        self.loss_history.clear()
        self.rst_count = 0
        self.last_alert = AlertLevel.NONE

# ============================================
# 模拟 UniversalSensor (通用传感器)
# ============================================

@dataclass
class HijackingSignature:
    sig_type: str
    expected_length: int = 0
    actual_length: int = 0
    entropy_delta: float = 0.0
    detected_at: float = 0.0

@dataclass
class DomainReputation:
    domain: str
    score: float = 100.0
    baseline_entropy: float = 4.5
    fail_count: int = 0
    hijack_count: int = 0
    signatures: List[HijackingSignature] = field(default_factory=list)

class SimUniversalSensor:
    """模拟通用业务回显探测器"""
    
    def __init__(self):
        self.reputations: Dict[str, DomainReputation] = {}
        self.reputation_decay = 0.95
        self.hijack_threshold = 0.3
        
        # 回调
        self.on_reputation_drop = None
        self.on_hijack_detected = None
    
    def register_domain(self, domain: str):
        """注册域名"""
        self.reputations[domain] = DomainReputation(domain=domain)
    
    def inject_hijack(self, domain: str, hijack_type: str, entropy_delta: float = 2.5):
        """注入劫持"""
        if domain not in self.reputations:
            return
        
        rep = self.reputations[domain]
        sig = HijackingSignature(
            sig_type=hijack_type,
            entropy_delta=entropy_delta,
            detected_at=time.time()
        )
        rep.signatures.append(sig)
        rep.hijack_count += 1
        
        # 扣分
        rep.score -= 15
        rep.score = max(0, rep.score)
        
        if self.on_hijack_detected:
            self.on_hijack_detected(domain, sig)
    
    def inject_failure(self, domain: str, count: int = 1):
        """注入失败"""
        if domain not in self.reputations:
            return
        
        rep = self.reputations[domain]
        rep.fail_count += count
        rep.score -= count * 5
        rep.score = max(0, rep.score)
    
    def probe_cycle(self, domain: str, success_rate: float = 1.0):
        """模拟探测周期"""
        if domain not in self.reputations:
            return
        
        rep = self.reputations[domain]
        
        # 衰减
        rep.score *= self.reputation_decay
        
        # 成功率加分
        rep.score += success_rate * 10
        
        # 限制范围
        rep.score = min(100, max(0, rep.score))
        
        # 信誉度下降告警
        if rep.score < 50 and self.on_reputation_drop:
            self.on_reputation_drop(domain, rep.score)
    
    def get_reputation(self, domain: str) -> Optional[DomainReputation]:
        return self.reputations.get(domain)

# ============================================
# 模拟 AutonomousGSwitch (自适应逃逸引擎)
# ============================================

class SwitchReason(Enum):
    NONE = 0
    LOW_REPUTATION = 1
    HTTP_ERROR = 2
    RST_FLOOD = 3
    JA4_FINGERPRINT = 4
    MANUAL = 5
    SCHEDULED = 6

@dataclass
class ShadowDomain:
    name: str
    is_warmed_up: bool = True
    usage_count: int = 0

@dataclass
class SwitchEvent:
    old_domain: str
    new_domain: str
    reason: SwitchReason
    reputation_score: float
    timestamp: float

class SimAutonomousGSwitch:
    """模拟自适应逃逸引擎"""
    
    def __init__(self):
        self.active_domain: str = ""
        self.shadow_pool: List[ShadowDomain] = []
        self.burned_domains: List[str] = []
        self.switch_history: List[SwitchEvent] = []
        self.error_counts = {"http_403": 0, "http_451": 0, "rst": 0}
        
        self.reputation_threshold = 40.0
        self.error_burst_threshold = 10
        self.switch_cooldown = 3.0  # 切换冷却时间 (秒)
        self.last_switch_time = 0.0
        
        # 回调
        self.on_switch = None
    
    def start(self, initial_domain: str):
        """启动"""
        self.active_domain = initial_domain
        self._warmup_shadow_pool()
    
    def _warmup_shadow_pool(self):
        """预热影子池"""
        for i in range(3):
            self.shadow_pool.append(ShadowDomain(
                name=f"shadow-{i+1}.cdn.example.com"
            ))

    def report_error(self, error_type: str):
        """上报错误"""
        if error_type in self.error_counts:
            self.error_counts[error_type] += 1
    
    def check_and_switch(self, domain: str, reputation_score: float) -> bool:
        """检查并切换"""
        if domain != self.active_domain:
            return False
        if reputation_score >= self.reputation_threshold:
            return False
        # 冷却检查
        if time.time() - self.last_switch_time < self.switch_cooldown:
            return False
        return self._execute_switch(SwitchReason.LOW_REPUTATION, reputation_score)
    
    def check_error_burst(self) -> bool:
        """检查错误爆发"""
        # 冷却检查
        if time.time() - self.last_switch_time < self.switch_cooldown:
            return False
        
        http_errors = self.error_counts["http_403"] + self.error_counts["http_451"]
        if http_errors > self.error_burst_threshold:
            self.error_counts = {"http_403": 0, "http_451": 0, "rst": 0}  # 重置
            return self._execute_switch(SwitchReason.HTTP_ERROR, 0)
        if self.error_counts["rst"] > self.error_burst_threshold * 2:
            self.error_counts = {"http_403": 0, "http_451": 0, "rst": 0}  # 重置
            return self._execute_switch(SwitchReason.JA4_FINGERPRINT, 0)
        return False
    
    def _execute_switch(self, reason: SwitchReason, score: float) -> bool:
        """执行切换"""
        if not self.shadow_pool:
            self._warmup_shadow_pool()
        
        new_domain = self.shadow_pool.pop(0)
        old_domain = self.active_domain
        
        self.burned_domains.append(old_domain)
        self.active_domain = new_domain.name
        new_domain.usage_count += 1
        self.last_switch_time = time.time()  # 记录切换时间
        
        event = SwitchEvent(
            old_domain=old_domain,
            new_domain=new_domain.name,
            reason=reason,
            reputation_score=score,
            timestamp=time.time()
        )
        self.switch_history.append(event)
        
        if self.on_switch:
            self.on_switch(event)
        
        # 补充影子池
        self._warmup_shadow_pool()
        
        return True
    
    def force_switch(self, reason: SwitchReason) -> bool:
        return self._execute_switch(reason, 0)

# ============================================
# 干扰模拟器
# ============================================

class JammingSimulator:
    """干扰模拟器"""
    
    def __init__(self, config: TestConfig):
        self.config = config
        self.stats = {
            "rst_injected": 0,
            "content_hijacked": 0,
            "packets_dropped": 0,
        }
    
    def get_drop_rate(self) -> float:
        """获取丢包率"""
        return {
            JammingLevel.NONE: 0.0,
            JammingLevel.LIGHT: 5.0,
            JammingLevel.MODERATE: 15.0,
            JammingLevel.TACTICAL: 25.0,
            JammingLevel.FULL_BLOCK: 80.0,
        }.get(self.config.jamming_level, 0.0)
    
    def get_rtt_delta(self) -> float:
        """获取 RTT 增量"""
        return {
            JammingLevel.NONE: 0.0,
            JammingLevel.LIGHT: 10.0,
            JammingLevel.MODERATE: 30.0,
            JammingLevel.TACTICAL: 100.0,
            JammingLevel.FULL_BLOCK: 500.0,
        }.get(self.config.jamming_level, 0.0)
    
    def should_inject_rst(self) -> bool:
        """是否注入 RST"""
        if self.config.jamming_level.value < JammingLevel.MODERATE.value:
            return False
        return random.random() < 0.3
    
    def should_inject_hijack(self) -> bool:
        """是否注入劫持"""
        if self.config.jamming_level.value < JammingLevel.TACTICAL.value:
            return False
        return random.random() < 0.2

# ============================================
# 测试结果
# ============================================

@dataclass
class TestResult:
    escape_latency_ms: float = 0.0
    false_positive_count: int = 0
    true_positive_count: int = 0
    reputation_samples: List[float] = field(default_factory=list)
    sni_updates: int = 0
    alerts_triggered: Dict[str, int] = field(default_factory=lambda: {"yellow": 0, "red": 0})
    total_cycles: int = 0

# ============================================
# 离线测试执行器
# ============================================

class OfflineStressTestRunner:
    """离线压力测试执行器"""
    
    def __init__(self, config: TestConfig):
        self.config = config
        self.jammer = JammingSimulator(config)
        self.monitor = SimHealthMonitor()
        self.sensor = SimUniversalSensor()
        self.gswitch = SimAutonomousGSwitch()
        self.result = TestResult()
        
        # 连接组件
        self._wire_components()
    
    def _wire_components(self):
        """连接组件回调"""
        # Monitor -> GSwitch
        def on_red_alert():
            self.result.alerts_triggered["red"] += 1
            self.gswitch.report_error("rst")
        
        def on_yellow_alert():
            self.result.alerts_triggered["yellow"] += 1
        
        self.monitor.on_red_alert = on_red_alert
        self.monitor.on_yellow_alert = on_yellow_alert
        
        # Sensor -> GSwitch
        def on_reputation_drop(domain: str, score: float):
            if self.gswitch.check_and_switch(domain, score):
                self.result.sni_updates += 1
        
        def on_hijack_detected(domain: str, sig: HijackingSignature):
            self.gswitch.report_error("http_403")
        
        self.sensor.on_reputation_drop = on_reputation_drop
        self.sensor.on_hijack_detected = on_hijack_detected
        
        # GSwitch 切换回调
        def on_switch(event: SwitchEvent):
            print(f"🔄 域名切换: {event.old_domain} → {event.new_domain} (reason={event.reason.name})")
        
        self.gswitch.on_switch = on_switch
    
    def run(self) -> TestResult:
        """执行测试"""
        print("=" * 60)
        print("🎯 Mirage Gateway 战损模拟测试 (离线模式)")
        print("=" * 60)
        print(f"目标域名: {self.config.target_domain}")
        print(f"干扰等级: {self.config.jamming_level.name}")
        print(f"测试时长: {self.config.duration_seconds}s")
        print("=" * 60)
        
        # 初始化
        self.sensor.register_domain(self.config.target_domain)
        self.gswitch.start(self.config.target_domain)
        
        # 基线阶段 (5s)
        print("\n📊 建立基线 (5s)...")
        self._run_baseline_phase(5)
        
        # 干扰阶段
        print("\n🔥 开始干扰...")
        escape_start = None
        escape_end = None
        
        start_time = time.time()
        cycle = 0
        
        while time.time() - start_time < self.config.duration_seconds:
            cycle += 1
            self.result.total_cycles = cycle
            
            # 注入干扰
            self._inject_jamming_cycle()
            
            # 评估
            quality = self.monitor.evaluate()
            
            # 探测周期
            success_rate = 1.0 - (self.jammer.get_drop_rate() / 100.0)
            self.sensor.probe_cycle(self.config.target_domain, success_rate)
            
            # 记录信誉分
            rep = self.sensor.get_reputation(self.config.target_domain)
            if rep:
                self.result.reputation_samples.append(rep.score)
                
                # 检测逃逸开始
                if rep.score < 50 and escape_start is None:
                    escape_start = time.time()
                    print(f"⚠️  信誉分下降至 {rep.score:.1f}，开始计时")
            
            # 检查错误爆发
            if self.gswitch.check_error_burst():
                self.result.sni_updates += 1
                if escape_start and not escape_end:
                    escape_end = time.time()
            
            # 检测逃逸完成
            if escape_start and not escape_end and self.result.sni_updates > 0:
                escape_end = time.time()
            
            time.sleep(0.1)  # 100ms 周期
        
        # 计算逃逸时长
        if escape_start and escape_end:
            self.result.escape_latency_ms = (escape_end - escape_start) * 1000
        
        return self.result

    def _run_baseline_phase(self, duration: int):
        """基线阶段"""
        for _ in range(duration * 10):  # 100ms 周期
            self.monitor.sample_normal()
            self.sensor.probe_cycle(self.config.target_domain, 1.0)
            time.sleep(0.1)
    
    def _inject_jamming_cycle(self):
        """注入干扰周期"""
        drop_rate = self.jammer.get_drop_rate()
        rtt_delta = self.jammer.get_rtt_delta()
        rst_burst = 5 if self.jammer.should_inject_rst() else 0
        
        self.monitor.inject_jamming(drop_rate, rtt_delta, rst_burst)
        
        if rst_burst > 0:
            self.jammer.stats["rst_injected"] += rst_burst
            self.gswitch.report_error("rst")
        
        if self.jammer.should_inject_hijack():
            self.jammer.stats["content_hijacked"] += 1
            self.sensor.inject_hijack(
                self.config.target_domain,
                "html_injection",
                entropy_delta=2.5
            )
        
        if random.random() < drop_rate / 100:
            self.jammer.stats["packets_dropped"] += 1
            self.sensor.inject_failure(self.config.target_domain)
    
    def print_report(self):
        """打印测试报告"""
        result = self.result
        
        print("\n" + "=" * 60)
        print("📋 测试报告")
        print("=" * 60)
        
        # 逃逸时长
        print(f"\n⏱️  逃逸时长: {result.escape_latency_ms:.0f}ms")
        if result.escape_latency_ms > 0:
            if result.escape_latency_ms < 1000:
                print("   评级: ✅ 优秀 (<1s)")
            elif result.escape_latency_ms < 3000:
                print("   评级: ⚠️  良好 (<3s)")
            else:
                print("   评级: ❌ 需优化 (>3s)")
        
        # 信誉分变化
        if result.reputation_samples:
            initial = result.reputation_samples[0]
            final = result.reputation_samples[-1]
            min_score = min(result.reputation_samples)
            print(f"\n📉 信誉分变化:")
            print(f"   初始: {initial:.1f}")
            print(f"   最低: {min_score:.1f}")
            print(f"   最终: {final:.1f}")
        
        # 告警统计
        print(f"\n🚨 告警统计:")
        print(f"   Yellow Alert: {result.alerts_triggered['yellow']} 次")
        print(f"   Red Alert: {result.alerts_triggered['red']} 次")
        
        # SNI 更新
        print(f"\n🔄 SNI 更新: {result.sni_updates} 次")
        
        # 干扰统计
        print(f"\n💥 干扰统计:")
        print(f"   RST 注入: {self.jammer.stats['rst_injected']} 次")
        print(f"   内容劫持: {self.jammer.stats['content_hijacked']} 次")
        print(f"   丢包模拟: {self.jammer.stats['packets_dropped']} 次")
        
        # 切换历史
        if self.gswitch.switch_history:
            print(f"\n🦎 切换历史:")
            for event in self.gswitch.switch_history:
                print(f"   {event.old_domain} → {event.new_domain} ({event.reason.name})")
        
        print("\n" + "=" * 60)

# ============================================
# 独立测试场景
# ============================================

def test_rst_detection():
    """测试 RST 注入检测"""
    print("\n🧪 测试场景: RST 注入检测")
    print("-" * 40)
    
    monitor = SimHealthMonitor()
    alert_triggered = False
    
    def on_red():
        nonlocal alert_triggered
        alert_triggered = True
    
    monitor.on_red_alert = on_red
    
    # 注入 RST
    for i in range(20):
        monitor.inject_jamming(loss_rate=5, rtt_delta=20, rst_burst=3)
        monitor.evaluate()
        if alert_triggered:
            break
    
    print(f"   RST 注入数: {monitor.rst_count}")
    print(f"   Red Alert 触发: {'✅ 是' if alert_triggered else '❌ 否'}")
    return alert_triggered

def test_content_hijack():
    """测试内容劫持检测"""
    print("\n🧪 测试场景: 内容劫持检测")
    print("-" * 40)
    
    sensor = SimUniversalSensor()
    sensor.register_domain("test.example.com")
    
    hijack_detected = False
    
    def on_hijack(domain, sig):
        nonlocal hijack_detected
        hijack_detected = True
    
    sensor.on_hijack_detected = on_hijack
    
    # 注入劫持
    sensor.inject_hijack("test.example.com", "html_injection", 3.0)
    
    rep = sensor.get_reputation("test.example.com")
    print(f"   劫持检测: {'✅ 成功' if hijack_detected else '❌ 失败'}")
    print(f"   信誉分: {rep.score:.1f}")
    return hijack_detected

def test_reputation_decay():
    """测试信誉分衰减"""
    print("\n🧪 测试场景: 信誉分衰减")
    print("-" * 40)
    
    sensor = SimUniversalSensor()
    sensor.register_domain("test.example.com")
    
    initial_score = sensor.get_reputation("test.example.com").score
    
    # 模拟持续失败
    for i in range(20):
        sensor.inject_failure("test.example.com", count=2)
        sensor.probe_cycle("test.example.com", success_rate=0.3)
    
    final_score = sensor.get_reputation("test.example.com").score
    
    print(f"   初始分: {initial_score:.1f}")
    print(f"   最终分: {final_score:.1f}")
    print(f"   衰减正常: {'✅ 是' if final_score < 50 else '❌ 否'}")
    return final_score < 50

def test_auto_switch():
    """测试自动切换"""
    print("\n🧪 测试场景: 自动切换")
    print("-" * 40)
    
    gswitch = SimAutonomousGSwitch()
    gswitch.start("primary.example.com")
    
    switched = False
    
    def on_switch(event):
        nonlocal switched
        switched = True
        print(f"   切换: {event.old_domain} → {event.new_domain}")
    
    gswitch.on_switch = on_switch
    
    # 模拟低信誉触发切换
    gswitch.check_and_switch("primary.example.com", 30.0)
    
    print(f"   切换触发: {'✅ 是' if switched else '❌ 否'}")
    print(f"   当前域名: {gswitch.active_domain}")
    return switched

# ============================================
# 分布式一致性测试
# ============================================

@dataclass
class SimPeerNode:
    """模拟对等节点"""
    id: str
    region: str
    active_domain: str = "cdn-test.example.com"
    reputation_score: float = 100.0
    last_sync_time: float = 0.0
    sync_latency_ms: float = 0.0
    is_synced: bool = True

class SimSyncBroadcaster:
    """模拟 M.C.C. 同步广播器"""
    
    def __init__(self, node_id: str):
        self.node_id = node_id
        self.peers: Dict[str, SimPeerNode] = {}
        self.message_queue: List[dict] = []
        self.stats = {
            "messages_sent": 0,
            "messages_received": 0,
            "sync_success": 0,
            "sync_failed": 0,
        }
    
    def add_peer(self, peer_id: str, region: str):
        self.peers[peer_id] = SimPeerNode(id=peer_id, region=region)
    
    def broadcast_gswitch(self, old_domain: str, new_domain: str, reason: str):
        """广播 G-Switch 切换"""
        msg = {
            "type": "gswitch",
            "old_domain": old_domain,
            "new_domain": new_domain,
            "reason": reason,
            "timestamp": time.time(),
            "source": self.node_id,
        }
        self.message_queue.append(msg)
        self.stats["messages_sent"] += 1
        
        # 模拟广播延迟
        sync_results = []
        for peer_id, peer in self.peers.items():
            latency = self._simulate_network_latency(peer.region)
            peer.sync_latency_ms = latency
            peer.last_sync_time = time.time()
            
            # 模拟同步成功率 (95%)
            if random.random() < 0.95:
                peer.active_domain = new_domain
                peer.is_synced = True
                self.stats["sync_success"] += 1
                sync_results.append((peer_id, True, latency))
            else:
                peer.is_synced = False
                self.stats["sync_failed"] += 1
                sync_results.append((peer_id, False, latency))
        
        return sync_results
    
    def broadcast_bdna_reset(self, template_id: int, reason: str):
        """广播 B-DNA Reset"""
        msg = {
            "type": "bdna_reset",
            "template_id": template_id,
            "reason": reason,
            "timestamp": time.time(),
            "source": self.node_id,
        }
        self.message_queue.append(msg)
        self.stats["messages_sent"] += 1
        return True
    
    def _simulate_network_latency(self, region: str) -> float:
        """模拟网络延迟"""
        base_latency = {
            "sg": 30,   # 新加坡
            "de": 150,  # 德国
            "us": 200,  # 美国
            "jp": 50,   # 日本
            "ch": 180,  # 瑞士
        }.get(region, 100)
        return base_latency + random.gauss(0, 20)
    
    def get_sync_status(self) -> Dict[str, bool]:
        """获取同步状态"""
        return {peer_id: peer.is_synced for peer_id, peer in self.peers.items()}
    
    def get_avg_sync_latency(self) -> float:
        """获取平均同步延迟"""
        if not self.peers:
            return 0
        return sum(p.sync_latency_ms for p in self.peers.values()) / len(self.peers)


class DistributedConsistencyTest:
    """分布式一致性测试"""
    
    def __init__(self, node_count: int = 5):
        self.node_count = node_count
        self.primary_node = "node-primary"
        self.broadcaster = SimSyncBroadcaster(self.primary_node)
        self.gswitch = SimAutonomousGSwitch()
        self.sensor = SimUniversalSensor()
        
        # 初始化对等节点
        regions = ["sg", "de", "us", "jp", "ch"]
        for i in range(node_count):
            node_id = f"node-{regions[i % len(regions)]}-{i}"
            self.broadcaster.add_peer(node_id, regions[i % len(regions)])
    
    def run(self, duration: int = 30) -> dict:
        """运行分布式一致性测试"""
        print("\n" + "=" * 60)
        print("🌐 分布式一致性测试")
        print("=" * 60)
        print(f"主节点: {self.primary_node}")
        print(f"对等节点数: {self.node_count}")
        print(f"测试时长: {duration}s")
        print("=" * 60)
        
        # 初始化
        self.sensor.register_domain("cdn-test.example.com")
        self.gswitch.start("cdn-test.example.com")
        
        results = {
            "total_switches": 0,
            "sync_events": [],
            "consistency_checks": [],
            "avg_sync_latency_ms": 0,
            "sync_success_rate": 0,
        }
        
        # 连接 GSwitch 回调到广播器
        def on_switch(event: SwitchEvent):
            sync_results = self.broadcaster.broadcast_gswitch(
                event.old_domain, event.new_domain, event.reason.name
            )
            results["total_switches"] += 1
            results["sync_events"].append({
                "timestamp": event.timestamp,
                "old_domain": event.old_domain,
                "new_domain": event.new_domain,
                "sync_results": sync_results,
            })
            
            # 打印同步状态
            synced = sum(1 for _, ok, _ in sync_results if ok)
            total = len(sync_results)
            avg_lat = sum(lat for _, _, lat in sync_results) / total if total > 0 else 0
            print(f"📡 广播完成: {synced}/{total} 节点同步 (avg_latency={avg_lat:.0f}ms)")
        
        self.gswitch.on_switch = on_switch
        
        # 模拟干扰
        print("\n🔥 开始模拟干扰...")
        start_time = time.time()
        
        while time.time() - start_time < duration:
            # 模拟信誉下降
            self.sensor.inject_failure("cdn-test.example.com", count=3)
            self.sensor.probe_cycle("cdn-test.example.com", success_rate=0.4)
            
            rep = self.sensor.get_reputation("cdn-test.example.com")
            if rep and rep.score < 40:
                self.gswitch.check_and_switch("cdn-test.example.com", rep.score)
            
            # 模拟 RST 攻击
            if random.random() < 0.3:
                self.gswitch.report_error("rst")
            
            self.gswitch.check_error_burst()
            
            # 一致性检查
            sync_status = self.broadcaster.get_sync_status()
            all_synced = all(sync_status.values())
            results["consistency_checks"].append({
                "timestamp": time.time(),
                "all_synced": all_synced,
                "synced_count": sum(sync_status.values()),
                "total_count": len(sync_status),
            })
            
            time.sleep(0.5)
        
        # 计算统计
        stats = self.broadcaster.stats
        total_sync = stats["sync_success"] + stats["sync_failed"]
        results["sync_success_rate"] = stats["sync_success"] / total_sync * 100 if total_sync > 0 else 0
        results["avg_sync_latency_ms"] = self.broadcaster.get_avg_sync_latency()
        
        return results
    
    def print_report(self, results: dict):
        """打印测试报告"""
        print("\n" + "=" * 60)
        print("📋 分布式一致性测试报告")
        print("=" * 60)
        
        print(f"\n🔄 总切换次数: {results['total_switches']}")
        print(f"📡 同步成功率: {results['sync_success_rate']:.1f}%")
        print(f"⏱️  平均同步延迟: {results['avg_sync_latency_ms']:.0f}ms")
        
        # 一致性分析
        checks = results["consistency_checks"]
        if checks:
            consistent_count = sum(1 for c in checks if c["all_synced"])
            consistency_rate = consistent_count / len(checks) * 100
            print(f"✅ 一致性达成率: {consistency_rate:.1f}%")
        
        # 节点同步状态
        print(f"\n🌐 节点同步状态:")
        sync_status = self.broadcaster.get_sync_status()
        for node_id, is_synced in sync_status.items():
            peer = self.broadcaster.peers[node_id]
            status = "✅ 同步" if is_synced else "❌ 失步"
            print(f"   {node_id}: {status} (latency={peer.sync_latency_ms:.0f}ms)")
        
        # 切换历史
        if results["sync_events"]:
            print(f"\n🦎 切换广播历史:")
            for event in results["sync_events"][-5:]:  # 最近 5 次
                synced = sum(1 for _, ok, _ in event["sync_results"] if ok)
                total = len(event["sync_results"])
                print(f"   {event['old_domain']} → {event['new_domain']} ({synced}/{total} synced)")
        
        # 判定
        print("\n" + "-" * 60)
        if results["sync_success_rate"] >= 95:
            print("🎯 结论: 分布式一致性 ✅ 达标 (>95%)")
        elif results["sync_success_rate"] >= 80:
            print("⚠️  结论: 分布式一致性 ⚠️ 基本达标 (80-95%)")
        else:
            print("❌ 结论: 分布式一致性 ❌ 不达标 (<80%)")
        
        if results["avg_sync_latency_ms"] < 100:
            print("🎯 同步延迟: ✅ 优秀 (<100ms)")
        elif results["avg_sync_latency_ms"] < 200:
            print("⚠️  同步延迟: ⚠️ 良好 (100-200ms)")
        else:
            print("❌ 同步延迟: ❌ 需优化 (>200ms)")
        
        print("=" * 60)


def test_distributed_consistency():
    """运行分布式一致性测试"""
    test = DistributedConsistencyTest(node_count=5)
    results = test.run(duration=20)
    test.print_report(results)
    return results["sync_success_rate"] >= 80


# ============================================
# 网络分区测试 (Network Partition)
# ============================================

@dataclass
class PartitionedNode:
    """分区节点"""
    id: str
    region: str
    partition_id: int = 0  # 所属分区
    active_domain: str = "cdn-test.example.com"
    is_reachable: bool = True
    last_heartbeat: float = 0.0
    local_switch_count: int = 0

class SimQuorumSync:
    """模拟法定人数同步器 (2PC-Lite)"""
    
    def __init__(self, node_id: str, quorum_size: int = 3):
        self.node_id = node_id
        self.quorum_size = quorum_size
        self.peers: Dict[str, PartitionedNode] = {}
        self.pending_commits: Dict[str, dict] = {}
        self.committed_ops: List[dict] = []
        self.stats = {
            "precommit_sent": 0,
            "precommit_ack": 0,
            "commit_success": 0,
            "commit_failed": 0,
            "force_resync": 0,
        }
    
    def add_peer(self, peer_id: str, region: str, partition_id: int = 0):
        self.peers[peer_id] = PartitionedNode(
            id=peer_id, region=region, partition_id=partition_id
        )
    
    def precommit(self, op_id: str, operation: dict) -> int:
        """预提交阶段"""
        self.stats["precommit_sent"] += 1
        ack_count = 0
        
        for peer_id, peer in self.peers.items():
            if not peer.is_reachable:
                continue
            
            # 模拟网络延迟
            latency = self._get_latency(peer.region)
            if latency < 500:  # 500ms 超时
                ack_count += 1
                self.stats["precommit_ack"] += 1
        
        self.pending_commits[op_id] = {
            "operation": operation,
            "ack_count": ack_count,
            "timestamp": time.time(),
        }
        
        return ack_count
    
    def commit(self, op_id: str) -> bool:
        """提交阶段"""
        if op_id not in self.pending_commits:
            return False
        
        pending = self.pending_commits[op_id]
        ack_count = pending["ack_count"]
        
        # 检查是否达到法定人数
        if ack_count >= self.quorum_size:
            self.committed_ops.append(pending["operation"])
            self.stats["commit_success"] += 1
            del self.pending_commits[op_id]
            return True
        else:
            self.stats["commit_failed"] += 1
            del self.pending_commits[op_id]
            return False
    
    def force_resync(self, peer_id: str) -> bool:
        """强制重同步"""
        if peer_id not in self.peers:
            return False
        
        peer = self.peers[peer_id]
        if not peer.is_reachable:
            return False
        
        # 模拟重同步：同步到最新提交的域名
        if self.committed_ops:
            peer.active_domain = self.committed_ops[-1]["new_domain"]
        peer.last_heartbeat = time.time()
        self.stats["force_resync"] += 1
        
        return True
    
    def sync_all_reachable(self):
        """同步所有可达节点到最新状态"""
        if not self.committed_ops:
            return
        
        latest_domain = self.committed_ops[-1]["new_domain"]
        for peer_id, peer in self.peers.items():
            if peer.is_reachable:
                peer.active_domain = latest_domain
    
    def _get_latency(self, region: str) -> float:
        base = {"sg": 30, "de": 150, "us": 200, "jp": 50, "ch": 180}.get(region, 100)
        return base + random.gauss(0, 30)


class SimMultiPathBuffer:
    """模拟双发选收缓冲器"""
    
    def __init__(self):
        self.dual_mode_active = False
        self.old_path = None
        self.new_path = None
        self.start_time = 0.0
        self.duration_ms = 100
        self.stats = {
            "total_sent": 0,
            "total_recv": 0,
            "deduped": 0,
            "old_path_pkts": 0,
            "new_path_pkts": 0,
        }
        self.seq_window: set = set()
    
    def enable_dual_send(self, old_path: str, new_path: str):
        """启用双发模式"""
        self.dual_mode_active = True
        self.old_path = old_path
        self.new_path = new_path
        self.start_time = time.time()
        print(f"🔀 双发选收启用: {old_path} + {new_path}")
    
    def send_dual(self, seq: int, data: bytes) -> bool:
        """双发数据"""
        if not self.dual_mode_active:
            return False
        
        # 检查是否超时
        if (time.time() - self.start_time) * 1000 > self.duration_ms:
            self.dual_mode_active = False
            print(f"🔀 双发选收结束: sent={self.stats['total_sent']}, dedup={self.stats['deduped']}")
            return False
        
        # 模拟双发
        self.stats["total_sent"] += 2
        
        # 模拟旧路径成功率 (70%)
        if random.random() < 0.7:
            self.stats["old_path_pkts"] += 1
        
        # 模拟新路径成功率 (90%)
        if random.random() < 0.9:
            self.stats["new_path_pkts"] += 1
        
        return True
    
    def receive_and_dedupe(self, seq: int, from_path: str) -> bool:
        """接收并去重"""
        self.stats["total_recv"] += 1
        
        if seq in self.seq_window:
            self.stats["deduped"] += 1
            return False
        
        self.seq_window.add(seq)
        
        # 限制窗口大小
        if len(self.seq_window) > 100:
            self.seq_window.pop()
        
        return True


class NetworkPartitionTest:
    """网络分区测试"""
    
    def __init__(self, node_count: int = 6):
        self.node_count = node_count
        self.primary_node = "node-primary"
        self.quorum_sync = SimQuorumSync(self.primary_node, quorum_size=node_count // 2 + 1)
        self.multipath_buffer = SimMultiPathBuffer()
        self.gswitch = SimAutonomousGSwitch()
        self.sensor = SimUniversalSensor()
        
        # 初始化节点（分成两个分区）
        regions = ["sg", "de", "us", "jp", "ch", "sg"]
        for i in range(node_count):
            node_id = f"node-{regions[i % len(regions)]}-{i}"
            partition_id = 0 if i < node_count // 2 else 1
            self.quorum_sync.add_peer(node_id, regions[i % len(regions)], partition_id)
    
    def simulate_partition(self, partition_to_isolate: int):
        """模拟网络分区"""
        print(f"\n⚡ 模拟网络分区: 隔离分区 {partition_to_isolate}")
        
        for peer_id, peer in self.quorum_sync.peers.items():
            if peer.partition_id == partition_to_isolate:
                peer.is_reachable = False
                print(f"   ❌ {peer_id} 不可达")
            else:
                peer.is_reachable = True
                print(f"   ✅ {peer_id} 可达")
    
    def heal_partition(self):
        """恢复网络分区"""
        print(f"\n🔧 恢复网络分区")
        for peer_id, peer in self.quorum_sync.peers.items():
            peer.is_reachable = True
            print(f"   ✅ {peer_id} 已恢复")
    
    def run(self, duration: int = 30) -> dict:
        """运行网络分区测试"""
        print("\n" + "=" * 60)
        print("🌐 网络分区测试 (Network Partition)")
        print("=" * 60)
        print(f"主节点: {self.primary_node}")
        print(f"总节点数: {self.node_count}")
        print(f"法定人数: {self.quorum_sync.quorum_size}")
        print(f"测试时长: {duration}s")
        print("=" * 60)
        
        # 初始化
        self.sensor.register_domain("cdn-test.example.com")
        self.gswitch.start("cdn-test.example.com")
        
        results = {
            "phase1_normal": {"switches": 0, "commits": 0, "failed": 0},
            "phase2_partition": {"switches": 0, "commits": 0, "failed": 0},
            "phase3_heal": {"switches": 0, "commits": 0, "failed": 0, "resyncs": 0},
            "dual_send_stats": {},
            "final_consistency": False,
        }
        
        # 阶段 1: 正常运行 (10s)
        print("\n📊 阶段 1: 正常运行 (10s)")
        results["phase1_normal"] = self._run_phase(10, "normal")
        
        # 阶段 2: 网络分区 (10s)
        print("\n📊 阶段 2: 网络分区 (10s)")
        self.simulate_partition(1)  # 隔离分区 1
        results["phase2_partition"] = self._run_phase(10, "partition")
        
        # 阶段 3: 恢复并重同步 (10s)
        print("\n📊 阶段 3: 恢复并重同步 (10s)")
        self.heal_partition()
        results["phase3_heal"] = self._run_phase(10, "heal")
        
        # 检查最终一致性
        results["final_consistency"] = self._check_final_consistency()
        results["dual_send_stats"] = self.multipath_buffer.stats.copy()
        
        return results
    
    def _run_phase(self, duration: int, phase_name: str) -> dict:
        """运行测试阶段"""
        phase_stats = {"switches": 0, "commits": 0, "failed": 0, "resyncs": 0}
        start_time = time.time()
        seq = 0
        
        while time.time() - start_time < duration:
            # 模拟信誉下降
            self.sensor.inject_failure("cdn-test.example.com", count=2)
            self.sensor.probe_cycle("cdn-test.example.com", success_rate=0.5)
            
            rep = self.sensor.get_reputation("cdn-test.example.com")
            
            # 检查是否需要切换
            if rep and rep.score < 40:
                old_domain = self.gswitch.active_domain
                if self.gswitch.check_and_switch("cdn-test.example.com", rep.score):
                    new_domain = self.gswitch.active_domain
                    phase_stats["switches"] += 1
                    
                    # 启用双发选收
                    self.multipath_buffer.enable_dual_send(old_domain, new_domain)
                    
                    # 尝试 2PC 提交
                    op_id = f"switch-{time.time()}"
                    ack_count = self.quorum_sync.precommit(op_id, {
                        "type": "gswitch",
                        "old_domain": old_domain,
                        "new_domain": new_domain,
                    })
                    
                    if self.quorum_sync.commit(op_id):
                        phase_stats["commits"] += 1
                        print(f"   ✅ 切换提交成功: {old_domain} → {new_domain} (ack={ack_count})")
                    else:
                        phase_stats["failed"] += 1
                        print(f"   ❌ 切换提交失败: {old_domain} → {new_domain} (ack={ack_count}, need={self.quorum_sync.quorum_size})")
            
            # 模拟双发数据
            if self.multipath_buffer.dual_mode_active:
                for _ in range(5):
                    seq += 1
                    self.multipath_buffer.send_dual(seq, b"test_data")
                    
                    # 模拟接收
                    if random.random() < 0.8:
                        self.multipath_buffer.receive_and_dedupe(seq, "old_path")
                    if random.random() < 0.9:
                        self.multipath_buffer.receive_and_dedupe(seq, "new_path")
            
            # 恢复阶段：尝试重同步失步节点
            if phase_name == "heal":
                # 同步所有可达节点
                self.quorum_sync.sync_all_reachable()
                for peer_id, peer in self.quorum_sync.peers.items():
                    if peer.is_reachable:
                        if self.quorum_sync.force_resync(peer_id):
                            phase_stats["resyncs"] += 1
            
            time.sleep(0.3)
        
        return phase_stats
    
    def _check_final_consistency(self) -> bool:
        """检查最终一致性"""
        if not self.quorum_sync.committed_ops:
            return True
        
        expected_domain = self.quorum_sync.committed_ops[-1]["new_domain"]
        consistent_count = 0
        
        for peer_id, peer in self.quorum_sync.peers.items():
            if peer.active_domain == expected_domain:
                consistent_count += 1
        
        return consistent_count >= self.quorum_sync.quorum_size
    
    def print_report(self, results: dict):
        """打印测试报告"""
        print("\n" + "=" * 60)
        print("📋 网络分区测试报告")
        print("=" * 60)
        
        # 阶段统计
        for phase_name, stats in [
            ("阶段 1 (正常)", results["phase1_normal"]),
            ("阶段 2 (分区)", results["phase2_partition"]),
            ("阶段 3 (恢复)", results["phase3_heal"]),
        ]:
            print(f"\n📊 {phase_name}:")
            print(f"   切换次数: {stats['switches']}")
            print(f"   提交成功: {stats['commits']}")
            print(f"   提交失败: {stats['failed']}")
            if "resyncs" in stats:
                print(f"   强制重同步: {stats['resyncs']}")
        
        # 双发选收统计
        dual_stats = results["dual_send_stats"]
        print(f"\n🔀 双发选收统计:")
        print(f"   总发送: {dual_stats.get('total_sent', 0)}")
        print(f"   总接收: {dual_stats.get('total_recv', 0)}")
        print(f"   去重数: {dual_stats.get('deduped', 0)}")
        print(f"   旧路径包: {dual_stats.get('old_path_pkts', 0)}")
        print(f"   新路径包: {dual_stats.get('new_path_pkts', 0)}")
        
        # 法定人数同步统计
        qs_stats = self.quorum_sync.stats
        print(f"\n📡 法定人数同步统计:")
        print(f"   预提交发送: {qs_stats['precommit_sent']}")
        print(f"   预提交确认: {qs_stats['precommit_ack']}")
        print(f"   提交成功: {qs_stats['commit_success']}")
        print(f"   提交失败: {qs_stats['commit_failed']}")
        print(f"   强制重同步: {qs_stats['force_resync']}")
        
        # 最终一致性
        print(f"\n✅ 最终一致性: {'达成' if results['final_consistency'] else '未达成'}")
        
        # 节点状态
        print(f"\n🌐 节点最终状态:")
        for peer_id, peer in self.quorum_sync.peers.items():
            status = "✅" if peer.is_reachable else "❌"
            print(f"   {peer_id}: {status} partition={peer.partition_id} domain={peer.active_domain}")
        
        # 判定
        print("\n" + "-" * 60)
        total_commits = results["phase1_normal"]["commits"] + results["phase2_partition"]["commits"] + results["phase3_heal"]["commits"]
        total_failed = results["phase1_normal"]["failed"] + results["phase2_partition"]["failed"] + results["phase3_heal"]["failed"]
        
        if total_commits > 0:
            success_rate = total_commits / (total_commits + total_failed) * 100
            print(f"🎯 总体提交成功率: {success_rate:.1f}%")
            
            if success_rate >= 80 and results["final_consistency"]:
                print("🎯 结论: 网络分区测试 ✅ 通过")
            elif success_rate >= 60:
                print("⚠️  结论: 网络分区测试 ⚠️ 基本通过")
            else:
                print("❌ 结论: 网络分区测试 ❌ 失败")
        else:
            print("⚠️  结论: 无切换发生，无法评估")
        
        print("=" * 60)


def test_network_partition():
    """运行网络分区测试"""
    test = NetworkPartitionTest(node_count=6)
    results = test.run(duration=30)
    test.print_report(results)
    return results["final_consistency"]


# ============================================
# 主函数
# ============================================

def main():
    parser = argparse.ArgumentParser(description="Mirage Gateway 战损模拟器 (离线模式)")
    parser.add_argument("--domain", default="cdn-test.example.com", help="目标域名")
    parser.add_argument("--level", type=int, default=3, choices=[0, 1, 2, 3, 4],
                        help="干扰等级 (0=无, 1=轻微, 2=中度, 3=精准, 4=全域)")
    parser.add_argument("--duration", type=int, default=30, help="测试时长(秒)")
    parser.add_argument("--scenario", choices=["full", "rst", "hijack", "decay", "switch", "all", "distributed", "partition"],
                        default="full", help="测试场景")
    
    args = parser.parse_args()
    
    if args.scenario == "all":
        # 运行所有单元测试
        print("\n" + "=" * 60)
        print("🧪 运行所有单元测试")
        print("=" * 60)
        
        results = {
            "RST 检测": test_rst_detection(),
            "劫持检测": test_content_hijack(),
            "信誉衰减": test_reputation_decay(),
            "自动切换": test_auto_switch(),
        }
        
        print("\n" + "=" * 60)
        print("📊 测试结果汇总")
        print("=" * 60)
        for name, passed in results.items():
            status = "✅ 通过" if passed else "❌ 失败"
            print(f"   {name}: {status}")
        
        return
    
    if args.scenario == "distributed":
        # 分布式一致性测试
        test_distributed_consistency()
        return
    
    if args.scenario == "partition":
        # 网络分区测试
        test_network_partition()
        return
    
    if args.scenario == "rst":
        test_rst_detection()
        return
    
    if args.scenario == "hijack":
        test_content_hijack()
        return
    
    if args.scenario == "decay":
        test_reputation_decay()
        return
    
    if args.scenario == "switch":
        test_auto_switch()
        return
    
    # 完整测试
    config = TestConfig(
        target_domain=args.domain,
        jamming_level=JammingLevel(args.level),
        duration_seconds=args.duration,
    )
    
    runner = OfflineStressTestRunner(config)
    runner.run()
    runner.print_report()

if __name__ == "__main__":
    main()
