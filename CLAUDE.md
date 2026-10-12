# notch-lab

- Vocabulary: `CONTEXT.md`. Use its words in code and prose. Do not use the words marked _Avoid_.
- Specification: `docs/spec/`. A change in behaviour updates its requirement in the same commit. Decisions: `docs/adr/`. Order of work: `docs/milestones.md`.
- Documents and code comments are ASD-STE100 (`docs/spec/00-writing.md`). Commits are Conventional Commits.
- Run the app only through `tools/dev` and `tools/verify/nl`. Every run uses an isolated `NOTCH_LAB_HOME` (`/tmp/notch-lab-dev`). Never touch `~/Library/Application Support/notch-lab`.
- `go test ./...` before a commit. `CGO_ENABLED=0` always.
- mygo is the fork in `go.mod` (`replace` to `github.com/sviniabanditka/mygo`, branch `notch-lab`). Upgrade it on purpose by editing the pinned version; never `go get -u`. The repository is private: `GOPRIVATE=github.com/sviniabanditka`.
- The fork's patches: `docs/spec/14-mygo-fork.md` and the fork's `docs/fork/patches.md`.
