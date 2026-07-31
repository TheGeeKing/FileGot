# FileGot

FileGot is a Windows-first movie and TV episode renamer written in Go. It
matches local video filenames against TMDB, previews every proposed change,
renames files without overwriting existing data, and keeps the last batch
available for undo.

## Requirements

- Go 1.26 or newer
- A 64-bit C compiler for Fyne. On Windows, use the MSYS2 UCRT64 toolchain.
- A TMDB API Read Access Token

## Develop

Ensure `C:\msys64\ucrt64\bin` appears before older MinGW installations in
`PATH`, then run:

```powershell
go run ./cmd/filegot
```

Use this for day-to-day work. It is fast; About shows `FileGot dev` because
Fyne app metadata is only baked in by packaging.

Open **Settings** and enter a TMDB Read Access Token. **Test Connection**
validates it and loads TMDB's supported metadata languages.

## Build

Create a distributable Windows GUI executable with Fyne release metadata
(name, version, icon) and no console window:

```powershell
.\build-windows.ps1
```

This runs `fyne package --release` for `./cmd/filegot` and writes `FileGot.exe`
at the repo root. Use it for release artifacts, not the inner dev loop.

## Verify

```powershell
gofmt -w cmd internal
go vet ./...
go test ./...
go test -race ./...
.\build-windows.ps1
```

The first Fyne build compiles native graphics dependencies and can take
several minutes.

## Safety

FileGot validates the complete batch before renaming. It never overwrites an
unrelated file, stages changes through same-directory temporary names, rolls
back failures, and persists a journal for startup recovery and Undo Last.

This product uses the TMDB API but is not endorsed or certified by TMDB.
