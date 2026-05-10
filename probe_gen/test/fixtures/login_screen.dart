import 'package:flutter_probe_annotation/flutter_probe_annotation.dart';

@ProbeSuite(
  name: 'Login',
  tests: [
    ProbeTest('user can log in', tags: ['smoke', 'critical'], steps: [
      Open(),
      Tap(id: 'email_field'),
      Type('alice@example.com'),
      Tap(id: 'password_field'),
      Type('hunter2'),
      Tap(text: 'Sign In'),
      WaitUntil.appears('Dashboard'),
      See('Dashboard'),
      DontSee('Invalid credentials'),
    ]),
    ProbeTest('shows error on bad password', steps: [
      Tap(id: 'email_field'),
      Type('alice@example.com'),
      Tap(text: 'Sign In'),
      See('Invalid credentials'),
    ]),
  ],
)
class LoginScreen {}
