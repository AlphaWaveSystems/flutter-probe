import 'package:flutter_probe_annotation/flutter_probe_annotation.dart';

@ProbeSuite(
  recipes: [
    ProbeRecipe('sign in', params: ['email', 'password'], steps: [
      Tap(id: 'email_field'),
      Type('<email>'),
      Tap(id: 'password_field'),
      Type('<password>'),
      Tap(text: 'Sign In'),
    ]),
  ],
  tests: [
    ProbeTest('uses sign in recipe', steps: [
      Open(),
      RecipeStep('sign in', args: ['alice@example.com', 'hunter2']),
      See('Dashboard'),
    ]),
  ],
)
class AuthScreen {}
