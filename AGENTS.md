# StarHub Agent Memory

## Code Style Rules

- **Always write code comments in English.** Do not use Chinese or other non-English languages in comments, commit messages, or identifiers. (Exception: non-ASCII strings that are intentional test data, e.g. CJK text used to verify token estimation in tests, may be kept.)
- Commit messages must also be written in English.

## Project Conventions

- Build/test verification commands: `go build ./...`, `go vet ./...`, `go test ./...`
- Go module name: `starhub`; Go 1.26+; pure-Go SQLite (no CGO)
