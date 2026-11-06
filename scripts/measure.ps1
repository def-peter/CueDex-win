#Requires -Version 7
<#
.SYNOPSIS
    Measures what CueDex costs on this machine and how quickly a completion
    reaches the screen.

.DESCRIPTION
    Restarts CueDex with --background, samples it while idle, then writes one
    protocol event and times the overlay from the write to its first visible
    frame. CPU is reported as a percentage of one core, the unit HANDOFF.md
    uses.

    Every figure is a median of repeated samples except the peak RSS during the
    animation and the trigger-to-frame latency, which are single measurements.
    A busy machine produces noisy numbers, so run it more than once.

.EXAMPLE
    pwsh -File scripts/measure.ps1
#>
param(
    [string]$Exe = "",
    [int]$IdleSeconds = 10,
    [int]$SampleMs = 200
)

$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
if (-not $Exe) {
    $Exe = Join-Path $root 'build\windows-amd64\CueDex.exe'
}
if (-not (Test-Path $Exe)) {
    throw "CueDex.exe not found at $Exe; run 'go tool mygo build' first"
}

Add-Type @'
using System;
using System.Text;
using System.Runtime.InteropServices;
public class CueDexWindows {
    delegate bool EnumProc(IntPtr hwnd, IntPtr param);
    [DllImport("user32.dll")] static extern bool EnumWindows(EnumProc callback, IntPtr param);
    [DllImport("user32.dll")] static extern uint GetWindowThreadProcessId(IntPtr hwnd, out uint pid);
    [DllImport("user32.dll")] static extern int GetClassName(IntPtr hwnd, StringBuilder name, int size);
    [DllImport("user32.dll")] static extern bool IsWindowVisible(IntPtr hwnd);

    // The callback is a field, not a temporary: a delegate created for one call
    // can be collected while EnumWindows still holds it, which makes the
    // enumeration fail silently and reports a visible overlay as hidden.
    static readonly EnumProc callback = Visit;
    static uint target;
    static bool found;

    static bool Visit(IntPtr hwnd, IntPtr param) {
        uint owner;
        GetWindowThreadProcessId(hwnd, out owner);
        if (owner == target) {
            var name = new StringBuilder(64);
            GetClassName(hwnd, name, 64);
            if (name.ToString() == "CueDexOverlay" && IsWindowVisible(hwnd)) {
                found = true;
                return false;
            }
        }
        return true;
    }

    public static bool AnyVisible(uint pid) {
        target = pid;
        found = false;
        EnumWindows(callback, IntPtr.Zero);
        return found;
    }

    // WaitForOverlay polls in native code: a PowerShell callback costs tens of
    // milliseconds per scan, which would be larger than the latency itself.
    public static int WaitForOverlay(uint pid, int timeoutMs) {
        var watch = System.Diagnostics.Stopwatch.StartNew();
        while (watch.ElapsedMilliseconds < timeoutMs) {
            if (AnyVisible(pid)) return (int)watch.ElapsedMilliseconds;
            System.Threading.Thread.Sleep(1);
        }
        return -1;
    }
}
'@

function Get-Median {
    param([double[]]$Values)
    $sorted = $Values | Sort-Object
    return $sorted[[int]($sorted.Count / 2)]
}

$cores = [Environment]::ProcessorCount
$app = Get-Process CueDex -ErrorAction SilentlyContinue | Select-Object -First 1
if ($app) {
    Write-Host "Stopping the running CueDex ($($app.Id)) for a clean measurement"
    Get-Process CueDex -ErrorAction SilentlyContinue | Stop-Process -Force
    Start-Sleep -Milliseconds 800
}

Write-Host "Starting $Exe --background"
$process = Start-Process -FilePath $Exe -ArgumentList '--background' -PassThru
Start-Sleep -Seconds 3
$process = Get-Process -Id $process.Id

# Idle: cue count is zero, no window, no animation.
$idleRss = @()
$process.Refresh()
$idleCpuStart = $process.TotalProcessorTime
$deadline = (Get-Date).AddSeconds($IdleSeconds)
while ((Get-Date) -lt $deadline) {
    $process.Refresh()
    $idleRss += $process.WorkingSet64 / 1MB
    Start-Sleep -Milliseconds $SampleMs
}
$process.Refresh()
$idleCpuMs = ($process.TotalProcessorTime - $idleCpuStart).TotalMilliseconds
$idleRssMedian = [math]::Round((Get-Median $idleRss), 1)
$idleCpuPercent = [math]::Round($idleCpuMs / ($IdleSeconds * 1000) * 100, 2)

