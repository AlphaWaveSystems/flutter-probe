import 'package:flutter_probe_annotation/flutter_probe_annotation.dart';

@ProbeSuite(
  beforeAll: [EnrollBiometric()],
  tests: [
    ProbeTest('matching face unlocks app', tags: ['biometric', 'happy'], steps: [
      Open(),
      Tap(text: 'Sign in with Face ID'),
      BiometricMatch(),
      WaitUntil.appears('Dashboard'),
      See('Dashboard'),
    ]),
    ProbeTest('non-matching face is rejected', steps: [
      Open(),
      Tap(text: 'Sign in with Face ID'),
      BiometricNoMatch(),
      See('Authentication failed'),
      DontSee('Dashboard'),
    ]),
  ],
)
class BiometricAuthScreen {}
