# attendance-device

> **For educational purposes only.** This demo shows how an attendance terminal
> (example: **FingerTec Face ID 5**) can talk directly to your own system over the iclock
> "push" protocol, without middleware such as AWDMS. There is no authentication and all data
> is kept in memory. Do not use it as is in production.

Written in Go using only the standard library (no third-party dependencies).
Runs on **Windows** and **Linux** (and macOS).

## Module name

`com.smc.att` is a Java-style package name. Go modules are named like domains, so it is
reversed to **`smc.com/att`** (see `go.mod`). Internal packages live under `smc.com/att/internal/...`.

## Project layout

```
main.go                      entry point, reads settings from environment variables
internal/iclock/             endpoints called by the DEVICE (/iclock/*)
internal/api/                endpoints for your SYSTEM (/status, /command)
internal/store/              in-memory state: devices, command queues, attendance
internal/logging/            middleware that logs every request and response
openapi.yaml                 OpenAPI 3 spec (import into Bruno, Postman, Swagger UI)
scripts/simulate-device.sh   device simulator for Linux/macOS (curl)
scripts/simulate-device.ps1  device simulator for Windows (PowerShell)
scripts/build.sh             cross-compile Linux + Windows binaries (bash)
scripts/build.ps1            cross-compile Linux + Windows binaries (PowerShell)
```

## Requirements

- [Go 1.22 or newer](https://go.dev/dl/) to build from source.
- Nothing else is needed to run a built binary.

## Run

### Linux / macOS

```bash
go run .
# or build first
go build -o attendance-device .
./attendance-device
```

### Windows (PowerShell)

```powershell
go run .
# or build first
go build -o attendance-device.exe .
.\attendance-device.exe
```

The server listens on port `8081` by default. On Windows, allow the program through
Windows Defender Firewall when asked, otherwise the device cannot reach it.

### Settings

| Environment variable | Default | Meaning |
|---|---|---|
| `ATT_ADDR` | `:8081` | listen address |
| `ATT_LOG_FILE` | `logs/traffic.log` | log file (JSON, one entry per line) |
| `ATT_ALLOWED_SN` | empty (accept all) | comma-separated serial numbers allowed to connect |

```bash
# Linux / macOS
ATT_ADDR=:9000 ATT_ALLOWED_SN=ABC123,DEF456 ./attendance-device
```

```powershell
# Windows PowerShell
$env:ATT_ADDR = ":9000"; $env:ATT_ALLOWED_SN = "ABC123,DEF456"; .\attendance-device.exe
```

### Build binaries for both platforms

```bash
bash scripts/build.sh        # from Linux/macOS
```

```powershell
.\scripts\build.ps1          # from Windows
```

Both scripts write to `dist/`:

```
dist/attendance-device-linux-amd64
dist/attendance-device-linux-arm64
dist/attendance-device-windows-amd64.exe
```

## Try it without a real device

Start the server, then in another terminal run the simulator:

```bash
bash scripts/simulate-device.sh                 # Linux / macOS
```

```powershell
.\scripts\simulate-device.ps1                   # Windows
# if scripts are blocked: powershell -ExecutionPolicy Bypass -File .\scripts\simulate-device.ps1
```

It walks through the full flow: handshake, queuing commands, the device polling and
reporting results, a face scan pushing an attendance record, and finally reading the status.

## Open in Bruno

`openapi.yaml` describes every endpoint with examples.

1. In Bruno, choose **Import Collection** and pick **OpenAPI V3 Spec**.
2. Select `openapi.yaml` from this folder.
3. The collection has two folders: **system** (`/status`, `/command`) and **iclock** (the
   calls a device makes, so you can play the device by hand).

The base URL is `http://localhost:8081`; change it in the collection environment if needed.

## Setting up the Face ID 5

Menu **Comm. → Cloud Server Setting** (menu names vary by firmware):

- Server Address: IP of the computer running this demo
- Server Port: `8081`
- Make sure push/ADMS mode is enabled, then restart the device.

## Communication flow

The device **always** opens the connection and the server only answers. Commands from your
system therefore wait in a queue until the device comes to ask for them.

```
 System                    attendance-device                    Face ID 5
   |                              |   GET /iclock/cdata?options=all  |
   |                              |<---------------------------------|  1. handshake
   |                              |---- settings (Delay, Realtime) ->|
   | POST /command {reboot}       |                                  |
   |----------------------------->|  2. command queued               |
   |                              |   GET /iclock/getrequest         |
   |                              |<---------------------------------|  3. "any work?"
   |                              |---- C:1:REBOOT ----------------->|
   |                              |   POST /iclock/devicecmd         |
   |                              |<---- ID=1&Return=0&CMD=REBOOT ---|  4. result
   |                              |   POST /iclock/cdata?table=ATTLOG|
   |                              |<---- 1001  2026-11-02 08:01:00 --|  5. face scan
   | GET /status                  |                                  |
   |----------------------------->|  6. system reads status/records  |
```

## Endpoints

### `/iclock/*` (called by the device)

| Method and path | Purpose |
|---|---|
| `GET /iclock/cdata?SN=..&options=all` | handshake, server returns push settings |
| `POST /iclock/cdata?SN=..&table=ATTLOG` | device pushes attendance records (tab-separated lines) |
| `POST /iclock/cdata?SN=..&table=OPERLOG` | device operation log (logged only) |
| `GET /iclock/getrequest?SN=..` | device takes the next command, or gets `OK` if none |
| `POST /iclock/devicecmd?SN=..` | device reports a command result (`Return=0` = success) |
| any other `/iclock/...` | answers `OK` |

### `/status`

- `GET /status` lists all devices: `online` (seen in the last 90 seconds), pending commands, attendance count.
- `GET /status?sn=ABC123` shows one device and its 20 latest attendance records.

### `/command`

- `GET /command?sn=ABC123` shows command history (queued → sent → done/failed).
- `POST /command` queues a command:

```bash
curl -X POST http://localhost:8081/command -H "Content-Type: application/json" \
  -d '{"sn":"ABC123","action":"reboot"}'
```

| action | Extra fields | Command sent |
|---|---|---|
| `reboot` | | `REBOOT` |
| `info` | | `INFO` |
| `check` | | `CHECK` |
| `query_users` | | `DATA QUERY USERINFO` |
| `query_attlog` | `from`, `to` (`YYYY-MM-DD HH:MM:SS`) | `DATA QUERY ATTLOG ...` |
| `add_user` | `pin`, `name` | `DATA UPDATE USERINFO ...` |
| `delete_user` | `pin` (numeric only, `*` is blocked) | `DATA DELETE USERINFO ...` |

## Logging

Every request and response (method, path, query, body, status, duration) is written as JSON
to stdout and to `logs/traffic.log`. Example:

```json
{"level":"INFO","msg":"REQUEST","method":"GET","path":"/iclock/getrequest","query":"SN=ABC123","body":""}
{"level":"INFO","msg":"RESPONSE","method":"GET","path":"/iclock/getrequest","status":200,"body":"C:1:REBOOT","duration_ms":0}
```

## Limitations

- Data is lost when the server restarts (no database).
- `/status` and `/command` have no authentication; run it on a trusted internal network only.
- Biometric templates (face/finger) and photos are not handled.

## Disclaimer

This is an independent learning project. FingerTec and ZKTeco are trademarks of their
respective owners; this project is not affiliated with or endorsed by them.