$events = Join-Path $env:APPDATA 'CueDex\Events'
New-Item -ItemType Directory -Force -Path $events | Out-Null
$id = [guid]::NewGuid().ToString('N')
$payload = @{
    source      = 'codex'
    eventID     = "measure-$id"
    sessionID   = 'measure'
    completedAt = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
} | ConvertTo-Json -Compress
$temporary = Join-Path $events ".measure-$id.tmp"
$final = Join-Path $events "cue-codex-$id.json"
Set-Content -Path $temporary -Value $payload -Encoding ascii -NoNewline

# The trigger is the producer's file write: a running app watches the directory,
# so the wake is not needed and starting a second process would only add the
# time PowerShell takes to create it.
Move-Item $temporary $final
$latencyMs = [CueDexWindows]::WaitForOverlay([uint32]$process.Id, 5000)
if ($latencyMs -lt 0) {
    Remove-Item $final -Force -ErrorAction SilentlyContinue
    Get-Process CueDex -ErrorAction SilentlyContinue | Stop-Process -Force
    throw "No overlay appeared within 5 s. Check that cues are enabled, not paused, and outside the quiet hours."
}
$watch = [Diagnostics.Stopwatch]::StartNew()

$animationCpu = @()
$animationRss = @()
if ($null -ne $latencyMs) {
    $process.Refresh()
    $animationCpuStart = $process.TotalProcessorTime
    while ($watch.ElapsedMilliseconds -lt 5000) {
        $process.Refresh()
        $animationRss += $process.WorkingSet64 / 1MB
        if (-not [CueDexWindows]::AnyVisible([uint32]$process.Id)) {
            break
        }
        Start-Sleep -Milliseconds 20
    }
    $animationMs = $watch.ElapsedMilliseconds
    $process.Refresh()
    $animationCpuMs = ($process.TotalProcessorTime - $animationCpuStart).TotalMilliseconds
    if ($animationMs -gt 0) {
        $animationCpu += [math]::Round($animationCpuMs / $animationMs * 100, 2)
    }
}

Remove-Item $final -Force -ErrorAction SilentlyContinue
Get-Process CueDex -ErrorAction SilentlyContinue | Stop-Process -Force

$animationCpuMedian = if ($animationCpu.Count -gt 0) { $animationCpu[0] } else { 0 }
$peakRss = [math]::Round(($animationRss | Measure-Object -Maximum).Maximum, 1)
$rssDelta = [math]::Round($peakRss - $idleRssMedian, 1)

# Budgets from HANDOFF.md sections 4 and 6.3, with room for a slower machine.
$limits = [ordered]@{
    'idle RSS'        = @{ Value = $idleRssMedian; Unit = 'MB'; Max = 60 }
    'idle CPU'        = @{ Value = $idleCpuPercent; Unit = '% of one core'; Max = 1.0 }
    'animation CPU'   = @{ Value = $animationCpuMedian; Unit = '% of one core'; Max = 8.0 }
    'RSS delta'       = @{ Value = $rssDelta; Unit = 'MB'; Max = 20 }
    'trigger to frame' = @{ Value = $latencyMs; Unit = 'ms'; Max = 100 }
}

Write-Host ""
Write-Host ("CueDex measurement on {0} logical cores" -f $cores)
Write-Host ("{0,-18} {1,10} {2,-20} {3,8} {4}" -f 'metric', 'value', 'unit', 'limit', 'result')
$failed = $false
foreach ($name in $limits.Keys) {
    $entry = $limits[$name]
    $pass = $entry.Value -le $entry.Max
    if (-not $pass) { $failed = $true }
    $result = if ($pass) { 'pass' } else { 'FAIL' }
    Write-Host ("{0,-18} {1,10} {2,-20} {3,8} {4}" -f $name, $entry.Value, $entry.Unit, $entry.Max, $result)
}
Write-Host ""

if ($failed) {
    Write-Host "One or more budgets were exceeded; run again to rule out a busy machine."
    exit 1
}
