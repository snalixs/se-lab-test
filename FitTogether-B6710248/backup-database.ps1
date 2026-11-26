$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot
$backups = Join-Path $root 'backups'
New-Item -ItemType Directory -Force -Path $backups | Out-Null
$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
$destination = Join-Path $backups ("fittogether-$stamp.dump")

Push-Location $root
try {
    $container = (& docker compose ps -q db).Trim()
    if ($LASTEXITCODE -ne 0 -or -not $container) { throw 'PostgreSQL container is not running' }
    & docker compose exec -T db pg_dump -U fittogether -d fittogether -Fc -f /tmp/fittogether-backup.dump
    if ($LASTEXITCODE -ne 0) { throw 'pg_dump failed' }
    & docker cp "${container}:/tmp/fittogether-backup.dump" $destination
    if ($LASTEXITCODE -ne 0) { throw 'docker cp failed' }
    $backup = Get-Item -LiteralPath $destination
    if ($backup.Length -le 0) { throw 'Backup is empty' }
    Write-Host "Backup saved to $($backup.FullName) ($($backup.Length) bytes)"
    $media = Join-Path $root 'media'
    if (Test-Path -LiteralPath $media) {
        $mediaBackup = Join-Path $backups "fittogether-$stamp-media"
        Copy-Item -LiteralPath $media -Destination $mediaBackup -Recurse -Force
        Write-Host "Images saved to $mediaBackup"
    }
}
finally { Pop-Location }
