import 'dart:async';
import 'package:flutter/scheduler.dart';
import 'package:flutter/widgets.dart';

/// ProbeSync implements the triple-signal synchronization model:
///   1. No pending frames   (SchedulerBinding)
///   2. No active animations (AnimationController tracking)
///   3. No in-flight HTTP requests (manual counter)
///
/// After every action, waitForSettled() is called before responding to the CLI.
class ProbeSync {
  ProbeSync._();
  static final ProbeSync instance = ProbeSync._();

  // ---- HTTP request tracking ----
  int _httpPending = 0;

  void httpRequestStarted() => _httpPending++;
  void httpRequestFinished() {
    if (_httpPending > 0) _httpPending--;
  }

  bool get hasInflightRequests => _httpPending > 0;

  // ---- Animation tracking ----
  final Set<AnimationController> _animations = {};

  void trackAnimation(AnimationController c) => _animations.add(c);
  void untrackAnimation(AnimationController c) => _animations.remove(c);

  bool get hasActiveAnimations =>
      _animations.any((a) => a.isAnimating);

  // ---- Settled check ----

  /// Wait until all three signals are idle, or [timeout] elapses.
  Future<void> waitForSettled({Duration timeout = const Duration(seconds: 10)}) async {
    final deadline = DateTime.now().add(timeout);

    while (DateTime.now().isBefore(deadline)) {
      await _flushScheduledFrame();
      if (_isSettled()) return;
      // Pump a frame tick then re-check
      await _pumpFrame();
    }

    // Last chance
    if (_isSettled()) return;
    throw TimeoutException(
      'ProbeSync: UI did not settle within ${timeout.inSeconds}s '
      '(frames=${_pendingFrames()}, animations=${_animations.where((a) => a.isAnimating).length}, http=$_httpPending)',
      timeout,
    );
  }

  /// A setState / markNeedsBuild (a tap's effect) schedules a frame, but
  /// schedulerPhase keeps reading idle until that frame starts, so the
  /// settled check below would pass before the UI reflects the action
  /// ("UI one tap behind"). Let a requested frame run first. Bounded: a
  /// screen that animates forever always has a frame scheduled and must
  /// not stall every action.
  Future<void> _flushScheduledFrame() async {
    final binding = SchedulerBinding.instance;
    if (!binding.framesEnabled || !binding.hasScheduledFrame) return;
    // Test bindings only run frames when the test pumps them; waiting here
    // would hang a widget test, so keep their (immediate) behaviour.
    if (binding is! WidgetsFlutterBinding) return;
    await binding.endOfFrame.timeout(
      const Duration(milliseconds: 250),
      onTimeout: () {},
    );
  }

  bool _isSettled() =>
      _pendingFrames() == 0 && !hasActiveAnimations && !hasInflightRequests;

  int _pendingFrames() {
    final binding = SchedulerBinding.instance;
    // framesEnabled && schedulerPhase != idle means there is work pending
    if (!binding.framesEnabled) return 0;
    return binding.schedulerPhase == SchedulerPhase.idle ? 0 : 1;
  }

  Future<void> _pumpFrame() async {
    final completer = Completer<void>();
    SchedulerBinding.instance.addPostFrameCallback((_) {
      completer.complete();
    });
    // If no frame is scheduled, add a short delay
    SchedulerBinding.instance.scheduleFrame();
    await completer.future.timeout(
      const Duration(milliseconds: 100),
      onTimeout: () {},
    );
  }
}
