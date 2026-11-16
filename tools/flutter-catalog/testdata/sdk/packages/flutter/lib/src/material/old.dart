import 'package:flutter/widgets.dart';

@Deprecated(
  'Use NewBar instead. '
  'This feature was deprecated after v0.0.1.',
)
class OldBar extends StatelessWidget {}

@deprecated
class OldestBar extends StatelessWidget {}

class NewBar extends StatelessWidget {}

class Loop1 extends Loop2 {}

class Loop2 extends Loop1 {}
