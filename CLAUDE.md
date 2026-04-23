## Seshat MCP

Use the `seshat` MCP server for codebase analysis. Call `list_projects` to discover available `project_id` values. Every code query must pass `project_id`.
The shared MCP registry lives at `$HOME/.seshat/config.yml` and should be served with `seshat mcp --registry $HOME/.seshat/config.yml`.
Run `seshat scan` before non-trivial codebase analysis if the index may be stale, after pulling new code, and after making code changes. Use `file_dependency_graph` before editing a file to understand impact.
