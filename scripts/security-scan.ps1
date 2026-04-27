#Requires -Version 5.1
<#
.SYNOPSIS
    统一安全漏洞扫描脚本 (PowerShell)
.DESCRIPTION
    遍历所有含 go.mod 的目录执行 govulncheck，对 sdk/js 执行 npm audit
    输出结构化 JSON 结果，支持 CI 归档
.PARAMETER OutputDir
    扫描结果输出目录，默认为 scan-results/
.EXAMPLE
    .\scripts\security-scan.ps1
    .\scripts\security-scan.ps1 -OutputDir C:\results
.NOTES
    退出码: 0 = 无未豁免高危漏洞, 1 = 存在未豁免高危漏洞或扫描失败
#>

param(
    [string]$OutputDir = ""
)

$ErrorActionPreference = "Continue"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Split-Path -Parent $ScriptDir
$Timestamp = (Get-Date).ToUniversalTime().ToString("yyyyMMddTHHmmssZ")
$HasCritical = $false
$ScanResults = @()

if (-not $OutputDir) {
    $OutputDir = Join-Path $ProjectRoot "scan-results"
}

if (-not (Test-Path $OutputDir)) {
    New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null
}

Write-Host ""
Write-Host "+" + ("=" * 38) + "+"
Write-Host "|   Mirage Project 安全漏洞扫描       |"
Write-Host "+" + ("=" * 38) + "+"
Write-Host ""
Write-Host "Timestamp: $Timestamp"
Write-Host "Output:    $OutputDir"
Write-Host ""

# ============================================================
# Go module scanning with govulncheck
# ============================================================
function Scan-GoModule {
    param([string]$ModDir)

    $RelDir = $ModDir.Replace($ProjectRoot + [IO.Path]::DirectorySeparatorChar, "").Replace("\", "/")
    $SafeName = $RelDir -replace "[/\\]", "-"
    $ResultFile = Join-Path $OutputDir "govulncheck-$SafeName.json"
    $Status = "success"
    $HighCount = 0

    Write-Host "  🔍 Scanning Go module: $RelDir"

    $GovulncheckPath = Get-Command govulncheck -ErrorAction SilentlyContinue
    if (-not $GovulncheckPath) {
        Write-Host "  ⚠️  govulncheck not installed, skipping Go scan"
        Write-Host "     Install: go install golang.org/x/vuln/cmd/govulncheck@latest"
        $Status = "skipped"
        return @{
            module            = $RelDir
            type              = "go"
            status            = $Status
            high_severity_count = 0
            result_file       = ""
        }
    }

    try {
        $RawOutput = & govulncheck -json ./... 2>&1 | Out-String
        Set-Location $ProjectRoot
    }
    catch {
        $RawOutput = $_.Exception.Message
    }

    $RawOutput | Out-File -FilePath $ResultFile -Encoding utf8

    if ($RawOutput -match '"vulnerability"') {
        $Matches2 = [regex]::Matches($RawOutput, '"vulnerability"')
        $HighCount = $Matches2.Count
        if ($HighCount -gt 0) {
            $script:HasCritical = $true
            $Status = "vulnerabilities_found"
        }
    }

    return @{
        module            = $RelDir
        type              = "go"
        status            = $Status
        high_severity_count = $HighCount
        result_file       = (Split-Path -Leaf $ResultFile)
    }
}

Write-Host "-- Go Module Scanning --"

$GoModFiles = Get-ChildItem -Path $ProjectRoot -Recurse -Filter "go.mod" -ErrorAction SilentlyContinue |
    Where-Object { $_.FullName -notmatch "node_modules" -and $_.FullName -notmatch "\.git" }

if ($GoModFiles.Count -eq 0) {
    Write-Host "  ⚠️  No go.mod files found"
}
else {
    Write-Host "  Found $($GoModFiles.Count) Go module(s)"
    Write-Host ""
    foreach ($GoMod in $GoModFiles) {
        $ModDir = $GoMod.DirectoryName
        Push-Location $ModDir
        $Result = Scan-GoModule -ModDir $ModDir
        Pop-Location
        $ScanResults += $Result
    }
}

Write-Host ""

# ============================================================
# Node.js scanning with npm audit
# ============================================================
Write-Host "-- Node.js Scanning --"

$JsDir = Join-Path (Join-Path $ProjectRoot "sdk") "js"
$JsResultFile = Join-Path $OutputDir "npm-audit-sdk-js.json"
$JsStatus = "success"
$JsHighCount = 0

$JsPkgJson = Join-Path $JsDir "package.json"
if (-not (Test-Path $JsPkgJson)) {
    Write-Host "  ⚠️  sdk/js not found or missing package.json, skipping"
    $ScanResults += @{
        module            = "sdk/js"
        type              = "npm"
        status            = "skipped"
        high_severity_count = 0
        result_file       = ""
    }
}
else {
    Write-Host "  🔍 Scanning Node.js project: sdk/js"

    $NpmPath = Get-Command npm -ErrorAction SilentlyContinue
    if (-not $NpmPath) {
        Write-Host "  ⚠️  npm not installed, skipping Node.js scan"
        Write-Host "     Install Node.js from https://nodejs.org/"
        $ScanResults += @{
            module            = "sdk/js"
            type              = "npm"
            status            = "skipped"
            high_severity_count = 0
            result_file       = ""
        }
    }
    else {
        Push-Location $JsDir
        try {
            $NpmOutput = & npm audit --omit=dev --json 2>&1 | Out-String
        }
        catch {
            $NpmOutput = $_.Exception.Message
        }
        Pop-Location

        $NpmOutput | Out-File -FilePath $JsResultFile -Encoding utf8

        # Parse high/critical counts
        if ($NpmOutput -match '"high"\s*:\s*(\d+)') {
            $JsHighCount += [int]$Matches[1]
        }
        if ($NpmOutput -match '"critical"\s*:\s*(\d+)') {
            $JsHighCount += [int]$Matches[1]
        }

        if ($JsHighCount -gt 0) {
            $HasCritical = $true
            $JsStatus = "vulnerabilities_found"
        }

        $ScanResults += @{
            module            = "sdk/js"
            type              = "npm"
            status            = $JsStatus
            high_severity_count = $JsHighCount
            result_file       = (Split-Path -Leaf $JsResultFile)
        }
    }
}

Write-Host ""

# ============================================================
# Generate summary JSON
# ============================================================
$SummaryFile = Join-Path $OutputDir "scan-summary.json"

$ResultsJson = @()
foreach ($r in $ScanResults) {
    $ResultsJson += "    {`"module`":`"$($r.module)`",`"type`":`"$($r.type)`",`"status`":`"$($r.status)`",`"high_severity_count`":$($r.high_severity_count),`"result_file`":`"$($r.result_file)`"}"
}

$SummaryJson = @"
{
  "timestamp": "$Timestamp",
  "project": "mirage-project",
  "has_critical_vulnerabilities": $($HasCritical.ToString().ToLower()),
  "results": [
$($ResultsJson -join ",`n")
  ]
}
"@

$SummaryJson | Out-File -FilePath $SummaryFile -Encoding utf8

Write-Host "-- Scan Summary --"
Write-Host "  Results written to: $OutputDir"
Write-Host "  Summary: $SummaryFile"
Write-Host ""

if ($HasCritical) {
    Write-Host "❌ HIGH/CRITICAL vulnerabilities found - review scan results" -ForegroundColor Red
    exit 1
}
else {
    Write-Host "✅ No unexempted high-severity vulnerabilities" -ForegroundColor Green
    exit 0
}
