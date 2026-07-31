# Contribution Guide

> English version: [CONTRIBUTING_EN.md](./CONTRIBUTING_EN.md)

Thank you for considering contributing to cursor-byok!

## Development Environment

| Dependency | Version Requirement |
|------------|---------------------|
| Go | >= 1.25 |
| Node.js | >= 20 |
| Yarn | 1.x (classic) |
| [Task](https://taskfile.dev) | >= 3 |
| [Wails v3 CLI](https://v3alpha.wails.dev) | alpha.74+ |

Linux extra dependencies: `libgtk-3-dev`, `libwebkit2gtk-4.1-dev` (required by Wails runtime).

## Quick Start

```bash
# Install frontend dependencies
cd frontend && yarn install --frozen-lockfile && cd ..

# Start dev mode (hot reload)
task dev

# Build current platform distribution package
task build
```

## Project Structure

```
├── main.go                 # Entrypoint
├── internal/               # Go backend (proxy, forwarding, client management, etc.)
├── frontend/               # Vue 3 + Vite + Tailwind frontend
│   ├── src/
│   │   ├── views/          # Views/Pages
│   │   ├── components/     # Components
│   │   ├── i18n/           # Internationalization (zh-CN / en-US / ja-JP / ru-RU)
│   │   └── state/          # Global state
│   └── plugins/            # Vite plugins (i18n static scanner, etc.)
├── prompt/                 # Built-in Agent prompt templates
├── proto/                  # Protobuf definitions
├── build/                  # Build configs & platform Taskfiles
├── scripts/                # Utility scripts (release, metrics)
└── Taskfile.yml            # Top-level task orchestration
```

## Development Guidelines

### Commit Messages

Use the [Conventional Commits](https://www.conventionalcommits.org/en/) style:

```
feat(proxy): support custom upstream timeout
fix(i18n): complete missing Japanese translation keys
release: 0.0.42
```

### Code Style

- Go: Follow `gofmt` / `go vet`, no extra linter configuration introduced.
- Frontend: Vue SFC + Composition API, prioritize Tailwind utility classes.
- New UI text must update all locale files simultaneously (`frontend/src/i18n/locales/`).

### Branching & PRs

1. Create feature branches from `main`: `feat/xxx`, `fix/xxx`.
2. Keep PRs small and focused; one PR addresses one issue.
3. Explain motivation and testing methods in PR descriptions.

## Build and Release

```bash
# Build for all platforms (macOS host only)
task build:all

# Prepare release assets
task release:prepare

# Publish to GitHub Releases
task release:github
```

## License

By submitting code, you agree to license your contribution under the [MIT License](./LICENSE).