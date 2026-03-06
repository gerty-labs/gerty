# Contributing to Gerty

Contributions are welcome. Bug fixes, documentation, Helm chart improvements, CLI features, and community support are all valued.

## What lives here vs. gerty-core

This repository contains the **agent**, **CLI**, and **Helm chart**. The server, rules engine, and SLM live in the private `gerty-core` repository. PRs that modify server logic, the rules engine, or SLM internals will be closed.

## Development Setup

```bash
# Requirements: Go 1.25+, Helm 3.x
git clone https://github.com/gerty-labs/gerty.git
cd gerty

make build   # Build agent + CLI binaries
make test    # Run all tests
make lint    # Go vet + staticcheck + helm lint
```

## Pull Request Process

1. Fork the repository and create a branch from `main`.
2. Write tests for new functionality.
3. Use [conventional commits](https://www.conventionalcommits.org/):
   - `feat:` new feature
   - `fix:` bug fix
   - `docs:` documentation
   - `test:` tests
   - `chore:` maintenance
4. Run `make test && make lint` before pushing.
5. Open a PR against `main`. Fill in the PR template.

## Code Style

- Follow existing patterns in the codebase.
- Keep the agent budget: 50MB RAM, 0.05 CPU. If your change increases resource usage, justify it.
- No external dependencies without discussion in an issue first.

## Community Support

Not a coder? You can still help:

- Answer questions in GitHub Issues and Discussions.
- Write tutorials or blog posts about running Gerty.
- Report bugs with detailed reproduction steps.
- Suggest documentation improvements.

## Security

Found a vulnerability? See [SECURITY.md](SECURITY.md). Do not open a public issue.

## License

By contributing, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).
