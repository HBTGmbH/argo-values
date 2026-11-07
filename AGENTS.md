# AGENTS.md

Short guidance for coding agents. This file is not user documentation.

## Documentation

- `README.md` and `docs/` contain the user-facing documentation (English, written for humans).
- Follow the existing documentation. If behavior, flags, configuration, or workflows change, update the affected docs in the same change.
- Add new pages to the map in `docs/README.md` and link them from `README.md` where relevant.
- Keep this file minimal; do not put user documentation here.

## Tests

- Add or update tests for every behavior change, including edge cases and error paths.
- Run `make test` (or `go test ./...`) before committing. Use `make check` for formatting, vet, and tests.

## Commit messages

Use Conventional Commits:

```text
<type>: <short summary>
```

Common types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `ci`.
Examples: `docs: add local testing guide`, `fix: reject missing ConfigMap`.
Keep the summary imperative, lower-case, and without a trailing period.
