$ErrorActionPreference = "Stop"

fyne package --release --source-dir ./cmd/filegot
if ($LASTEXITCODE -ne 0) {
    exit $LASTEXITCODE
}

Move-Item -Force ./cmd/filegot/FileGot.exe ./FileGot.exe
