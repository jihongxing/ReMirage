param(
    [string]$ProfileFamily = "chrome-win",
    [string]$OutDir = "artifacts\dpi-audit\baseline\chrome-win",
    [string[]]$Sites = @("google.com", "youtube.com", "cloudflare.com", "github.com", "wikipedia.org"),
    [int]$Count = 20,
    [string]$Chrome = "chrome.exe"
)

$ErrorActionPreference = "Stop"

if ($ProfileFamily -ne "chrome-win") {
    throw "This Windows runner only captures chrome-win; got $ProfileFamily"
}

if (-not (Get-Command tshark -ErrorAction SilentlyContinue)) {
    throw "tshark is required; install Wireshark with Npcap"
}

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

$stamp = (Get-Date).ToUniversalTime().ToString("yyyyMMddTHHmmssZ")
$pcap = Join-Path $OutDir "$ProfileFamily-$stamp.pcapng"
$meta = Join-Path $OutDir "capture-metadata.json"

$capture = Start-Process -FilePath "tshark" -ArgumentList @("-w", $pcap, "-f", "tcp port 443 or udp port 443") -PassThru -WindowStyle Hidden
Start-Sleep -Seconds 2

for ($i = 1; $i -le $Count; $i++) {
    foreach ($site in $Sites) {
        Start-Process -FilePath $Chrome -ArgumentList @("--headless=new", "--disable-gpu", "https://$site/?remirage_capture=$i") -WindowStyle Hidden -Wait -ErrorAction SilentlyContinue
    }
}

Start-Sleep -Seconds 2
Stop-Process -Id $capture.Id -ErrorAction SilentlyContinue

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
    interface = "default"
    network_conditions = "uncontrolled"
    captured_at = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
    pcapng = (Split-Path $pcap -Leaf)
}

$metadata | ConvertTo-Json -Depth 4 | Set-Content -Path $meta -Encoding UTF8
Write-Host "Captured $pcap"
