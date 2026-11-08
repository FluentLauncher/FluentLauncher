$ErrorActionPreference = 'Stop'
go build -trimpath -ldflags "-s -w" -o resources/examples/official.echo/echo.exe ./plugins/echo
go build -trimpath -ldflags "-s -w" -o resources/examples/official.devtools/devtools.exe ./plugins/devtools
go build -trimpath -ldflags "-s -w" -o resources/examples/official.clipboard/clipboard.exe ./plugins/clipboard
Write-Output "official plugins rebuilt"
