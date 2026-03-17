---
title: Plugins
description: Extend ProbeScript with custom commands using YAML-based plugins.
---

FlutterProbe supports a plugin system that lets you define custom ProbeScript commands backed by Dart handlers in the ProbeAgent.

## Plugin Definition

Plugins are YAML files in the `plugins/` directory:

```yaml
# plugins/auth_bypass.yaml
command: "bypass login as"
method: "probe.plugin.auth_bypass"
description: "Authenticate directly using a dev token"
params:
  token: "${1}"
```

## Plugin Fields

| Field | Required | Description |
|-------|----------|-------------|
| `command` | Yes | The ProbeScript command name |
| `method` | Yes | JSON-RPC method name dispatched to the Dart agent |
| `description` | No | Human-readable description |
| `params` | No | Parameter mapping (`${1}`, `${2}`, etc. for positional args) |

## Using Plugins in Tests

Once a plugin is defined, use it like any other ProbeScript command:

```
test "admin can access dashboard"
  bypass login as "admin-dev-token"
  see "Admin Dashboard"
```

## Dart Handler

The plugin dispatches a JSON-RPC call to the ProbeAgent. You need a corresponding handler in your app's agent setup to process the method:

```dart
// In your app's ProbeAgent configuration
ProbeAgent.registerPlugin('probe.plugin.auth_bypass', (params) async {
  final token = params['token'] as String;
  // Perform direct authentication using the dev token
  await AuthService.instance.loginWithDevToken(token);
});
```

## Plugin Directory

By default, plugins are loaded from the `plugins/` directory relative to `probe.yaml`. You can have multiple plugin files:

```
plugins/
  auth_bypass.yaml
  seed_data.yaml
  reset_state.yaml
```

## Use Cases

- **Auth bypass** — Skip the login flow in tests by authenticating directly with a dev token
- **Data seeding** — Populate the app with test data before running tests
- **State reset** — Reset specific app state without clearing all data
- **Feature flags** — Toggle feature flags during tests
- **Custom assertions** — Check app-specific conditions not covered by standard ProbeScript
