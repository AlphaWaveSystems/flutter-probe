import 'dart:async';
import 'dart:io';
import 'dart:developer' show Timeline;
import 'dart:ui' show FramePhase, FrameTiming;

import 'package:flutter/scheduler.dart';

/// Collects frame timings and process memory while a `start measuring` window is open.
///
/// Frame timings come from Flutter itself (SchedulerBinding timings callback), so
/// they are the same on every platform and on physical devices. Memory is the
/// process's resident set size, sampled from inside the app. CPU is not available
/// to Dart: the CLI samples it from the device.
class ProbePerf {
  ProbePerf._();
  static final ProbePerf instance = ProbePerf._();

  /// A frame whose build+raster span exceeds this is "slow" (one 60 Hz interval).
  static const double slowFrameMs = 16.7;

  bool _measuring = false;
  DateTime? _startedAt;
  int _startMicros = 0;
  Timer? _memTimer;
  TimingsCallback? _cb;

  final List<double> _total = [];
  double _buildSum = 0;
  double _rasterSum = 0;
  int _rssStart = 0;
  int _rssMax = 0;
  int _rssLast = 0;

  bool get measuring => _measuring;

  int _rss() {
    try {
      return ProcessInfo.currentRss;
    } catch (_) {
      return 0;
    }
  }

  void start() {
    if (_measuring) stopWindow();
    _total.clear();
    _buildSum = 0;
    _rasterSum = 0;
    _startedAt = DateTime.now();
    _startMicros = Timeline.now;
    _rssStart = _rss();
    _rssMax = _rssStart;
    _rssLast = _rssStart;
    _cb = (List<FrameTiming> timings) {
      for (final t in timings) {
        // Timings arrive in batches, some of frames drawn before this window opened.
        if (t.timestampInMicroseconds(FramePhase.rasterFinish) < _startMicros) continue;
        _total.add(t.totalSpan.inMicroseconds / 1000.0);
        _buildSum += t.buildDuration.inMicroseconds / 1000.0;
        _rasterSum += t.rasterDuration.inMicroseconds / 1000.0;
      }
    };
    SchedulerBinding.instance.addTimingsCallback(_cb!);
    _memTimer = Timer.periodic(const Duration(milliseconds: 250), (_) => _sampleMemory());
    _measuring = true;
  }

  void _sampleMemory() {
    final r = _rss();
    _rssLast = r;
    if (r > _rssMax) _rssMax = r;
  }

  void stopWindow() {
    if (_cb != null) SchedulerBinding.instance.removeTimingsCallback(_cb!);
    _cb = null;
    _memTimer?.cancel();
    _memTimer = null;
    _measuring = false;
  }

  /// The numbers so far; keeps measuring.
  Map<String, dynamic> snapshot() {
    if (_measuring) _sampleMemory();
    final durationMs = _startedAt == null ? 0 : DateTime.now().difference(_startedAt!).inMilliseconds;
    final sorted = [..._total]..sort();
    double pct(double p) {
      if (sorted.isEmpty) return 0;
      final i = ((sorted.length - 1) * p).round();
      return sorted[i];
    }

    final slow = _total.where((t) => t > slowFrameMs).length;
    return {
      'durationMs': durationMs,
      'frames': _total.length,
      'slowFrames': slow,
      'slowFramePct': _total.isEmpty ? 0.0 : slow * 100.0 / _total.length,
      'frameP50Ms': pct(0.5),
      'frameP95Ms': pct(0.95),
      'frameP99Ms': pct(0.99),
      'frameMaxMs': sorted.isEmpty ? 0.0 : sorted.last,
      'buildAvgMs': _total.isEmpty ? 0.0 : _buildSum / _total.length,
      'rasterAvgMs': _total.isEmpty ? 0.0 : _rasterSum / _total.length,
      'rssStartBytes': _rssStart,
      'rssPeakBytes': _rssMax,
      'rssEndBytes': _rssLast,
      'measuring': _measuring,
    };
  }
}
