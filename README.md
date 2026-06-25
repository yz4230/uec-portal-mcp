# go-cli-template

A minimal template for building Go command-line applications with Cobra and structured logging via `slog` + `tint`.

## Features

- Cobra-based CLI entrypoint
- Global `--verbose` / `-v` flag
- Structured logging with `log/slog`
- Colorized terminal log output with `tint`
- Small, easy-to-extend project layout

## Project Structure

```text
.
├── cmd/
│   └── root.go
├── main.go
├── go.mod
└── go.sum
```

## Requirements

- Go 1.26 or later

## Getting Started

Clone the repository and run the CLI:

```bash
go run .
```

Run with verbose logging enabled:

```bash
go run . --verbose
```

Build a binary:

```bash
go build -o mycli .
./mycli --verbose
```

## Docker Tasks

Docker helper tasks are provided via mise:

```bash
mise run docker:build
mise run docker:run
mise run docker:stop
mise run docker:rm
```

The server listens on `PORT`, which defaults to `8080`.
For local runs, the port can also be set with `--port`:

```bash
go run . serve --http --port 18080
```

The tasks default to `IMAGE_NAME=uec-portal-mcp`,
`CONTAINER_NAME=uec-portal-mcp`, `HOST_PORT=$PORT`, and
`CONTAINER_PORT=$PORT`. Override them as needed:

```bash
PORT=18080 CONTAINER_NAME=uec-portal-mcp-dev mise run docker:run
```

## What This Template Sets Up

The root command configures a default logger in `PersistentPreRun`:

- standard log level by default
- debug-level logging with `--verbose`
- source locations in log output
- time-only timestamps for readable terminal logs

## Extending the Template

Typical next steps:

1. Add subcommands under `cmd/`
2. Define application-specific flags
3. Replace the module path in `go.mod`
4. Add business logic and tests

## Dependencies

- [`github.com/spf13/cobra`](https://github.com/spf13/cobra)
- [`github.com/lmittmann/tint`](https://github.com/lmittmann/tint)

## License

Add a license that matches how you want to share this template.
