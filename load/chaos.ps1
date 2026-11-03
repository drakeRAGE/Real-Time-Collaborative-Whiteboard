# Restarts the board container while strokes are published on wb.board.stroke.v1
# and checks BoardService.GetHistory. Three consecutive runs must keep the count.
# The probe runs inside the compose network (golang image) because this host
# blocks newly built executables.
# Continue: Windows PowerShell turns native stderr into a terminating error when Stop is set.
$ErrorActionPreference = 'Continue'

$Root = Split-Path -Parent $PSScriptRoot
$Compose = Join-Path $Root 'deploy\compose\docker-compose.yml'
$Secret = $null
foreach ($line in Get-Content -LiteralPath $Compose) {
    if ($line -match '^\s*SUPABASE_JWT_SECRET:\s*(\S+)\s*$') {
        $Secret = $Matches[1].Trim('"').Trim("'")
        break
    }
}
if (-not $Secret) {
    throw 'SUPABASE_JWT_SECRET is not set in deploy/compose/docker-compose.yml'
}

$jwtFile = Join-Path $env:TEMP 'wb-harness-jwt.txt'
$linuxBin = Join-Path $Root 'load\harness\harness-linux'
Set-Content -LiteralPath $jwtFile -Value $Secret -NoNewline -Encoding ascii

function Invoke-Harness {
    param([Parameter(Mandatory = $true)][string[]]$HarnessArgs, [string]$Name)
    $runArgs = @(
        'run', '--rm', '--network', 'whiteboard_default',
        '-v', "${Root}:/src",
        '-v', "${env:TEMP}:/hosttmp",
        '-w', '/src/load/harness',
        '-e', 'HARNESS_JWT_SECRET_FILE=/hosttmp/wb-harness-jwt.txt',
        '-e', 'GATEWAY_URL=http://gateway:8080',
        '-e', 'NATS_URL=nats://nats:4222',
        '-e', 'BOARD_GRPC=board:7002',
        '-e', 'DATABASE_URL=postgres://postgres:postgres@postgres:5432/whiteboard?sslmode=disable'
    )
    if ($Name) { $runArgs += @('--name', $Name) }
    $runArgs += @('golang:1.26-alpine', '/src/load/harness/harness-linux')
    $runArgs += $HarnessArgs
    $script:HarnessOutput = & docker @runArgs
    $script:HarnessExit = $LASTEXITCODE
}

try {
    if (Test-Path -LiteralPath $linuxBin) { Remove-Item -LiteralPath $linuxBin -Force }
    Write-Host 'building chaos probe in the golang image'
    & docker run --rm --network whiteboard_default `
        -v "${Root}:/src" `
        -v "C:\Users\Interloop12\go\pkg\mod:/go/pkg/mod" `
        -w /src/load/harness `
        -e GOWORK=off -e CGO_ENABLED=0 `
        golang:1.26-alpine `
        go build -mod=readonly -o /src/load/harness/harness-linux .
    if ($LASTEXITCODE -ne 0) { throw "harness build failed ($LASTEXITCODE)" }

    $board = (& docker compose -f $Compose ps -q board).Trim()
    if (-not $board) { throw 'board container is not running' }

    $runs = 3
    $strokes = 40
    $failed = 0
    for ($i = 1; $i -le $runs; $i++) {
        $statusName = "wb-chaos-$i.json"
        $status = Join-Path $env:TEMP $statusName
        if (Test-Path -LiteralPath $status) { Remove-Item -LiteralPath $status -Force }
        $pubName = "wb-chaos-pub-$i"
        cmd /c "docker rm -f $pubName >nul 2>&1"
        Write-Host "chaos run ${i} of ${runs}: publishing $strokes strokes"
        & docker run -d --name $pubName --network whiteboard_default `
            -v "${Root}:/src" `
            -v "${env:TEMP}:/hosttmp" `
            -w /src/load/harness `
            -e HARNESS_JWT_SECRET_FILE=/hosttmp/wb-harness-jwt.txt `
            -e GATEWAY_URL=http://gateway:8080 `
            -e NATS_URL=nats://nats:4222 `
            -e BOARD_GRPC=board:7002 `
            -e DATABASE_URL=postgres://postgres:postgres@postgres:5432/whiteboard?sslmode=disable `
            golang:1.26-alpine `
            /src/load/harness/harness-linux -mode publish -strokes "$strokes" -gap 150ms -status "/hosttmp/$statusName" | Out-Null
        if ($LASTEXITCODE -ne 0) { throw "could not start publish container on run $i" }

        $meta = $null
        $deadline = (Get-Date).AddSeconds(40)
        while ((Get-Date) -lt $deadline) {
            if (Test-Path -LiteralPath $status) {
                try {
                    $meta = Get-Content -LiteralPath $status -Raw | ConvertFrom-Json
                    if ($meta.phase -eq 'publishing' -or $meta.phase -eq 'done') { break }
                } catch {
                    Start-Sleep -Milliseconds 50
                }
            }
            $running = docker inspect -f '{{.State.Running}}' $pubName 2>$null
            if ($running -eq 'false') { break }
            Start-Sleep -Milliseconds 100
        }
        if (-not $meta.roomId) {
            Write-Host (docker logs $pubName 2>&1 | Out-String)
            throw "run $i did not start publishing"
        }
        Write-Host "chaos run ${i}: docker restart board during ingest (room $($meta.roomId))"
        & docker restart -t 5 $board
        if ($LASTEXITCODE -ne 0) { throw "docker restart failed on run $i" }
        $pubExit = docker wait $pubName
        $logs = docker logs $pubName 2>&1 | Out-String
        & docker rm $pubName | Out-Null
        if ($logs.Trim()) { Write-Host $logs.Trim() }
        if ("$pubExit".Trim() -ne '0') { throw "publish exited $pubExit on run $i" }

        $healthy = $false
        $healthDeadline = (Get-Date).AddSeconds(90)
        while ((Get-Date) -lt $healthDeadline) {
            try {
                $res = Invoke-WebRequest -Uri 'http://localhost:8002/healthz' -UseBasicParsing -TimeoutSec 2
                if ($res.StatusCode -eq 200) { $healthy = $true; break }
            } catch {
                Start-Sleep -Seconds 2
            }
        }
        if (-not $healthy) { throw "board healthz did not return 200 after run $i" }

        Invoke-Harness -HarnessArgs @('-mode', 'count', '-room', $meta.roomId, '-user', $meta.userId, '-expect', "$strokes", '-wait', '60s')
        $countExit = $script:HarnessExit
        $count = @($script:HarnessOutput) | Select-Object -Last 1
        $count = "$count".Trim()
        Write-Host "chaos run ${i}: GetHistory count $count want $strokes (exit $countExit)"
        if ($countExit -ne 0 -or $count -ne "$strokes") { $failed++ }
    }

    if ($failed -gt 0) {
        Write-Host "FAIL chaos $failed of $runs runs lost strokes"
        exit 1
    }
    Write-Host "PASS chaos $runs runs, GetHistory count stayed $strokes"
    exit 0
}
finally {
    if (Test-Path -LiteralPath $jwtFile) { Remove-Item -LiteralPath $jwtFile -Force }
    if (Test-Path -LiteralPath $linuxBin) { Remove-Item -LiteralPath $linuxBin -Force }
}
