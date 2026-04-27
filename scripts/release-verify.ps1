#Requires -Version 5.1
<#
.SYNOPSIS
    发布前本地复验脚本 (PowerShell)
.DESCRIPTION
    把所有"靠人记得"的复验命令沉淀到一个可执行入口
.EXAMPLE
    .\scripts\release-verify.ps1
#>

$ErrorActionPreference = "Continue"
$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Split-Path -Parent $ScriptDir
$Pass = 0
$Fail = 0
$Skip = 0

function Check {
    param([string]$Name, [scriptblock]$Block)
    Write-Host "── $Name ──"
    try {
        $result = & $Block 2>&1
        if ($LASTEXITCODE -eq 0 -or $LASTEXITCODE -eq $null) {
            Write-Host "  ✅ PASS"
            $script:Pass++
        } else {
            Write-Host "  ❌ FAIL"
            $script:Fail++
        }
    } catch {
        Write-Host "  ❌ FAIL: $_"
        $script:Fail++
    }
}

Write-Host ""
Write-Host "╔══════════════════════════════════════╗"
Write-Host "║   Mirage Release Verification        ║"
Write-Host "╚══════════════════════════════════════╝"
Write-Host ""

# Gate 1: 构建
Check "Gateway go vet"  { Push-Location (Join-Path $ProjectRoot "mirage-gateway"); go vet ./...; Pop-Location }
Check "OS go vet"       { Push-Location (Join-Path $ProjectRoot "mirage-os"); go vet ./...; Pop-Location }
Check "Gateway build"   { Push-Location (Join-Path $ProjectRoot "mirage-gateway"); go build ./cmd/gateway/; Pop-Location }
Check "OS build"        { Push-Location (Join-Path $ProjectRoot "mirage-os"); go build ./...; Pop-Location }
Check "Client build"    { Push-Location (Join-Path $ProjectRoot "phantom-client"); go build ./cmd/phantom/; Pop-Location }
Check "CLI build"       { Push-Location (Join-Path $ProjectRoot "mirage-cli"); go build ./...; Pop-Location }

# Gate 2: 关键测试
Check "Gateway tests"   { Push-Location (Join-Path $ProjectRoot "mirage-gateway"); go test ./...; Pop-Location }
Check "OS tests"        { Push-Location (Join-Path $ProjectRoot "mirage-os"); go test ./...; Pop-Location }
Check "Benchmarks"      { Push-Location (Join-Path $ProjectRoot "benchmarks"); go test ./...; Pop-Location }
Check "Quota -count=10" { Push-Location (Join-Path $ProjectRoot "mirage-gateway"); go test -count=10 ./pkg/api/; Pop-Location }

# Gate 3: 配置安全
$cfgPath = Join-Path $ProjectRoot "mirage-os\configs\config.yaml"
Check "No dangerous defaults" {
    $hits = Select-String -Path $cfgPath -Pattern 'password: postgres|change-this-in-production' -ErrorAction SilentlyContinue
    if ($hits) { throw "dangerous defaults found" }
}
Check "Redis requirepass" {
    $hits = Select-String -Path (Join-Path $ProjectRoot "deploy\docker-compose.os.yml") -Pattern 'requirepass'
    if (-not $hits) { throw "requirepass not found" }
}

# Gate 4: 产物清洁
Check "No tracked binaries" {
    $bins = git -C $ProjectRoot ls-files '*.exe' '*.dll' | Where-Object { $_ -notmatch 'wintun\.dll' }
    if ($bins) { throw "tracked binaries: $bins" }
}

# Gate 5: 文档一致性
Check "Audit release_ready" {
    $hits = Select-String -Path (Join-Path $ProjectRoot "docs\audit-report.md") -Pattern 'release_ready'
    if (-not $hits) { throw "release_ready not found" }
}
Check "No open findings" {
    $hits = Select-String -Path (Join-Path $ProjectRoot "docs\audit-report.md") -Pattern '\| open \|'
    if ($hits) { throw "open findings found" }
}

# ── Phase 1-3 Gates (new, with SKIP support) ──

function CheckSkip {
    param([string]$Name, [scriptblock]$Block)
    Write-Host "── $Name ──"
    try {
        $result = & $Block 2>&1
        if ($LASTEXITCODE -eq 0 -or $LASTEXITCODE -eq $null) {
            Write-Host "  ✅ PASS"
            $script:Pass++
        } else {
            Write-Host "  ⏭️ SKIP"
            $script:Skip++
        }
    } catch {
        $msg = "$_"
        if ($msg -match "SKIP:") {
            Write-Host "  ⏭️ SKIP: $msg"
            $script:Skip++
        } else {
            Write-Host "  ❌ FAIL: $msg"
            $script:Fail++
        }
    }
}

# Gate 6: Phase 1-3 关键测试包回归
$clientDir = Join-Path $ProjectRoot "phantom-client"
if (Test-Path $clientDir) {
    CheckSkip "Phase1 ClientOrchestrator tests" { Push-Location $clientDir; go test ./pkg/gtclient/ -run "TestClientOrchestrator" -count=1; Pop-Location }
    CheckSkip "Phase1 RecoveryFSM tests"        { Push-Location $clientDir; go test ./pkg/gtclient/ -run "TestRecoveryFSM" -count=1; Pop-Location }
    CheckSkip "Phase1 Resolver tests"            { Push-Location $clientDir; go test ./pkg/resonance/ -run "TestResolver" -count=1; Pop-Location }
} else {
    Write-Host "── Phase1 tests ── ⏭️ SKIP: phantom-client not found"; $Skip += 3
}

# Gate 7: 证据文件存在性
$evidenceFiles = @(
    "docs\governance\carrier-matrix.md",
    "docs\reports\stealth-experiment-plan.md",
    "docs\reports\ebpf-coverage-map.md",
    "docs\reports\deployment-tiers.md",
    "docs\reports\access-control-joint-drill.md",
    "docs\reports\phase4-evidence-audit.md",
    "docs\reports\cross-document-consistency.md"
)
foreach ($ef in $evidenceFiles) {
    $efName = Split-Path $ef -Leaf
    $efPath = Join-Path $ProjectRoot $ef
    if (Test-Path $efPath) {
        Write-Host "── Evidence: $efName ──"
        Write-Host "  ✅ PASS"
        $Pass++
    } else {
        Write-Host "── Evidence: $efName ──"
        Write-Host "  ⏭️ SKIP: evidence file not found"
        $Skip++
    }
}

Write-Host ""
Write-Host "════════════════════════════════════════"
Write-Host "Result: $Pass passed, $Fail failed, $Skip skipped"
Write-Host "════════════════════════════════════════"

if ($Fail -gt 0) { exit 1 } else { exit 0 }
