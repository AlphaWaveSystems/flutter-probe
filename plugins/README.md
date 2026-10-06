# plugins

Example ProbeScript plugins. A plugin maps a custom command to an agent RPC method so tests can use it like a
built-in step.

`auth_bypass.yaml` defines `bypass login as "<token>"`, which authenticates with a dev token through the
`probe.plugin.auth_bypass` method instead of driving the login UI:

```yaml
command: "bypass login as"
method: "probe.plugin.auth_bypass"
description: "Authenticate directly using a dev token — no UI interaction"
params:
  token: "${1}"
```

Plugin docs: [flutterprobe.dev/tools/plugins](https://flutterprobe.dev/tools/plugins/).
