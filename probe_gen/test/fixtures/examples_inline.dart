import 'package:flutter_probe_annotation/flutter_probe_annotation.dart';

@ProbeSuite(
  tests: [
    ProbeTest('login with <email>', steps: [
      Open(),
      Type('<email>', into: Field(id: 'email')),
      Type('<password>', into: Field(id: 'password')),
      Tap(text: 'Sign In'),
      See('<result>'),
    ], examples: Examples(
      headers: ['email', 'password', 'result'],
      rows: [
        ['alice@test.com', 'hunter2', 'Dashboard'],
        ['bob@test.com', 'wrong', 'Invalid credentials'],
      ],
    )),
  ],
)
class InlineExamplesScreen {}
