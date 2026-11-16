import 'package:flutter_probe_agent/src/build_info.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('probeBuildInfoFrom', () {
    test('reports every field the build knows', () {
      final m = probeBuildInfoFrom(
        flutterVersion: '3.50.0',
        flutterChannel: 'stable',
        frameworkRevision: 'ab29120',
        dartVersion: '3.14.0',
        platformVersion:
            '3.99.9 (stable) (Mon Jan 1 00:00:00 2099 +0000) on "macos_arm64"',
      );
      expect(m, {
        'flutter_version': '3.50.0',
        'flutter_channel': 'stable',
        'framework_revision': 'ab29120',
        'dart_version':
            '3.14.0', // the build's own value wins over the running VM's
      });
    });

    test(
        'an app built by a tool that sets nothing reports nothing it does not know',
        () {
      expect(probeBuildInfoFrom(), isEmpty);
      expect(
          probeBuildInfoFrom(
              flutterVersion: '',
              flutterChannel: null,
              frameworkRevision: '',
              dartVersion: ''),
          isEmpty);
    });

    test(
        'the Dart version falls back to the running VM, the Flutter fields do not fall back to anything',
        () {
      final m = probeBuildInfoFrom(
          platformVersion:
              '3.12.2 (stable) (Tue Jul 21 10:00:00 2026 -0700) on "android_arm64"');
      expect(m, {'dart_version': '3.12.2'});
    });

    test('a value that is not plain version text is left out, not repaired',
        () {
      final hostile = <String>[
        '3.50.0 \n3.49.0', // a second line
        '3.50.0; rm -rf /',
        '<script>alert(1)</script>',
        '3.50.0\u202e', // a bidi override
        '3.5\u200b0.0', // a zero-width space
        'v3 50',
        'é',
        'a' * 65,
        ' 3.50.0',
        '3.50.0 ',
      ];
      for (final h in hostile) {
        final m = probeBuildInfoFrom(
            flutterVersion: h,
            flutterChannel: h,
            frameworkRevision: h,
            dartVersion: h);
        expect(m, isEmpty, reason: 'accepted ${h.codeUnits}');
      }
    });

    test('64 characters are allowed, 65 are not', () {
      expect(probeBuildInfoFrom(flutterVersion: 'a' * 64),
          {'flutter_version': 'a' * 64});
      expect(probeBuildInfoFrom(flutterVersion: 'a' * 65), isEmpty);
    });

    test('a bad build value does not hide a good VM version', () {
      final m = probeBuildInfoFrom(
          dartVersion: 'not plain!', platformVersion: '3.12.2 (stable)');
      expect(m['dart_version'], '3.12.2');
    });

    test('letters, digits, dots, dashes and plus signs are all plain', () {
      final m = probeBuildInfoFrom(
          flutterVersion: '3.50.0-0.1.pre+build.7',
          flutterChannel: 'beta',
          frameworkRevision: 'AB12cd');
      expect(m['flutter_version'], '3.50.0-0.1.pre+build.7');
      expect(m['flutter_channel'], 'beta');
      expect(m['framework_revision'], 'AB12cd');
    });
  });

  group('dartVersionOf', () {
    test('takes the leading version of Platform.version', () {
      expect(
          dartVersionOf('3.12.2 (stable) (Tue Jul 21 2026) on "macos_arm64"'),
          '3.12.2');
      expect(dartVersionOf('  3.14.0-dev.1.0 (dev)'), '3.14.0-dev.1.0');
    });
    test('says nothing for text that does not start with a version', () {
      expect(dartVersionOf(null), isNull);
      expect(dartVersionOf(''), isNull);
      expect(dartVersionOf('(stable) 3.12.2'), isNull);
      expect(dartVersionOf('3.12.2;x (stable)'), isNull);
    });
  });

  test(
      'the real build info of this test run is plain text (whatever the tool set)',
      () {
    final m = probeBuildInfo();
    const allowed = {
      'flutter_version',
      'flutter_channel',
      'framework_revision',
      'dart_version'
    };
    expect(allowed.containsAll(m.keys), isTrue, reason: '$m');
    for (final v in m.values) {
      expect(RegExp(r'^[A-Za-z0-9.+-]{1,64}$').hasMatch(v), isTrue, reason: v);
    }
    // the running VM always gives a Dart version
    expect(m['dart_version'], isNotNull);
  });
}
