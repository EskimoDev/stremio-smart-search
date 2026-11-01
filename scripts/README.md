# Build and test scripts

Run these from the repository root. They are development helpers, not app runtime requirements.

| File | Purpose |
|---|---|
| [build.ps1](build.ps1) | Compile one OS/architecture, stamp its version and run the archive packager |
| [windows-installer.nsi](windows-installer.nsi) | Build a Windows installer from an already compiled binary using NSIS |
| [smoke.ps1](smoke.ps1) | Start a built helper with a handwritten fixture, check the manifest and three searches, then stop it |
| [ui-test.mjs](ui-test.mjs) | Test settings-page behaviour with Node.js and a mocked API |

## Build an archive

Requires Go and PowerShell 7. For example:

```powershell
./scripts/build.ps1 -TargetOS windows -TargetArch amd64 -Version 0.1.0-preview.3
```

Targets are `windows`, `darwin` and `linux`; architectures are `amd64` and `arm64`. Outputs go in `dist/`. Use `-GoExecutable` if Go is not on your PATH.

This command creates the binary and portable packages. It does **not** create a Windows installer or a macOS DMG: the [release workflow](../.github/workflows/release.yml) adds NSIS and native macOS signing/DMG steps.

## Check a built helper

```powershell
./scripts/smoke.ps1 -Binary ./dist/smart-search-windows-amd64.exe
```

The smoke test uses an isolated temporary data folder and port 11498. It does not download a real catalogue or connect to your Stremio account.

For regular source checks and release preparation, see [Contributing](../CONTRIBUTING.md).
