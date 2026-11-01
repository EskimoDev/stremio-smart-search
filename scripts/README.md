# Scripts

Build, packaging and test helpers. None of these ship inside the app.

| File | Used by | Purpose |
|---|---|---|
| `build.ps1` | Release workflow, local builds | Builds the binary for one OS and architecture, stamps the version with `-ldflags "-X main.version=..."`, then runs `tools/package` |
| `windows-installer.nsi` | Release workflow | NSIS script for the Windows `Setup.exe` |
| `smoke.ps1` | CI | Starts a built helper and checks the manifest and a search respond |
| `ui-test.mjs` | CI, local | Setup-page behaviour tests run with Node.js against a mocked status API |

Build one target locally (needs PowerShell 7 and Go):

```powershell
./scripts/build.ps1 -TargetOS windows -TargetArch amd64 -Version 0.1.0-preview.3
```
