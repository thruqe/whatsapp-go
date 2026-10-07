# Contributing Guidelines

Welcome to whatsrook-sdk. These guidelines should be reviewed before submitting issues or pull requests.

## 1. Development Workflow

| Task | Command | Description |
| :--- | :--- | :--- |
| `fmt` | `cargo fmt --all` | Format all Rust source files. |
| `check` | `cargo clippy --all-targets -- -D warnings` | Lint with zero-warning policy. |
| `test` | `cargo test --all-features` | Run the complete test suite. |
| `doc` | `cargo doc --no-deps --open` | Build and open rustdoc locally. |
| `build` | `cargo build --release` | Compile the library in release mode. |

## 2. Architecture & Guidelines

- **Code Quality**: `cargo fmt` and `cargo clippy -- -D warnings` must pass before opening a pull request.
- **Testing**: Existing tests must pass, and unit tests should be added for new logic using `cargo test`.
- **Commits**: Conventional commit conventions must be followed (`feat:`, `fix:`, `refactor:`, `docs:`, `test:`).
- **Documentation**: Every public item must have a rustdoc comment. Examples in doc comments must compile (`cargo test --doc`).
- **Semver**: Public API changes must follow [Semantic Versioning](https://semver.org/). Breaking changes require a major version bump.

## 3. Pull Request Workflow

1. Create a feature branch from `master`.
2. Implement changes following the architecture guidelines.
3. Validate locally: `cargo fmt --all`, `cargo clippy --all-targets -- -D warnings`, `cargo test`, `cargo doc --no-deps`.
4. Update `CHANGELOG.md` under an `[Unreleased]` section.
5. Submit a Pull Request targeting the `master` branch.

## 4. Governance & Policies

- **Security Policy**: For reporting vulnerabilities, consult [SECURITY](./SECURITY.md).
