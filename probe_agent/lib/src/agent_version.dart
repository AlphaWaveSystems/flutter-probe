/// Agent version reported in mDNS TXT records, `GET /probe/status`, and the
/// `probe.ping` handshake result (`agent_version`) so the CLI can detect a
/// version mismatch on connect. Dart cannot read its own pubspec.yaml at
/// runtime, so this constant must be bumped by hand in the same commit that
/// bumps the `version:` field in pubspec.yaml — it had drifted to 0.7.0 while
/// pubspec.yaml moved on to 0.9.9 before this file was split out; keep them
/// in sync going forward.
///
/// Lives in its own file (rather than server.dart, where it previously
/// lived) so executor.dart can read it too without an executor.dart <->
/// server.dart import cycle.
const String probeAgentVersion = '0.10.2';
