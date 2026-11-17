# Adago design MCP connection

Registers the local `adago-design` Model Context Protocol server with the agent. The bundle contains no code: its patch inserts `@deepseek-ai/dsh-mcp-client` with the endpoint below.

## Configuration

| Field | Value |
|---|---|
| Server name (tool prefix) | `adago_design` |
| Transport | `streamable-http` |
| Endpoint | `http://127.0.0.1:8788/mcp` |
| Startup budget | 30 s per connection attempt |
| Startup failure | harness keeps running; reconnect retries automatically |

## Tools

The server's tools appear as `mcp__adago_design__<tool>`. They cover design systems and tokens (`design_systems`, `design_tokens`, `design_components`, `design_search`), canvas and screenshots (`project_create`, `project_list`, `design_artboard`, `design_preview`, `design_screen`, `canvas_add`, `canvas_arrange`, `canvas_screenshot`, `canvas_export`), icons and logos (`design_icons`, `design_icon_manage`, `design_logo`), and audits (`design_audit`, `design_derive`, `design_compare`). Ask the agent to compare design systems, build a component preview or a full screen, draw an icon, or export the canvas to use them.

## Requirements and limits

- The server must be running on port 8788. When it is offline at DSH startup the harness still starts, and the connection keeps retrying with backoff.
- Design work must belong to a project: create one with `project_create` (or pick an existing one with `project_list`) before asking for previews or screens.
- Screenshots require the canvas page to be open in the built-in window or a browser.
- Turning the bundle off unregisters every `adago_design` tool until it is turned on again.
