# Node Hunter 一键启动：后端 :8080 + Next.js :3000
$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $Root

Write-Host "==> building backend..." -ForegroundColor Cyan
go build -o node-hunter-server.exe ./cmd/server

# 释放端口（可选）
foreach ($port in 8080, 3000) {
  $conns = Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue
  foreach ($c in $conns) {
    Write-Host "killing pid $($c.OwningProcess) on :$port"
    Stop-Process -Id $c.OwningProcess -Force -ErrorAction SilentlyContinue
  }
}

Write-Host "==> starting API on :8080" -ForegroundColor Cyan
Start-Process -FilePath "$Root\node-hunter-server.exe" `
  -ArgumentList "-addr",":8080","-config","configs\config.yaml" `
  -WorkingDirectory $Root `
  -WindowStyle Minimized

Start-Sleep -Seconds 1

Write-Host "==> starting Next.js on :3000" -ForegroundColor Cyan
Set-Location "$Root\web"
if (-not (Test-Path "node_modules")) {
  npm install
}

Write-Host ""
Write-Host "打开浏览器: http://localhost:3000" -ForegroundColor Green
Write-Host "API 健康检查: http://127.0.0.1:8080/api/health" -ForegroundColor Green
Write-Host "按 Ctrl+C 只停前端；后端在最小化窗口中，可从任务管理器结束 node-hunter-server" -ForegroundColor Yellow
Write-Host ""

npm run dev -- -H 127.0.0.1 -p 3000
