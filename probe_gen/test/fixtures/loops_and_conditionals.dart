import 'package:flutter_probe_annotation/flutter_probe_annotation.dart';

@ProbeSuite(
  tests: [
    ProbeTest('handles onboarding and scrolls', steps: [
      Open(),
      If('Welcome to onboarding', then: [
        Tap(text: 'Skip'),
      ], otherwise: [
        Tap(text: 'Continue'),
      ]),
      Repeat(3, body: [
        Swipe.up(),
        WaitFor.duration(0.5),
      ]),
      See('End of list'),
    ]),
  ],
)
class OnboardingScreen {}
