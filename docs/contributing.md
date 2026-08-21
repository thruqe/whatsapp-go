# Contributing to WhatsRook

Thank you for your interest in contributing to WhatsRook!

## 1. Development Tasks

All development workflows are managed through the [Taskfile](../Taskfile.yml):

| Task      | Command        | Description                                                            |
| --------- | -------------- | ---------------------------------------------------------------------- |
| `fmt`     | `task fmt`     | Format and vet all Go codebase files (`go fmt`, `gofmt -s`, `go vet`). |
| `test`    | `task test`    | Run the complete test suite across root, `wa-core`, and `cli`.         |
| `build`   | `task build`   | Compile the CLI binary into `bin/whatsrook`.                           |
| `bump`    | `task bump`    | Bump release version date (`D.M.YY`) across version metadata files.    |
| `proto`   | `task proto`   | Compile and update protobuf definitions in `wa-core/proto`.            |
| `install` | `task install` | Download and tidy all Go module dependencies.                          |

## 2. Code Guidelines & Architecture

- **Architecture Guidelines**: Refer to [AGENTS.md](../AGENTS.md) for the core package layout and abstraction boundaries between `whatsrook`, `wa-core`, `utils`, and `cli`.
- **Formatting**: Always run `task fmt` before opening a pull request.
- **Testing**: Ensure existing tests pass and add unit tests for new features using `task test`.

## 3. Governance, Security & Liability

- **Code of Conduct**: Please review the [Code of Conduct](../CODE_OF_CONDUCT.md) before participating in discussions or submitting contributions.
- **Security Policy**: For reporting vulnerabilities, consult the [Security Policy](../SECURITY.md).
- **Disclaimer**: Review the project liability terms in [DISCLAIMER.md](../DISCLAIMER.md).
