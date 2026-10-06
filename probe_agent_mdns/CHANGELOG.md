# Changelog

## 0.15.0

- Initial release. mDNS/Bonjour advertising moved out of `flutter_probe_agent` (FP-15) so the core agent has
  no native plugin dependency and nothing native is linked into release builds of apps that only use the agent.
