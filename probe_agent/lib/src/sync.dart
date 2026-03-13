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
