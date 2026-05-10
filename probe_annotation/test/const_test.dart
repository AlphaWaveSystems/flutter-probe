// Verifies that every annotation/step/selector type can be const-constructed.
// If this file compiles, all the constructors are valid for use in @-annotations.

import 'package:flutter_probe_annotation/flutter_probe_annotation.dart';
import 'package:test/test.dart';

void main() {
  test('annotation types are const-constructible', () {
    const suite = ProbeSuite(
      name: 'Login',
      beforeEach: [Open()],
      tests: [
        ProbeTest('user can log in', tags: ['smoke'], steps: [
          Tap(id: 'email'),
          Type('alice@example.com'),
          Tap(text: 'Sign In'),
          WaitUntil.appears('Dashboard'),
          See('Dashboard'),
          DontSee('Error'),
        ]),
        ProbeTest('with examples', steps: [
          Type('<email>'),
        ], examples: Examples(headers: ['email'], rows: [
          ['a@b.com'],
          ['c@d.com'],
        ])),
      ],
      recipes: [
        ProbeRecipe('sign in', params: ['email', 'password'], steps: [
          Type('<email>'),
        ]),
      ],
    );
    expect(suite.tests.length, 2);
    expect(suite.tests[0].steps.length, 6);
  });

  test('all step types are const-constructible', () {
    // Just construct one of each; success means they all compile as const.
    const steps = <Step>[
      Open(),
      OpenLink('https://example.com'),
      Close(),
      Restart(),
      Kill(),
      ClearAppData(),
      Tap(id: 'x'),
      DoubleTap(text: 'y'),
      LongPress(id: 'z'),
      Press('home'),
      GoBack(),
      Type('hello'),
      Clear(id: 'field'),
      Swipe.up(),
      Scroll.down(),
      Drag(from: IdSel('a'), to: IdSel('b')),
      Pinch(zoomIn: true),
      Rotate.landscape(),
      Toggle('switch'),
      Shake(),
      AllowPermission('camera'),
      DenyPermission('mic'),
      GrantAllPermissions(),
      RevokeAllPermissions(),
      CopyToClipboard('x'),
      PasteFromClipboard(),
      SetLocation(37.7749, -122.4194),
      VerifyExternalBrowser(),
      TakeScreenshot('shot'),
      CompareScreenshot('shot'),
      DumpWidgetTree(),
      SaveLogs(),
      Pause(),
      Log('hi'),
      Store('val', as: 'name'),
      See('Dashboard'),
      DontSee('Error'),
      WaitFor.duration(2),
      WaitForPageLoad(),
      WaitForNetworkIdle(),
      WaitForAnimations(),
      WaitUntil.appears('X'),
      WaitUntil.disappears('Y'),
      If('Onboarding', then: [Tap(text: 'Skip')]),
      Repeat(3, body: [Swipe.up()]),
      RunDart('print("hi");'),
      Mock(method: HttpMethod.get, path: '/api'),
      CallHttp(method: HttpMethod.post, url: 'https://x'),
      RecipeStep('login', args: ['a@b.com', 'pw']),
    ];
    expect(steps.length, greaterThan(40));
  });

  test('all selector types are const-constructible', () {
    const selectors = <Selector>[
      TextSel('Login'),
      IdSel('login_button'),
      TypeSel('ElevatedButton'),
      Field(text: 'Email'),
      Field(id: 'email'),
      Ordinal(2, 'Item'),
      Ordinal(1, 'Item', container: 'List'),
      Below('a', anchor: 'b'),
      Above('a', anchor: 'b'),
      LeftOf('a', anchor: 'b'),
      RightOf('a', anchor: 'b'),
      InContainer('Email', container: 'LoginForm'),
    ];
    expect(selectors.length, 12);
  });
}
