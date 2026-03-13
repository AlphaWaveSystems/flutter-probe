import 'dart:async';
import 'protocol.dart';

/// ProbePlugin is the base class for custom command plugins.
///
/// Register a plugin with [ProbePluginRegistry.register] before the agent starts.
///
/// Example:
/// ```dart
/// class AuthBypassPlugin extends ProbePlugin {
///   @override
///   String get command => 'probe.plugin.auth_bypass';
///
///   @override
///   Future<dynamic> execute(Map<String, dynamic> params) async {
///     final token = params['token'] as String;
///     await GetIt.I<AuthService>().loginWithToken(token);
///     return {'ok': true};
///   }
/// }
///
/// // Register before ProbeAgent.start():
/// ProbePluginRegistry.register(AuthBypassPlugin());
/// ```
abstract class ProbePlugin {
  /// The JSON-RPC method name this plugin handles.
  /// Convention: probe.plugin.<name>
  String get command;

  /// Human-readable description shown in probe lint --list-plugins.
  String get description => '';

  /// Execute the plugin command with the given params.
  Future<dynamic> execute(Map<String, dynamic> params);
}

/// ProbePluginRegistry holds all registered plugins.
class ProbePluginRegistry {
  ProbePluginRegistry._();

  static final Map<String, ProbePlugin> _plugins = {};

  /// Register a plugin. Must be called before ProbeAgent.start().
  static void register(ProbePlugin plugin) {
    _plugins[plugin.command] = plugin;
  }

  /// Returns the plugin for [method], or null if not found.
  static ProbePlugin? find(String method) => _plugins[method];

  /// Returns true if any plugin handles [method].
  static bool handles(String method) => _plugins.containsKey(method);

  /// Dispatches a request to the registered plugin, returns a ProbeResponse.
  static Future<ProbeResponse> dispatch(ProbeRequest req) async {
    final plugin = _plugins[req.method];
    if (plugin == null) {
      return ProbeResponse.err(
        req.id,
        ProbeError(ProbeError.methodNotFound, 'No plugin for ${req.method}'),
      );
    }
    try {
      final result = await plugin.execute(req.params);
      return ProbeResponse.ok(req.id, result);
    } catch (e) {
      return ProbeResponse.err(
        req.id,
        ProbeError(ProbeError.internalError, e.toString()),
      );
    }
  }

  /// Lists all registered plugin method names.
  static List<String> get registeredMethods => _plugins.keys.toList();
}

/// Example plugin: token-based auth bypass (for dev/test only).
///
/// Usage in .probe file:
/// ```
/// bypass login as "admin"
/// see "Admin Dashboard"
/// ```
///
/// Plugin YAML (plugins/auth_bypass.yaml):
/// ```yaml
/// command: "bypass login as"
/// method: "probe.plugin.auth_bypass"
/// description: "Bypass login with a pre-set token"
/// params:
///   token: "${1}"
/// ```
class AuthBypassPlugin extends ProbePlugin {
  final Future<void> Function(String token) _loginFn;

  AuthBypassPlugin(this._loginFn);

  @override
  String get command => 'probe.plugin.auth_bypass';

  @override
  String get description => 'Bypass authentication with a pre-issued token';

  @override
  Future<dynamic> execute(Map<String, dynamic> params) async {
    final token = params['token'] as String? ?? '';
    await _loginFn(token);
    return {'ok': true};
  }
}
