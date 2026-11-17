import 'dart:async';
import 'dart:ui' show FlutterView;

import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';

/// ViewReady makes the agent answer only once the app's view has a real size (issue #793).
///
/// After a relaunch, an app can draw its first frames with a 0×0 view and get its real size a second or so later (measured on an
/// Android 15 emulator: about 1 to 1.2 s). A `ListView` laid out at zero height builds only its first child. The view's size changes the
/// moment the engine reports it, but the widgets are laid out at it only in the next frame: a request that comes in between sees the
/// tree of the 0×0 layout, half a screen (measured: no request arrived while the size was 0×0, yet steps saw the half-built tree).
/// So the agent records the size each frame was laid out at ([attach]), and the view is ready only when that size is the view's real size.
/// [wait] waits for such a frame.
///
/// It never makes an app slow that has nothing to wait for:
/// - a view with a real size: [isReady] is a synchronous check, and [wait] returns at once without an `await`;
/// - no view at all (`platformDispatcher.views` empty), or an app that is not in the foreground (paused, hidden, inactive,
///   detached): no wait;
/// - a size that does not come within [cap]: the agent stops waiting for it until the view's metrics change again, so at most one
///   capped wait happens per "size unknown" episode.
///
/// [cap] stays well under the CLI's limits: the HTTP client's handshake runs inside the dial timeout, which is 5 s in Studio and 30 s
/// in the CLI by default.
class ViewReady with WidgetsBindingObserver {
  ViewReady._();

  /// The one instance the executor uses.
  static final ViewReady instance = ViewReady._();

  /// The longest [wait] waits.
  static const cap = Duration(milliseconds: 2500);

  /// How often [wait] looks at the size while it is zero (the engine may schedule no frame until the size arrives).
  static const _tick = Duration(milliseconds: 16);

  bool _observing = false;

  /// The size the last frame was laid out at, recorded by the persistent frame callback; null before [attach] or before the first frame.
  Size? _laidOut;

  /// Whether [attach] ran. Without it the agent cannot tell a laid-out size from a reported one, and the view is ready once it has a size
  /// (the rule before this one).
  bool _attached = false;

  /// Records the size each frame is laid out at. It never makes the binding and never throws: it takes effect only when the app's binding
  /// exists, and otherwise does nothing, so the executor calls it again on each request until it has ([attached]). Until then the earlier
  /// rule applies.
  void attach() {
    if (_attached) return;
    attachAttempts++;
    try {
      final binding = WidgetsBinding.instance;
      binding.addPersistentFrameCallback((_) {
        final views = ViewReady.views();
        _laidOut = views.isEmpty ? null : views.first.physicalSize;
      });
      // Attached late (the app made its binding after start()): with no frame scheduled, the tree is laid out at the current size (a new
      // size always schedules a frame), so that size is taken as laid out. With a frame pending, the agent waits for it.
      final views = ViewReady.views();
      if (!binding.hasScheduledFrame && views.isNotEmpty)
        _laidOut = views.first.physicalSize;
      _attached = true;
    } catch (_) {
      // No binding (or it refused the callback): keep the earlier rule rather than fail.
    }
  }

  /// [attach], when the app's binding exists and is an app's binding (a [WidgetsFlutterBinding] or a subclass of it). A test binding runs
  /// frames only when the test pumps them, so a request must not wait for a frame there; under one the agent keeps the earlier rule, as
  /// ProbeSync does. The executor calls this on each request until the agent is attached.
  void attachToAppBinding() {
    if (_attached) return;
    try {
      if (WidgetsBinding.instance is! WidgetsFlutterBinding) return;
    } catch (_) {
      return; // no binding yet
    }
    attach();
  }

  /// Whether [attach] took effect (for tests and diagnostics).
  bool get attached => _attached;

  /// How many times [attach] tried to register its frame callback (for tests: once it took effect, it never tries again).
  @visibleForTesting
  int attachAttempts = 0;

  /// Set when [cap] was reached; cleared when the view's metrics change (a new size may be on its way).
  bool _gaveUp = false;

  /// Whether a capped wait happened and the size is still unknown (for tests and diagnostics).
  bool get gaveUp => _gaveUp;

  /// True when there is nothing to wait for. Cheap: no await, no allocation beyond reading the binding.
  /// The views the app has; a test replaces it.
  @visibleForTesting
  static Iterable<FlutterView> Function() views =
      () => WidgetsBinding.instance.platformDispatcher.views;

  bool get isReady {
    final binding = WidgetsBinding.instance;
    final views = ViewReady.views();
    if (views.isEmpty) return true;
    final state = binding.lifecycleState;
    if (state != null && state != AppLifecycleState.resumed) return true;
    if (_gaveUp) return true;
    final size = views.first.physicalSize;
    if (size.isEmpty) return false;
    return !_attached || _laidOut == size;
  }

  /// Waits, at most [cap], until the first view has a real size and a frame has been laid out at it. Returns the milliseconds it
  /// waited (0 when there was nothing to wait for). Never throws.
  Future<int> wait() {
    if (isReady) return SynchronousFuture<int>(0);
    return _waitForSize();
  }

  Future<int> _waitForSize() async {
    _observe();
    final watch = Stopwatch()
      ..start(); // reports the wait; the cap itself is a timer
    final capped = Completer<void>();
    final capTimer = Timer(cap, capped.complete);
    try {
      while (!capped.isCompleted) {
        await Future.any([Future<void>.delayed(_tick), capped.future]);
        if (capped.isCompleted) break;
        final binding = WidgetsBinding.instance;
        final views = ViewReady.views();
        final state = binding.lifecycleState;
        if (views.isEmpty ||
            (state != null && state != AppLifecycleState.resumed)) {
          return watch
              .elapsedMilliseconds; // went to the background, or has no view now: nothing to wait for
        }
        final size = views.first.physicalSize;
        if (!size.isEmpty) {
          if (!_attached || _laidOut == size) {
            return watch
                .elapsedMilliseconds; // a frame was laid out at the real size
          }
          // The size is real: wait for a frame laid out at it, within the cap.
          await Future.any([binding.endOfFrame, capped.future]);
          if (_laidOut == size) return watch.elapsedMilliseconds;
          // A size with no frame laid out at it within the cap: the same episode, not waited for again.
          if (capped.isCompleted) break;
        }
      }
      _giveUp();
      return watch.elapsedMilliseconds;
    } finally {
      capTimer.cancel();
    }
  }

  void _observe() {
    if (_observing) return;
    _observing = true;
    WidgetsBinding.instance.addObserver(this);
  }

  @override
  void didChangeMetrics() {
    // Only a new size starts a new episode: other metrics (keyboard insets, padding) change without it, and a view that stays 0×0 by
    // design must not be waited for again on every such change.
    if (!_gaveUp) return;
    final views = ViewReady.views();
    final size = views.isEmpty ? null : views.first.physicalSize;
    if (size != _sizeAtGiveUp) _gaveUp = false;
  }

  /// The size of the view when the cap was reached.
  Size? _sizeAtGiveUp;

  void _giveUp() {
    _gaveUp = true;
    final views = ViewReady.views();
    _sizeAtGiveUp = views.isEmpty ? null : views.first.physicalSize;
  }

  /// Forgets a capped wait, the laid-out size and the attach, and stops observing. For tests.
  @visibleForTesting
  void reset() {
    _gaveUp = false;
    _laidOut = null;
    attachAttempts = 0;
    // A later attach adds its callback again (persistent callbacks cannot be removed; the extra one is harmless).
    _attached = false;
    if (_observing) {
      WidgetsBinding.instance.removeObserver(this);
      _observing = false;
    }
  }
}
