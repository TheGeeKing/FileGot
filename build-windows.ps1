$ErrorActionPreference = "Stop"

go build -trimpath -ldflags="-H windowsgui" -o FileGot.exe ./cmd/filegot
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}
