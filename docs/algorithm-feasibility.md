# Algorithm Feasibility

## Go

### Feasible in MVP

- File discovery from include/exclude paths
- AST parsing for functions, methods, types, packages, and imports
- `declared_in`, `contains`, `imports`
- Best-effort `calls` for direct identifiers and simple selector expressions
- Best-effort `references` for composite literal type usage

### Deferred

- Full type resolution across packages
- Interface implementation inference beyond simple symbol capture
- Framework-aware route tracing
- Semantic ranking and context building

## Ruby

### Feasible in MVP

- File-level discovery and checksum
- Best-effort extraction of modules, classes, and methods
- Best-effort `declared_in` and `contains`
- Heuristic `calls` and `references`

### Known Limits

- Dynamic dispatch is not resolved
- Metaprogramming and `method_missing` are not modeled
- DSL-heavy code may lose relationship fidelity

## Incremental Scan

- Feasible with `git diff`, file checksums, and targeted reparse
- Not fully implemented in the bootstrap
- The CLI already carries the `scan_mode` contract needed for later rollout
