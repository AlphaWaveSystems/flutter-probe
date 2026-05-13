/// Annotation classes that the `flutter_probe_gen` builder reads at build
/// time to emit `.probe` test files.
library;

import 'examples.dart';
import 'steps.dart';

/// Declares a ProbeScript test suite. Apply to any top-level Dart class
/// (typically a screen or page widget).
///
/// All fields are optional. Hooks (`beforeEach`, `afterEach`, `beforeAll`,
/// `afterAll`, `onFailure`) are emitted at the top of the generated `.probe`
/// file. Recipes are declared inline via [recipes] or — for sharing across
/// suites — via standalone classes annotated with [ProbeRecipe].
///
/// Example:
///
/// ```dart
/// @ProbeSuite(
///   beforeEach: [Open()],
///   tests: [
///     ProbeTest('happy path', steps: [
///       Tap(text: 'Sign In'),
///       See('Dashboard'),
///     ]),
///   ],
/// )
/// class LoginScreen extends StatelessWidget { /* … */ }
/// ```
class ProbeSuite {
  /// Optional human-readable suite name. Defaults to the annotated class name
  /// when emitting the generated `.probe` file's source comment.
  final String? name;

  /// The tests in this suite, each emitted as a top-level `test "..."` block.
  final List<ProbeTest> tests;

  /// Steps run before every test in this suite (`before each test`).
  final List<Step> beforeEach;

  /// Steps run after every test in this suite (`after each test`).
  final List<Step> afterEach;

  /// Steps run once before any test in this file (`before all tests`).
  final List<Step> beforeAll;

  /// Steps run once after all tests in this file (`after all tests`).
  final List<Step> afterAll;

  /// Steps run when any test fails (`on failure`).
  final List<Step> onFailure;

  /// Recipes declared inline alongside this suite. Each emits a `recipe`
  /// block at the top of the file.
  final List<ProbeRecipe> recipes;

  const ProbeSuite({
    this.name,
    this.tests = const [],
    this.beforeEach = const [],
    this.afterEach = const [],
    this.beforeAll = const [],
    this.afterAll = const [],
    this.onFailure = const [],
    this.recipes = const [],
  });
}

/// A single test case. Used inside [ProbeSuite.tests] or as a standalone
/// annotation on a class for one-off tests.
///
/// Emits: `test "name"` with optional `@tag` lines and an indented body of
/// the [steps].
class ProbeTest {
  final String name;
  final List<String> tags;
  final List<Step> steps;
  final Examples? examples;

  const ProbeTest(
    this.name, {
    this.tags = const [],
    this.steps = const [],
    this.examples,
  });
}

/// Declares a reusable recipe with named parameters. Use [RecipeStep] inside
/// a test's steps to invoke it.
///
/// Example:
///
/// ```dart
/// @ProbeRecipe('sign in', params: ['email', 'password'], steps: [
///   Tap(id: 'email_field'),
///   Type('<email>'),
///   Tap(id: 'password_field'),
///   Type('<password>'),
///   Tap(text: 'Sign In'),
/// ])
/// class SignInRecipe {}
/// ```
///
/// In step text fields, reference parameters as `<paramName>` — they are
/// substituted at test-run time by the row value or recipe argument.
class ProbeRecipe {
  final String name;
  final List<String> params;
  final List<Step> steps;

  const ProbeRecipe(
    this.name, {
    this.params = const [],
    this.steps = const [],
  });
}

/// Declares a multi-device composite test. The annotated class becomes a
/// `composite test "name"` block in the generated `.probe` file.
///
/// Devices are declared by alias (`A`, `B`, `Sender`, etc.) and steps are
/// scoped per-device using [OnDevice]. [Sync] barriers between [OnDevice]
/// groups force every device to reach the same checkpoint before any
/// device proceeds.
///
/// Example — chat between two users on two simulators:
///
/// ```dart
/// @ProbeCompositeTest(
///   name: 'alice sends bob a message',
///   tags: ['composite', 'smoke'],
///   devices: [
///     Device('A', target: 'iPhone 15 Simulator'),
///     Device('B', target: 'Pixel 9 Emulator'),
///   ],
///   body: [
///     OnDevice('A', steps: [
///       Open(),
///       Tap(text: 'Sign in as Alice'),
///     ]),
///     OnDevice('B', steps: [
///       Open(),
///       Tap(text: 'Sign in as Bob'),
///     ]),
///     Sync('both signed in'),
///     OnDevice('A', steps: [
///       Tap(text: 'New message'),
///       Type('hello bob'),
///       Tap(text: 'Send'),
///     ]),
///     OnDevice('B', steps: [
///       WaitUntil.appears('hello bob'),
///       See('hello bob'),
///     ]),
///   ],
/// )
/// class ChatComposite {}
/// ```
class ProbeCompositeTest {
  final String name;
  final List<String> tags;
  final List<Device> devices;

  /// Composite body — must contain only [OnDevice] and [Sync] elements.
  final List<Step> body;

  const ProbeCompositeTest({
    required this.name,
    this.tags = const [],
    this.devices = const [],
    this.body = const [],
  });
}

/// One device entry in a [ProbeCompositeTest.devices] list. The [alias]
/// is referenced by [OnDevice]; [target] is an optional human-readable
/// device name shown in the generated `.probe` header and in failure
/// messages.
class Device {
  final String alias;
  final String? target;
  const Device(this.alias, {this.target});
}
