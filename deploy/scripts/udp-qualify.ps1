# udp-qualify.ps1 - UDP 网络资质检测脚本 (Windows)
# 用途：部署前验证客户端/服务器的 UDP 收发能力和性能是否达标
# 用法：powershell -ExecutionPolicy Bypass -File udp-qualify.ps1 [-Port 8443] [-Duration 10] [-RemoteIP ""]
# 如果指定 RemoteIP，会额外测试到远程服务器的 UDP 连通性

param(
    [int]$Port = 8443,
    [int]$Duration = 10,
    [string]$RemoteIP = ""
)

$PASS = 0; $FAIL = 0; $WARN = 0

function Pass($msg) { Write-Host "[PASS] $msg" -ForegroundColor Green; $script:PASS++ }
function Fail($msg) { Write-Host "[FAIL] $msg" -ForegroundColor Red; $script:FAIL++ }
function Warn($msg) { Write-Host "[WARN] $msg" -ForegroundColor Yellow; $script:WARN++ }

Write-Host "============================================"
Write-Host " Mirage UDP 网络资质检测 (Windows)"
Write-Host " 测试端口: $Port | 时长: ${Duration}s"
if ($RemoteIP) { Write-Host " 远程目标: $RemoteIP" }
Write-Host "============================================"
Write-Host ""

# 1. 系统信息
Write-Host "--- 基础环境 ---"
$os = [System.Environment]::OSVersion
$arch = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
Pass "系统: $($os.VersionString) ($arch)"

# 检查管理员权限
$isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
if ($isAdmin) { Pass "管理员权限: 是" } else { Warn "非管理员运行（Wintun 需要管理员权限）" }

# 2. UDP Socket 创建能力
Write-Host ""
Write-Host "--- UDP Socket 能力 ---"
try {
    $testSocket = New-Object System.Net.Sockets.UdpClient($Port)
    $testSocket.Close()
    Pass "UDP $Port 端口可绑定"
} catch {
    Fail "UDP $Port 端口绑定失败: $($_.Exception.Message)"
}

# 3. UDP 回环测试
Write-Host ""
Write-Host "--- UDP 回环测试 ---"

$loopbackPort = $Port + 100
$recvCount = 0
$sendCount = 100

# 启动接收线程
$recvJob = Start-Job -ScriptBlock {
    param($p)
    $s = New-Object System.Net.Sockets.UdpClient($p)
    $s.Client.ReceiveTimeout = 3000
    $ep = New-Object System.Net.IPEndPoint([System.Net.IPAddress]::Any, 0)
    $count = 0
    for ($i = 0; $i -lt 100; $i++) {
        try {
            $null = $s.Receive([ref]$ep)
            $count++
        } catch { break }
    }
    $s.Close()
    return $count
} -ArgumentList $loopbackPort

Start-Sleep -Milliseconds 300

# 发送 100 个包
$sender = New-Object System.Net.Sockets.UdpClient
$payload = [byte[]](1..32)
for ($i = 0; $i -lt $sendCount; $i++) {
    $null = $sender.Send($payload, $payload.Length, "127.0.0.1", $loopbackPort)
}
$sender.Close()

$recvJob | Wait-Job -Timeout 10 | Out-Null
$recvCount = Receive-Job $recvJob
Remove-Job $recvJob -Force

if ($recvCount -ge 99) {
    Pass "回环收发: $recvCount/$sendCount 包 (丢包率 $($sendCount - $recvCount)%)"
} elseif ($recvCount -ge 90) {
    Warn "回环收发: $recvCount/$sendCount 包 (丢包率 $($sendCount - $recvCount)%)"
} else {
    Fail "回环收发: $recvCount/$sendCount 包 (丢包率 $($sendCount - $recvCount)%)"
}

# 4. UDP 吞吐量测试
Write-Host ""
Write-Host "--- UDP 吞吐量测试 (${Duration}s) ---"

$throughputPort = $Port + 200
$pktSize = 1200

