import 'package:flutter_probe_annotation/flutter_probe_annotation.dart';

@ProbeSuite(
  tests: [
    ProbeTest('deliver default signal', steps: [
      Open(),
      DeliverSignal('push_permission'),
      See('Notifications enabled'),
    ]),
    ProbeTest('deliver signal with value', steps: [
      Open(),
      DeliverSignal('payment_result', value: 'success'),
      See('Payment confirmed'),
    ]),
  ],
)
class SignalScreen {}
