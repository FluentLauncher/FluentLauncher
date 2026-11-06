$ErrorActionPreference = 'Stop'
go build -trimpath -ldflags "-s -w" -o resources/examples/official.echo/echo.exe ./plugins/echo
go build -trimpath -ldflags "-s -w" -o resources/examples/official.devtools/devtools.exe ./plugins/devtools
Write-Output "official plugins rebuilt"