# 接收线程
$recvThroughputJob = Start-Job -ScriptBlock {
    param($p, $dur)
    $s = New-Object System.Net.Sockets.UdpClient($p)
    $s.Client.ReceiveTimeout = 2000
    $s.Client.ReceiveBufferSize = 16 * 1024 * 1024
    $ep = New-Object System.Net.IPEndPoint([System.Net.IPAddress]::Any, 0)
    $totalBytes = 0; $totalPkts = 0
    $deadline = [DateTime]::Now.AddSeconds($dur + 2)
    while ([DateTime]::Now -lt $deadline) {
        try {
            $data = $s.Receive([ref]$ep)
            $totalBytes += $data.Length
            $totalPkts++
        } catch { break }
    }
    $s.Close()
    return @{ Bytes = $totalBytes; Pkts = $totalPkts }
} -ArgumentList $throughputPort, $Duration

Start-Sleep -Milliseconds 500

# 发送线程
$sendPayload = New-Object byte[] $pktSize
$sendSocket = New-Object System.Net.Sockets.UdpClient
$sendSocket.Client.SendBufferSize = 16 * 1024 * 1024
$sw = [System.Diagnostics.Stopwatch]::StartNew()
$sentPkts = 0

while ($sw.Elapsed.TotalSeconds -lt $Duration) {
    try {
        $null = $sendSocket.Send($sendPayload, $pktSize, "127.0.0.1", $throughputPort)
        $sentPkts++
    } catch { }
}
$elapsed = $sw.Elapsed.TotalSeconds
$sendSocket.Close()

Start-Sleep -Seconds 2
$recvThroughputJob | Wait-Job -Timeout 5 | Out-Null
$result = Receive-Job $recvThroughputJob
Remove-Job $recvThroughputJob -Force

if ($result -and $result.Pkts -gt 0) {
    $mbps = [math]::Round(($result.Bytes * 8) / ($elapsed * 1000000), 1)
    $pps = [math]::Round($result.Pkts / $elapsed, 0)
    $lossPct = [math]::Round((1 - $result.Pkts / $sentPkts) * 100, 1)

    if ($mbps -gt 100) {
        Pass "吞吐量: ${mbps} Mbps (${pps} pps)"
    } elseif ($mbps -gt 10) {
        Warn "吞吐量: ${mbps} Mbps (${pps} pps) — 建议 > 100 Mbps"
    } else {
        Fail "吞吐量: ${mbps} Mbps (${pps} pps) — 严重不足"
    }
    Write-Host "     发送: $sentPkts 包 | 接收: $($result.Pkts) 包 | 丢包: ${lossPct}%"
} else {
    Fail "吞吐量测试执行失败"
}

# 5. UDP 延迟测试
Write-Host ""
Write-Host "--- UDP 延迟测试 ---"

$latencyPort = $Port + 300
$latencies = @()

# 同步延迟测试（单线程回环）
$latSock = New-Object System.Net.Sockets.UdpClient($latencyPort)
$latSock.Client.ReceiveTimeout = 1000
$latSender = New-Object System.Net.Sockets.UdpClient
$ep = New-Object System.Net.IPEndPoint([System.Net.IPAddress]::Any, 0)

for ($i = 0; $i -lt 100; $i++) {
    $ts = [System.Diagnostics.Stopwatch]::GetTimestamp()
    $tsBytes = [BitConverter]::GetBytes($ts)
    $null = $latSender.Send($tsBytes, $tsBytes.Length, "127.0.0.1", $latencyPort)
    try {
        $data = $latSock.Receive([ref]$ep)
        $now = [System.Diagnostics.Stopwatch]::GetTimestamp()
        $then = [BitConverter]::ToInt64($data, 0)
        $freq = [System.Diagnostics.Stopwatch]::Frequency
        $usec = [math]::Round(($now - $then) * 1000000.0 / $freq, 1)
        $latencies += $usec
    } catch { }
}
$latSock.Close()
$latSender.Close()

if ($latencies.Count -gt 0) {
    $sorted = $latencies | Sort-Object
    $avg = [math]::Round(($latencies | Measure-Object -Average).Average, 1)
    $p99 = $sorted[[math]::Floor($sorted.Count * 0.99)]

    if ($avg -lt 100) {
        Pass "延迟: avg=${avg}us p99=${p99}us"
    } elseif ($avg -lt 1000) {
        Warn "延迟: avg=${avg}us p99=${p99}us — 偏高"
    } else {
        Fail "延迟: avg=${avg}us p99=${p99}us — 严重偏高"
    }
} else {
    Fail "延迟测试执行失败"
}

# 6. Windows 防火墙检查
Write-Host ""
Write-Host "--- 防火墙检查 ---"

