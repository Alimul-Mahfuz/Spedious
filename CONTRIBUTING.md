# Contributing to Spedious

Thank you for your interest in contributing to Spedious!

## Principles

1. **Generic Engine**: The core engine must never contain business domain concepts (such as "order", "checkout", or "cart"). It only knows requests, variables, extractors, assertions, timing, and flow control.
2. **Minimal Dependencies**: Prefer the Go standard library. Only introduce third-party dependencies when there is a strong justification.
3. **Accuracy**: Latency measurements must be scientifically sound, avoiding coordinated omission.
4. **Testing & Quality**: Every package must have thorough unit/integration tests with `-race` clean execution.

## Development Workflow

1. Fork and clone the repository.
2. Ensure Go 1.22+ is installed.
3. Run tests and linting:
   ```bash
   make test
   make lint
   ```
4. Build the binary:
   ```bash
   make build
   ```

## Commit Conventions

We follow Conventional Commits:
- `feat:` A new feature
- `fix:` A bug fix
- `docs:` Documentation only changes
- `test:` Adding or updating tests
- `chore:` Changes to build process or tooling
