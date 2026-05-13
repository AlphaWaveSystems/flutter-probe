import 'package:flutter_probe_annotation/flutter_probe_annotation.dart';

@ProbeSuite(
  tests: [
    ProbeTest('every wait variant', steps: [
      Open(),
      WaitFor.duration(1.5),
      WaitForPageLoad(),
      WaitForNetworkIdle(),
      WaitForAnimations(),
      WaitUntil.appears('Dashboard'),
      WaitUntil.disappears('Loading'),
      WaitUntil.idAppears('login_form'),
      WaitUntil.idDisappears('spinner'),
      See('Dashboard'),
    ]),
  ],
)
class WaitVariantsScreen {}
