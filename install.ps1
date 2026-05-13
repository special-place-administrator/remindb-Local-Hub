#Requires -Version 5.1
[CmdletBinding()]
param(
	[string]$Prefix = (Join-Path $env:LOCALAPPDATA 'Programs\remindb'),
	[switch]$Help
)

$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

if ($PSVersionTable.PSVersion.Major -lt 5) {
	throw "remindb installer requires PowerShell 5.1 or newer; current version is $($PSVersionTable.PSVersion)"
}

function Get-Sha256Hex {
	param([Parameter(Mandatory)][string]$Path)
	$sha = [System.Security.Cryptography.SHA256]::Create()
	try {
		$stream = [System.IO.File]::OpenRead($Path)
		try {
			$bytes = $sha.ComputeHash($stream)
		} finally {
			$stream.Dispose()
		}
		return ([System.BitConverter]::ToString($bytes) -replace '-','').ToLower()
	} finally {
		$sha.Dispose()
	}
}

$repo = 'special-place-administrator/remindb-Local-Hub'

function Show-Usage {
	@"
Usage: install.ps1 [-Prefix PATH]

Download and install the latest remindb release from GitHub.

Options:
  -Prefix PATH   Install root; binary is placed at PATH\bin\remindb.exe.
                 Default: %LOCALAPPDATA%\Programs\remindb
  -Help          Show this help.
"@
}

if ($Help) {
	Show-Usage
	exit 0
}

function Get-Architecture {
	if (-not [System.Environment]::Is64BitOperatingSystem) {
		throw '32-bit Windows is not supported'
	}
	if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64' -or $env:PROCESSOR_ARCHITEW6432 -eq 'ARM64') {
		return 'arm64'
	}
	return 'x86_64'
}

$arch = Get-Architecture

Write-Host "Resolving latest release for $repo..."
$headers = @{ 'User-Agent' = 'remindb-install' }
$release = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest" -Headers $headers
$tag = $release.tag_name
if (-not $tag) {
	throw 'failed to resolve latest release tag'
}

$version = $tag.TrimStart('v')
$archive = "remindb_${version}_Windows_${arch}.zip"
$downloadUrl = "https://github.com/$repo/releases/download/$tag/$archive"
$checksumsUrl = "https://github.com/$repo/releases/download/$tag/checksums.txt"

$tmpdir = Join-Path $env:TEMP "remindb-$([guid]::NewGuid())"
New-Item -ItemType Directory -Path $tmpdir -Force | Out-Null

try {
	$archivePath = Join-Path $tmpdir $archive
	Write-Host "Downloading $archive..."
	Invoke-WebRequest -Uri $downloadUrl -OutFile $archivePath

	Write-Host 'Verifying checksum...'
	$checksumsPath = Join-Path $tmpdir 'checksums.txt'
	Invoke-WebRequest -Uri $checksumsUrl -OutFile $checksumsPath

	$expected = $null
	foreach ($line in Get-Content $checksumsPath) {
		$parts = $line -split '\s+', 2
		if ($parts.Count -eq 2 -and $parts[1].Trim() -eq $archive) {
			$expected = $parts[0].Trim().ToLower()
			break
		}
	}
	if (-not $expected) {
		throw "$archive not listed in checksums.txt"
	}

	$actual = Get-Sha256Hex -Path $archivePath
	if ($actual -ne $expected) {
		throw "checksum mismatch for ${archive}: expected $expected, got $actual"
	}

	$binDir = Join-Path $Prefix 'bin'
	$binPath = Join-Path $binDir 'remindb.exe'
	New-Item -ItemType Directory -Path $binDir -Force | Out-Null

	Write-Host "Installing to $binPath..."
	Expand-Archive -Path $archivePath -DestinationPath $tmpdir -Force

	$srcExe = Join-Path $tmpdir 'remindb.exe'
	$lastErr = $null
	$copied = $false
	foreach ($attempt in 1..10) {
		try {
			if (Test-Path $binPath) { Remove-Item -Path $binPath -Force -ErrorAction Stop }
			Copy-Item -Path $srcExe -Destination $binPath -Force -ErrorAction Stop
			$copied = $true
			break
		} catch {
			$lastErr = $_
			Start-Sleep -Milliseconds 500
		}
	}

	if (-not $copied) {
		$svc = Get-Service -Name 'remindb-singleton' -ErrorAction SilentlyContinue
		$svcHint = if ($svc -and $svc.Status -eq 'Running') {
			"`n  The 'remindb-singleton' Windows Service is currently running and holds $binPath open.`n  Stop it first (admin shell): remindb service stop`n  Then re-run this installer, and restart it: remindb service start"
		} else {
			"`n  Some process is holding $binPath open. Check Get-Process remindb and stop any holders."
		}
		throw "failed to install after 10 attempts: $lastErr$svcHint"
	}

	Write-Host "Installed: $binPath ($tag)"

	$onPath = ($env:PATH -split ';') | Where-Object { $_ -ieq $binDir }
	if (-not $onPath) {
		Write-Host ''
		Write-Host "Note: $binDir is not on your PATH. Add it persistently with:"
		Write-Host "  [Environment]::SetEnvironmentVariable('Path', [Environment]::GetEnvironmentVariable('Path','User') + ';$binDir', 'User')"
	}
}
finally {
	Remove-Item -Path $tmpdir -Recurse -Force -ErrorAction SilentlyContinue
}
