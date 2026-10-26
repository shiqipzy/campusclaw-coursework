$ErrorActionPreference = 'Stop'
$projectPath = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$linuxPath = '/mnt/' + $projectPath.Substring(0,1).ToLower() + $projectPath.Substring(2).Replace('\','/')
$runDir = Join-Path $projectPath 'tmp'
New-Item -ItemType Directory -Force $runDir | Out-Null
$helperScript = Join-Path $runDir 'keep-campusclaw.sh'
$linuxHelper = $linuxPath + '/tmp/keep-campusclaw.sh'
$scriptText = "#!/bin/sh`ncd '$linuxPath' || exit 1`ndocker compose up --build -d || exit 1`ndocker compose exec -T web nginx -s reload`nsleep infinity`n"
[System.IO.File]::WriteAllText($helperScript,$scriptText,[System.Text.UTF8Encoding]::new($false))
$pidFile=Join-Path $runDir 'keepalive.pid'
if(Test-Path $pidFile){
    $previous=Get-CimInstance Win32_Process -Filter ("ProcessId="+(Get-Content $pidFile)) -ErrorAction SilentlyContinue
    if($previous -and $previous.CommandLine -like '*keep-campusclaw.sh*'){Write-Output 'CampusClaw helper is already running: http://localhost:8080';exit 0}
}
$helper=Start-Process -FilePath 'wsl.exe' -ArgumentList @('-u','root','--','sh',$linuxHelper) -WindowStyle Hidden -PassThru -RedirectStandardOutput (Join-Path $runDir 'startup.log') -RedirectStandardError (Join-Path $runDir 'startup-error.log')
$helper.Id | Set-Content $pidFile
Write-Output ('CampusClaw is starting at http://localhost:8080; helper PID '+$helper.Id)
