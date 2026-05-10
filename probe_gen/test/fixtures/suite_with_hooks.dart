import 'package:flutter_probe_annotation/flutter_probe_annotation.dart';

@ProbeSuite(
  beforeAll: [
    Open(),
    AllowPermission('camera'),
  ],
  beforeEach: [
    See('Home'),
  ],
  afterEach: [
    TakeScreenshot('after_each'),
  ],
  onFailure: [
    DumpWidgetTree(),
    SaveLogs(),
  ],
  afterAll: [
    Close(),
  ],
  tests: [
    ProbeTest('renders home', steps: [
      See('Welcome'),
    ]),
  ],
)
class HomeScreen {}
