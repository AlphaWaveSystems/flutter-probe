import 'dart:async';
import 'package:flutter/gestures.dart';
import 'package:flutter/services.dart';
import 'package:flutter/widgets.dart';

/// ProbeGestures implements advanced gestures not covered by the basic executor.
class ProbeGestures {
  ProbeGestures._();
  static final ProbeGestures instance = ProbeGestures._();

  // ---- Pinch-to-zoom ----

  /// Performs a two-finger pinch gesture at [center].
  /// [scale] > 1 = zoom in, < 1 = zoom out.
  Future<void> pinch(Offset center, double scale) async {
    const int steps = 20;
    final double startRadius = 80.0;
    final double endRadius = startRadius * scale;

    final binding = GestureBinding.instance;

    // Pointer 1: top-left, Pointer 2: bottom-right
    int p1 = 1, p2 = 2;

    // Start both fingers
    binding.handlePointerEvent(PointerDownEvent(
      pointer: p1,
      position: center + Offset(-startRadius, 0),
    ));
    binding.handlePointerEvent(PointerDownEvent(
      pointer: p2,
      position: center + Offset(startRadius, 0),
    ));

    // Move fingers apart/together in steps
    for (int i = 1; i <= steps; i++) {
      final t = i / steps;
      final radius = startRadius + (endRadius - startRadius) * t;
      binding.handlePointerEvent(PointerMoveEvent(
        pointer: p1,
        position: center + Offset(-radius, 0),
      ));
      binding.handlePointerEvent(PointerMoveEvent(
        pointer: p2,
        position: center + Offset(radius, 0),
      ));
      await Future.delayed(const Duration(milliseconds: 16));
    }

    // Lift both fingers
    binding.handlePointerEvent(PointerUpEvent(
      pointer: p1,
      position: center + Offset(-endRadius, 0),
    ));
    binding.handlePointerEvent(PointerUpEvent(
      pointer: p2,
      position: center + Offset(endRadius, 0),
    ));
  }

  // ---- Two-finger rotation ----

  Future<void> rotate(Offset center, double angleDegrees) async {
    const int steps = 20;
    final double radians = angleDegrees * 3.14159 / 180.0;
    final double radius = 80.0;
    final binding = GestureBinding.instance;
    int p1 = 3, p2 = 4;

    binding.handlePointerEvent(PointerDownEvent(
      pointer: p1, position: center + Offset(radius, 0),
    ));
    binding.handlePointerEvent(PointerDownEvent(
      pointer: p2, position: center + Offset(-radius, 0),
    ));

    for (int i = 1; i <= steps; i++) {
      final angle = radians * i / steps;
      binding.handlePointerEvent(PointerMoveEvent(
        pointer: p1,
        position: center + Offset(radius * _cos(angle), radius * _sin(angle)),
      ));
      binding.handlePointerEvent(PointerMoveEvent(
        pointer: p2,
        position: center + Offset(-radius * _cos(angle), -radius * _sin(angle)),
      ));
      await Future.delayed(const Duration(milliseconds: 16));
    }

    binding.handlePointerEvent(PointerUpEvent(pointer: p1, position: center));
    binding.handlePointerEvent(PointerUpEvent(pointer: p2, position: center));
  }

  // ---- Device actions via platform channels ----

  /// Sets the system locale (e.g. "fr_FR").
  Future<void> setLocale(String locale) async {
    // On Android: adb shell setprop persist.sys.locale fr-FR
    // Within the app we emit a notification for the test runner to handle via ADB.
    // The CLI side handles actual locale change; agent just acknowledges.
  }

  /// Toggles dark mode (app-level only — system toggle requires ADB).
  Future<void> toggleDarkMode() async {
    // Notify the app via a platform channel or dependency injection
    // This is app-specific; emitting as a platform message.
    await _platformChannel.invokeMethod<void>('probe.toggle_dark_mode');
  }

  /// Simulates a network connectivity change.
  Future<void> setNetworkState({required bool enabled}) async {
    // Android: requires adb shell svc wifi enable/disable (CLI-side)
    // Agent just acknowledges the intent.
  }

  /// Triggers a device shake via the accelerometer mock channel.
  Future<void> shake() async {
    await _platformChannel.invokeMethod<void>('probe.shake');
  }

  // ---- Keyboard ----

  /// Dismisses the on-screen keyboard.
  Future<void> closeKeyboard() async {
    await SystemChannels.textInput.invokeMethod<void>('TextInput.hide');
  }

  /// Opens the keyboard by requesting focus.
  Future<void> openKeyboard(Element element) async {
    (element as StatefulElement).state;
    // Focus the element
    FocusScope.of(element).requestFocus(FocusNode());
  }

  // ---- Helpers ----

  static const MethodChannel _platformChannel = MethodChannel('probe_agent/device');

  static double _cos(double r) {
    // Taylor series approximation for small angles
    double result = 1.0;
    double term = 1.0;
    for (int i = 1; i <= 6; i++) {
      term *= -r * r / (2 * i * (2 * i - 1));
      result += term;
    }
    return result;
  }

  static double _sin(double r) {
    double result = r;
    double term = r;
    for (int i = 1; i <= 6; i++) {
      term *= -r * r / ((2 * i + 1) * (2 * i));
      result += term;
    }
    return result;
  }
}