$fwProfiles = Get-NetFirewallProfile -ErrorAction SilentlyContinue
if ($fwProfiles) {
    $enabledProfiles = $fwProfiles | Where-Object { $_.Enabled -eq $true }
    if ($enabledProfiles.Count -gt 0) {
        $names = ($enabledProfiles | ForEach-Object { $_.Name }) -join ", "
        Warn "Windows 防火墙已启用: $names（可能拦截 UDP 入站回复）"
    } else {
        Pass "Windows 防火墙已关闭"
    }
}

# 检查 UDP 放行规则
$udpRules = Get-NetFirewallRule -Direction Inbound -Action Allow -Enabled True -ErrorAction SilentlyContinue |
    Get-NetFirewallPortFilter -ErrorAction SilentlyContinue |
    Where-Object { $_.Protocol -eq "UDP" }
if ($udpRules) {
    Pass "存在 UDP 入站放行规则 ($($udpRules.Count) 条)"
} else {
    Warn "无显式 UDP 入站放行规则"
}

# 7. 远程 UDP 连通性测试（可选）
if ($RemoteIP) {
    Write-Host ""
    Write-Host "--- 远程 UDP 连通性 ($RemoteIP`:$Port) ---"

    $remoteUdp = New-Object System.Net.Sockets.UdpClient
    $testPayload = [byte[]](77, 73, 82, 65, 71, 69)  # "MIRAGE"
    $sent = $false
    try {
        $null = $remoteUdp.Send($testPayload, $testPayload.Length, $RemoteIP, $Port)
        $sent = $true
    } catch {
        Fail "UDP 发送失败: $($_.Exception.Message)"
    }
    $remoteUdp.Close()

    if ($sent) {
        Write-Host "     已发送测试包到 ${RemoteIP}:${Port}"
        Write-Host "     请在服务器侧运行: sudo tcpdump -i eth0 udp port $Port -n -c 1"
        Write-Host "     如果 tcpdump 有输出 → UDP 通路正常"
        Write-Host "     如果 tcpdump 无输出 → ISP/路由器封锁了 UDP 出站"
        Warn "远程连通性需人工确认（查看服务器 tcpdump）"
    }
}

# 8. WFP 驱动检查（VPN 残留检测）
Write-Host ""
Write-Host "--- WFP 驱动检查 ---"

$wfpProviders = netsh wfp show filters 2>$null | Select-String -Pattern "filterName" -SimpleMatch | Measure-Object
if ($wfpProviders.Count -gt 50) {
    Warn "WFP 过滤规则较多 ($($wfpProviders.Count) 条)，可能有 VPN 残留驱动拦截 UDP"
} else {
    Pass "WFP 过滤规则数量正常 ($($wfpProviders.Count) 条)"
}

# 检查常见 VPN 驱动
$vpnDrivers = @("tap0901", "wintun", "npcap", "v2ray", "clash", "wireguard")
$foundDrivers = @()
foreach ($drv in $vpnDrivers) {
    $found = Get-WindowsDriver -Online -ErrorAction SilentlyContinue | Where-Object { $_.OriginalFileName -match $drv }
    if ($found) { $foundDrivers += $drv }
}
# 也检查服务
$vpnServices = Get-Service -ErrorAction SilentlyContinue | Where-Object { $_.Name -match "v2ray|clash|wireguard|openvpn|tap" -and $_.Status -eq "Running" }
if ($vpnServices) {
    Warn "检测到运行中的 VPN 服务: $(($vpnServices | ForEach-Object { $_.Name }) -join ', ')"
} else {
    Pass "无运行中的 VPN 服务"
}

# 汇总
Write-Host ""
Write-Host "============================================"
Write-Host " 检测结果汇总"
Write-Host "============================================"
Write-Host " 通过: $PASS | 警告: $WARN | 失败: $FAIL" -ForegroundColor $(if ($FAIL -eq 0) { "Green" } elseif ($FAIL -le 2) { "Yellow" } else { "Red" })
Write-Host ""

if ($FAIL -eq 0) {
    Write-Host "✅ 客户端 UDP 资质合格，可运行 Phantom Client" -ForegroundColor Green
    exit 0
} elseif ($FAIL -le 2) {
    Write-Host "⚠️  存在问题，建议修复后再运行" -ForegroundColor Yellow
    exit 1
} else {
    Write-Host "❌ UDP 资质不合格，无法正常运行" -ForegroundColor Red
    exit 2
}
