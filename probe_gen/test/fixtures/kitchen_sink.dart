import 'package:flutter_probe_annotation/flutter_probe_annotation.dart';

@ProbeSuite(
  tests: [
    ProbeTest('kitchen sink — one of every step type', steps: [
      // App lifecycle
      Open(),
      OpenLink('https://example.com/foo'),
      Close(),
      Restart(),
      Kill(),
      ClearAppData(),
      // Tap-family
      Tap(text: 'Login'),
      Tap(id: 'submit'),
      Tap(text: 'Cookies', ifVisible: true),
      DoubleTap(text: 'Map'),
      LongPress(id: 'thumbnail'),
      GoBack(),
      // Text input
      Type('alice@example.com', into: Field(id: 'email')),
      Type('hunter2', into: Field(text: 'Password'), ifVisible: true),
      Clear(id: 'search'),
      // Motion
      Swipe.up(),
      Swipe.down(on: IdSel('list')),
      Scroll.up(),
      Scroll.right(on: TextSel('Container')),
      Drag(from: IdSel('a'), to: IdSel('b')),
      Rotate.landscape(),
      Toggle('Notifications'),
      Shake(),
      // Permissions
      AllowPermission('camera'),
      DenyPermission('contacts'),
      GrantAllPermissions(),
      RevokeAllPermissions(),
      // Clipboard / device
      CopyToClipboard('hello world'),
      PasteFromClipboard(),
      SetLocation(37.7749, -122.4194),
      VerifyExternalBrowser(),
      // Diagnostics
      TakeScreenshot('snap'),
      CompareScreenshot('snap'),
      DumpWidgetTree(),
      SaveLogs(),
      Pause(),
      Log('breadcrumb here'),
      // Variables
      Store('xyz', as: 'token'),
      // Control flow
      If('Onboarding', then: [Tap(text: 'Skip')], otherwise: [Tap(text: 'Continue')]),
      Repeat(2, body: [Swipe.up(), WaitFor.duration(0.25)]),
      // Selector zoo via See
      See.selector(TypeSel('ElevatedButton')),
      See.selector(InContainer('Email', container: 'LoginForm')),
      See.selector(Above('Title', anchor: 'Subtitle')),
      See.selector(LeftOf('Item1', anchor: 'Item2')),
      See.selector(RightOf('Item3', anchor: 'Item2')),
      // Dart escape
      RunDart('debugPrint("hi");\nfinal x = 42;'),
    ]),
  ],
)
class KitchenSinkScreen {}
