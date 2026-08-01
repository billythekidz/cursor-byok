# Execution Plan: Translate Chinese and Vietnamese Text to English

Date: 2026-08-01

## Status

Active

## Outcome

All Chinese (CJK) and Vietnamese text across backend Go files, frontend Vue/JS components, build scripts (`Taskfile.yml`), and agent documentation (`.agents/skills/`) are safely translated into clear, professional English without breaking application logic, API contracts, formatting specifiers, or build processes.

## Context

- `docs/WORKFLOW.md`: Durable execution plan workflow.
- `AGENTS.md`: Safety rules (no breaking changes, read-only inspection, executable proof required).
- Existing translation artifacts: `zh-comments.jsonl`, `translation-*.json`, `build_translations.cjs`.

## Scope

In scope:
- **Backend Go Code (`internal/`, `cursor-tab-server/`, `proto/`, `prompt/`)**: Translate log messages, error strings (`fmt.Errorf`, `errors.New`), and user-facing text.
- **Frontend UI & State (`frontend/src/`)**: Translate hardcoded Chinese labels, state strings, and error messages in Vue components and JS state files.
- **Build Configurations (`Taskfile.yml`)**: Translate task summaries, error output messages, and application metadata strings.
- **Agent Skill Documentation (`.agents/skills/`)**: Translate markdown skill instructions into clear English.
- **Temporary Tools (`tmp_tools/`)**: Clean up or translate temporary tool comments.

Out of scope:
- Translation dictionary files that store raw locale mappings (e.g. `zh-CN.json`, `ja-JP.json`, `zh-comments.jsonl`).
- Binary assets and font files.

## Approach

1. **Phase 1: Backend Go Code Translation (`internal/`, `cursor-tab-server/`, `proto/`, `prompt/`)**
   - Translate error messages, log statements, and user-visible string literals into clear English.
   - Maintain all `%s`, `%v`, `%d`, `%w` format specifiers and error wrapping logic.
   - Validate with `go build ./...` to ensure zero compilation errors.

2. **Phase 2: Frontend Component & State Translation (`frontend/src/`)**
   - Translate UI labels, fallback messages, and state strings in Vue files (`ModelEditor.vue`, `ModelConfig.vue`, `HomeMetricsCard.vue`, `ModelAdapterModal.vue`) and `appState.js`.
   - Preserve Vue template interpolation, icon classes, and prop names.
   - Validate with frontend build script.

3. **Phase 3: API Contracts, State Keys, Enums & Identifiers Migration (High-Care Phase with Rollback Plan)**
   - Audit all internal object keys, state strings, fallback identifiers, and wire contract fields across Go and Frontend.
   - **Rollback Preparation**:
     - Create a clean git commit checkpoint before starting key migration.
     - Save a key mapping matrix (`scratch/key_migration_map.json`) mapping old CJK keys/values to new English keys/values.
   - Migrate any non-English keys/values in state stores, default configs, or API contracts.
   - Verify full end-to-end compatibility between Go backend responses and Vue frontend state listeners.

4. **Phase 4: Build Automation (`Taskfile.yml`)**
   - Update Taskfile descriptions, summaries, error outputs, and `APP_NAME`.

5. **Phase 5: Agent Skill Documentation (`.agents/skills/`)**
   - Translate Chinese markdown content in skill files to English while preserving markdown formatting and technical terms.

6. **Phase 6: Cleanup & Final Automated Verification**
   - Run automated scan script (`scratch/scan.js`) to confirm all targeted source files are clean of CJK and Vietnamese text.

## Risks And Recovery

- **Risk**: Accidentally breaking string format specifiers (e.g. `%w`, `%s`) or JSON key names / state contracts during key migration.
  - **Mitigation**: Strictly inspect diffs to ensure wire compatibility. Use `scratch/key_migration_map.json` to verify 1-to-1 key mappings.
- **Rollback Procedure**:
  - Before Phase 3, record `git rev-parse HEAD` into `scratch/pre_migration_commit.txt`.
  - If any API contract or state key migration causes regression in frontend-backend communication or storage loading, run `git checkout <pre_migration_commit> -- <affected_files>` or `git reset --hard <pre_migration_commit>` to restore state immediately.

## Progress

- [x] Audit repository for CJK and Vietnamese occurrences.
- [ ] Phase 1: Translate Backend Go code (`internal/`, `cursor-tab-server/`, `proto/`, `prompt/`).
- [ ] Phase 2: Translate Frontend Vue & JS UI text (`frontend/src/`).
- [ ] Phase 3: Migrate API Contracts, State Keys, Enums & Identifiers (with git checkpoint & mapping matrix).
- [ ] Phase 4: Translate `Taskfile.yml`.
- [ ] Phase 5: Translate Agent Skill files (`.agents/skills/`).
- [ ] Phase 6: Cleanup `tmp_tools/` & verify with automated scanner.

## Decisions

- 2026-08-01: Keep `zh-CN.json` and `ja-JP.json` intact as legitimate i18n locale dictionary assets, but ensure all default source code fallback text is strictly English.

## Validation

- Focused proof: `go build ./...` for Go modules; frontend build for UI.
- Scanner check: Re-run `scratch/scan.js` to ensure 0 CJK/VI string matches in source files.
