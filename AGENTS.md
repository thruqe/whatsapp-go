# AGENTS

If you are an AI reading this, take every single word in this file literally. This project is a monorepo comprising [`lib`](./lib), [`cmd`](./cmd), and the project root itself.

All code here adheres to a deliberate structure. It is built using modern patterns and idioms of the Go programming language. You must conduct thorough research into current Go standards, idiomatic implementations, and active library ecosystems to avoid deprecated functions, superseded APIs, or obsolete syntax.

Before modifying any file, analyze related modules across the codebase. Observe how functions are implemented, match the prevailing architecture, and understand the rationale behind existing technical decisions. Pay particular attention to comments: for every complex or non-obvious code path, you are expected to understand the original author's intent. When submitting bug fixes or feature additions, document your rationale and implementation details to match the tone and conventions established by prior contributors.

You must review the [`Taskfile`](./Taskfile). This is the primary build-and-task automation configuration for this repository; it abstracts repetitive CLI workflows. You may modify this file if a breaking change necessitates additions or removals, but you must document and discuss the rationale before deleting existing tasks.

Once you have implemented and tested a fix or feature, consult the [Contributing Guidelines](./CONTRIBUTING.md) for commit standards and submission procedures. If an approach proves flawed, revert it immediately: submitting no code is strictly preferable to merging broken or untested logic.

When writing complex or low-level logic—especially code managing manual memory allocations, concurrency primitives, or unsafe operations—review the [Security Policy](./SECURITY.md) and adhere strictly to its requirements.
