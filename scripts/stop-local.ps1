$ErrorActionPreference='Stop'
$projectPath=(Resolve-Path (Join-Path $PSScriptRoot '..')).Path
Push-Location $projectPath
try {
    wsl -u root -- docker compose down
    if($LASTEXITCODE -ne 0){throw 'docker compose down failed'}
    $pidFile=Join-Path $projectPath 'tmp/keepalive.pid'
    if(Test-Path $pidFile){
        $helper=Get-CimInstance Win32_Process -Filter ("ProcessId="+(Get-Content $pidFile)) -ErrorAction SilentlyContinue
        if($helper -and $helper.CommandLine -like '*keep-campusclaw.sh*'){Stop-Process -Id $helper.ProcessId}
    }
} finally { Pop-Location }
Write-Output 'CampusClaw stopped; database and upload volumes preserved.'
