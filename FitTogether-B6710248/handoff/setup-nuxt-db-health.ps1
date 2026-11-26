param([string]$ProjectPath = (Join-Path $env:USERPROFILE 'Documents\T17-FitTogether'))
$ErrorActionPreference = 'Stop'
$appPath = Join-Path $ProjectPath 'frontend\app\app.vue'
$routeDirectory = Join-Path $ProjectPath 'frontend\server\api'
$routePath = Join-Path $routeDirectory 'db-health.get.ts'

$app = @'
<script setup lang="ts">
const { data, error } = await useFetch('/api/db-health')
</script>

<template>
  <main>
    <h1>FitTogether</h1>
    <p v-if="error">ตรวจสอบฐานข้อมูลไม่สำเร็จ</p>
    <p v-else-if="data?.result === 1">เชื่อมต่อฐานข้อมูลสำเร็จ</p>
  </main>
</template>
'@

$route = @'
export default defineEventHandler(() => {
  return $fetch<{ result: number }>('http://127.0.0.1:8081/health/db')
})
'@

$existingApp = Get-Content -LiteralPath $appPath -Raw
$appMatches = ($existingApp -replace '\s+', ' ').Trim() -ceq ($app -replace '\s+', ' ').Trim()
$starterPattern = '^\s*<template>\s*<div>\s*<NuxtRouteAnnouncer\s*/>\s*<NuxtWelcome\s*/>\s*</div>\s*</template>\s*$'
if (-not $appMatches -and $existingApp -cnotmatch $starterPattern) {
    throw 'app.vue already has custom content.'
}

if (Test-Path -LiteralPath $routePath) {
    $existingRoute = Get-Content -LiteralPath $routePath -Raw
    if (($existingRoute -replace '\s+', ' ').Trim() -cne ($route -replace '\s+', ' ').Trim()) {
        throw 'db-health.get.ts already has different content.'
    }
} else {
    New-Item -ItemType Directory -Path $routeDirectory -Force | Out-Null
    Set-Content -LiteralPath $routePath -Value $route -Encoding utf8
}

if (-not $appMatches) {
    Set-Content -LiteralPath $appPath -Value $app -Encoding utf8
}

Write-Host 'Nuxt database check added. Refresh http://localhost:3000'
