$ErrorActionPreference = 'Stop'
$root = $PSScriptRoot
$backend = Join-Path $root 'backend'
$frontend = Join-Path $root 'frontend'
$runtime = Join-Path $root '.runtime'
$apiProcess = $null

$env:DATABASE_URL = 'postgres://fittogether:fittogether_dev@127.0.0.1:55432/fittogether?sslmode=disable'
$env:APP_ENV = 'development'
$env:API_BASE = 'http://127.0.0.1:8080'
$env:MEDIA_DIR = Join-Path $root 'media'

Push-Location $root
try {
    & docker info --format '{{.ServerVersion}}' *> $null
    if ($LASTEXITCODE -ne 0) {
        Write-Host 'Starting Docker Desktop...'
        & docker desktop start
        $dockerReady = $false
        for ($attempt = 1; $attempt -le 45; $attempt++) {
            Start-Sleep -Seconds 2
            & docker info --format '{{.ServerVersion}}' *> $null
            if ($LASTEXITCODE -eq 0) {
                $dockerReady = $true
                break
            }
            if ($attempt % 5 -eq 0) { Write-Host 'Waiting for Docker Engine...' }
        }
        if (-not $dockerReady) {
            throw 'Docker Engine is not ready. Open Docker Desktop, wait for Engine running, then run start-dev.ps1 again.'
        }
    }

    & docker compose up -d --wait db
    if ($LASTEXITCODE -ne 0) { throw 'PostgreSQL did not start' }

    Push-Location $backend
    try {
        & go run . -migrate
        if ($LASTEXITCODE -ne 0) { throw 'Database migration failed' }
        New-Item -ItemType Directory -Force -Path $runtime | Out-Null
        & go build -o (Join-Path $runtime 'fittogether-api.exe') .
        if ($LASTEXITCODE -ne 0) { throw 'Backend build failed' }
    }
    finally { Pop-Location }

    $apiReady = $false
    $apiPort = 8080
    try {
        $health = Invoke-RestMethod 'http://127.0.0.1:8080/healthz' -TimeoutSec 2
        if ($health.status -eq 'ok') {
            if ($health.image_upload -eq 'multipart-10') {
                $apiReady = $true
            }
            else {
                $apiPort = 8081
                Write-Host 'An older API is using port 8080. Starting the updated API on port 8081.'
            }
        }
    }
    catch {}
    $env:HTTP_ADDR = "127.0.0.1:$apiPort"
    $env:API_BASE = "http://127.0.0.1:$apiPort"
    $apiHealthUrl = "$env:API_BASE/healthz"
    if (-not $apiReady) {
        $apiProcess = Start-Process -FilePath (Join-Path $runtime 'fittogether-api.exe') -WorkingDirectory $backend -WindowStyle Hidden -PassThru
        for ($attempt = 0; $attempt -lt 20; $attempt++) {
            Start-Sleep -Seconds 1
            try {
                $health = Invoke-RestMethod $apiHealthUrl -TimeoutSec 2
                if ($health.status -eq 'ok' -and $health.image_upload -eq 'multipart-10') {
                    $apiReady = $true
                    break
                }
            }
            catch {}
            if ($apiProcess.HasExited) { break }
        }
    }
    if (-not $apiReady) { throw "Updated Backend did not start on port $apiPort" }

    Push-Location $frontend
    try {
        if (-not (Test-Path -LiteralPath 'node_modules')) {
            & npm.cmd ci
            if ($LASTEXITCODE -ne 0) { throw 'Frontend package installation failed' }
        }
        Write-Host 'FitTogether is ready at http://127.0.0.1:3000'
        & npm.cmd run dev
        if ($LASTEXITCODE -ne 0) { throw 'Frontend stopped with an error' }
    }
    finally { Pop-Location }
}
finally {
    if ($apiProcess -and -not $apiProcess.HasExited) {
        Stop-Process -Id $apiProcess.Id
    }
    Pop-Location
}
