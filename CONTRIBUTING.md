# Contributing Guidelines

Welcome to WhatsRook. Review the following guidelines before submitting issues or pull requests.

## 1. Development Management

All development workflows are managed through the [Taskfile](./Taskfile.yml):

| Task      | Command        | Description                                                         |
| :-------- | :------------- | :------------------------------------------------------------------ |
| `fmt`     | `task fmt`     | Format and vet all Go files (`go fmt`, `gofmt -s`, `go vet`).       |
| `test`    | `task test`    | Run the complete test suite across root, `wa-core`, and `cli`.      |
| `build`   | `task build`   | Compile the CLI binary into `bin/whatsrook`.                        |
| `fix`     | `task fix`     | Apply Go modernizers across all packages.                           |
| `install` | `task install` | Download and tidy all Go module dependencies.                       |
| `update`  | `task update`  | Upgrade all Go dependencies across modules.                         |
| `proto`   | `task proto`   | Compile and update protobuf definitions in `wa-core/proto`.         |
| `bump`    | `task bump`    | Bump release version date (`D.M.YY`) across version metadata files. |
| `clean`   | `task clean`   | Remove temporary build artifacts and binaries.                      |

## 2. Architecture & Guidelines

- **Architecture Guidelines**: Refer to [AGENTS.md](./AGENTS.md) for package layering and boundaries between `whatsrook`, `wa-core`, `utils`, and `cli`.
- **Code Quality**: Always run `task fmt` before opening a pull request.
- **Testing**: Ensure existing tests pass and add unit tests for new logic with `task test`.
- **Commits**: Follow conventional commit conventions (`feat:`, `fix:`, `refactor:`, `docs:`, `test:`).

## 3. Pull Request Workflow

1. Create a feature branch from `master`.
2. Implement changes following the architecture guidelines.
3. Validate locally with `task fmt`, `task test`, and `task build`.
4. Submit a Pull Request targeting the `master` branch.

## 4. Governance & Policies

- **Code of Conduct**: Review [CODE_OF_CONDUCT.md](./CODE_OF_CONDUCT.md) before participating in discussions or submitting PRs.
- **Security Policy**: For reporting vulnerabilities, consult [SECURITY.md](./SECURITY.md).
- **Disclaimer**: Review liability and educational use terms in [DISCLAIMER.md](./DISCLAIMER.md).
