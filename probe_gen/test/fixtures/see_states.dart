import 'package:flutter_probe_annotation/flutter_probe_annotation.dart';

@ProbeSuite(
  tests: [
    ProbeTest('exhaustive see assertions', steps: [
      See('Welcome'),
      See('Sign In', state: SeeState.enabled),
      See('Submit', state: SeeState.disabled),
      See('Agree to terms', state: SeeState.checked),
      See('email', state: SeeState.focused),
      See('Logout', containing: 'out'),
      See('123-456-7890', matching: r'^\d{3}-\d{3}-\d{4}$'),
      // Combined: state + containing in the same assertion (the bug fix).
      See('email field', state: SeeState.enabled, containing: 'email'),
      See('Item', exactly: 5),
      DontSee('Error'),
      DontSee.id('error_banner'),
      // Selector forms — id and rich selector.
      See.id('password_field', state: SeeState.focused),
      See.selector(Ordinal(2, 'List Item')),
      See.selector(Below('Subtitle', anchor: 'Title')),
    ]),
  ],
)
class SeeStatesScreen {}
