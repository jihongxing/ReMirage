param(
    [string]$ProfileFamily = "chrome-win",
    [string]$OutDir = "artifacts\dpi-audit\baseline\chrome-win",
    [string[]]$Sites = @("cloudflare.com", "github.com", "wikipedia.org", "example.com"),
    [int]$Count = 20,
    [string]$Chrome = "chrome.exe",
    [int]$TimeoutSeconds = 10,
    [string]$Interface = ""
)

$ErrorActionPreference = "Stop"

if ($ProfileFamily -ne "chrome-win") {
    throw "This Windows runner only captures chrome-win; got $ProfileFamily"
}

if (-not (Get-Command tshark -ErrorAction SilentlyContinue)) {
    throw "tshark is required; install Wireshark with Npcap"
}

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
$OutDir = (Resolve-Path -LiteralPath $OutDir).Path

$stamp = (Get-Date).ToUniversalTime().ToString("yyyyMMddTHHmmssZ")
$pcap = Join-Path $OutDir "$ProfileFamily-$stamp.pcapng"
$meta = Join-Path $OutDir "capture-metadata.json"
$tsharkStdout = Join-Path $OutDir "tshark-stdout.log"
$tsharkStderr = Join-Path $OutDir "tshark-stderr.log"

$captureArgs = ""
if ($Interface -ne "") {
    $captureArgs += "-i `"$Interface`" "
}
$captureArgs += "-f `"tcp port 443 or udp port 443`" -w `"$pcap`""

$capture = Start-Process `
    -FilePath "tshark" `
    -ArgumentList $captureArgs `
    -PassThru `
    -WindowStyle Hidden `
    -RedirectStandardOutput $tsharkStdout `
    -RedirectStandardError $tsharkStderr
Start-Sleep -Seconds 2

if ($capture.HasExited) {
    $stderr = ""
    if (Test-Path -LiteralPath $tsharkStderr) {
        $stderr = Get-Content -LiteralPath $tsharkStderr -Raw -ErrorAction SilentlyContinue
    }
    throw "tshark exited before capture started. Interface='$Interface'. Args='$captureArgs'. Error='$stderr'"
}

for ($i = 1; $i -le $Count; $i++) {
    foreach ($site in $Sites) {
        $args = @(
            "--headless=new",
            "--disable-gpu",
            "--disable-quic",
            "--user-data-dir=$env:TEMP\remirage-chrome-m13",
            "https://$site/?remirage_capture=$i"
        )
        $browser = Start-Process -FilePath $Chrome -ArgumentList $args -WindowStyle Hidden -PassThru -ErrorAction SilentlyContinue
        if ($browser) {
            $exited = $browser.WaitForExit($TimeoutSeconds * 1000)
            if (-not $exited) {
                Stop-Process -Id $browser.Id -Force -ErrorAction SilentlyContinue
                Start-Sleep -Milliseconds 500
            }
        }
    }
}

Start-Sleep -Seconds 2
Stop-Process -Id $capture.Id -ErrorAction SilentlyContinue
Wait-Process -Id $capture.Id -Timeout 5 -ErrorAction SilentlyContinue

if (-not (Test-Path -LiteralPath $pcap)) {
    $stderr = ""
    if (Test-Path -LiteralPath $tsharkStderr) {
        $stderr = Get-Content -LiteralPath $tsharkStderr -Raw -ErrorAction SilentlyContinue
    }
    throw "capture failed: pcapng was not created at $pcap. tshark stderr: $stderr"
}

$pcapInfo = Get-Item -LiteralPath $pcap
if ($pcapInfo.Length -le 0) {
    throw "capture failed: pcapng is empty at $pcap"
}

$chromeVersion = ""
try { $chromeVersion = (& $Chrome --version 2>$null | Select-Object -First 1) } catch {}

$metadata = [ordered]@{
    profile_family = $ProfileFamily
    native_os = $true
    os = "windows"
    os_version = (Get-CimInstance Win32_OperatingSystem).Version
    browser = "Google Chrome"
    browser_version = $chromeVersion
    capture_tool = "tshark+Npcap"
    interface = $(if ($Interface -ne "") { $Interface } else { "default" })
    network_conditions = "uncontrolled"
    captured_at = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    pcapng = (Split-Path $pcap -Leaf)
}

$metadataJson = $metadata | ConvertTo-Json -Depth 4
[System.IO.File]::WriteAllText($meta, $metadataJson, [System.Text.UTF8Encoding]::new($false))
Write-Host "Captured $pcap"
