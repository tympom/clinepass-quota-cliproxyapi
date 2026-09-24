# AGENTS.md

Include parent `AGENTS.md`.

## Releases

- Every release gets a new version; never re-cut or move a published tag (Plugin Store clarity).
- CI does not run tests; run `go test ./...` and `go vet ./...` on a clean checkout of the exact commit before tagging.
