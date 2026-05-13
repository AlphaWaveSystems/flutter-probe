/// The full step DSL for ProbeScript. Each subclass of [Step] corresponds to
/// exactly one ProbeScript construct and has a fully `const` constructor so
/// it can be used inside annotations.
library;

import 'selectors.dart';

/// Base type for every test step.
sealed class Step {
  const Step();
}

// ---- App lifecycle ----

/// Emits: `open the app`.
class Open extends Step {
  const Open();
}

/// Emits: `open link "url"`.
class OpenLink extends Step {
  final String url;
  const OpenLink(this.url);
}

/// Emits: `close the app`.
class Close extends Step {
  const Close();
}

/// Emits: `restart the app`. Force-stops and relaunches preserving data.
class Restart extends Step {
  const Restart();
}

/// Emits: `kill the app`. Force-stops without relaunching.
class Kill extends Step {
  const Kill();
}

/// Emits: `clear app data`. Destructive: wipes app storage and relaunches.
class ClearAppData extends Step {
  const ClearAppData();
}

// ---- Tap-family actions ----

/// Emits: `tap "Text"` or `tap #id` (with optional `if visible` suffix).
class Tap extends Step {
  final String? text;
  final String? id;
  final Selector? selector;
  final bool ifVisible;
  const Tap({this.text, this.id, this.selector, this.ifVisible = false})
      : assert(text != null || id != null || selector != null,
            'Tap requires text:, id:, or selector:');
}

/// Emits: `double tap "X"`.
class DoubleTap extends Step {
  final String? text;
  final String? id;
  final Selector? selector;
  const DoubleTap({this.text, this.id, this.selector})
      : assert(text != null || id != null || selector != null);
}

/// Emits: `long press "X"`.
class LongPress extends Step {
  final String? text;
  final String? id;
  final Selector? selector;
  const LongPress({this.text, this.id, this.selector})
      : assert(text != null || id != null || selector != null);
}

/// Emits: `press home` / `press back` / `press <key>`.
///
/// Not yet supported by the FlutterProbe runtime — the Go-side parser has
/// no `press` case and the emitted text will be misparsed as a recipe
/// call. Will be re-enabled in a future release that wires runtime support.
@Deprecated('Not yet supported by the runtime — coming in a future release')
class Press extends Step {
  final String key; // home | back | volume_up | volume_down | …
  const Press(this.key);
}

/// Emits: `go back`. Equivalent to the platform back gesture.
class GoBack extends Step {
  const GoBack();
}

// ---- Text input ----

/// Emits: `type "text"` or `type "text" into #target`.
class Type extends Step {
  final String text;
  final Selector? into;
  final bool ifVisible;
  const Type(this.text, {this.into, this.ifVisible = false});
}

/// Emits: `clear "X"` — clears the value of a field.
class Clear extends Step {
  final String? text;
  final String? id;
  final Selector? selector;
  const Clear({this.text, this.id, this.selector})
      : assert(text != null || id != null || selector != null);
}

// ---- Scroll / swipe / drag ----

/// Direction for [Swipe] and [Scroll].
enum Direction { up, down, left, right }

/// Emits: `swipe up` / `swipe down "X"`.
class Swipe extends Step {
  final Direction direction;
  final Selector? on;
  const Swipe(this.direction, {this.on});
  const Swipe.up({Selector? on}) : this(Direction.up, on: on);
  const Swipe.down({Selector? on}) : this(Direction.down, on: on);
  const Swipe.left({Selector? on}) : this(Direction.left, on: on);
  const Swipe.right({Selector? on}) : this(Direction.right, on: on);
}

/// Emits: `scroll up` / `scroll down "Container"`.
class Scroll extends Step {
  final Direction direction;
  final Selector? on;
  const Scroll(this.direction, {this.on});
  const Scroll.up({Selector? on}) : this(Direction.up, on: on);
  const Scroll.down({Selector? on}) : this(Direction.down, on: on);
  const Scroll.left({Selector? on}) : this(Direction.left, on: on);
  const Scroll.right({Selector? on}) : this(Direction.right, on: on);
}

/// Emits: `drag "from" to "to"`.
class Drag extends Step {
  final Selector from;
  final Selector to;
  const Drag({required this.from, required this.to});
}

/// Emits: `pinch in` / `pinch out`.
///
/// Not yet supported by the FlutterProbe runtime — the Go-side parser has
/// no `pinch` case and the emitted text will be misparsed as a recipe
/// call. Will be re-enabled in a future release that wires runtime support.
@Deprecated('Not yet supported by the runtime — coming in a future release')
class Pinch extends Step {
  final bool zoomIn;
  const Pinch({this.zoomIn = false});
}

/// Emits: `rotate portrait` / `rotate landscape` / `rotate <name>`.
class Rotate extends Step {
  final String orientation;
  const Rotate(this.orientation);
  const Rotate.portrait() : this('portrait');
  const Rotate.landscape() : this('landscape');
}

/// Emits: `toggle "X"`.
class Toggle extends Step {
  final String name;
  const Toggle(this.name);
}

/// Emits: `shake`. Triggers a device shake gesture (simulators only).
class Shake extends Step {
  const Shake();
}

// ---- Permissions ----

/// Emits: `allow permission "name"`.
class AllowPermission extends Step {
  final String name;
  const AllowPermission(this.name);
}

/// Emits: `deny permission "name"`.
class DenyPermission extends Step {
  final String name;
  const DenyPermission(this.name);
}

/// Emits: `grant all permissions`.
class GrantAllPermissions extends Step {
  const GrantAllPermissions();
}

/// Emits: `revoke all permissions`.
class RevokeAllPermissions extends Step {
  const RevokeAllPermissions();
}

// ---- Clipboard / device control ----

/// Emits: `copy "text" to clipboard`.
class CopyToClipboard extends Step {
  final String text;
  const CopyToClipboard(this.text);
}

/// Emits: `paste from clipboard`.
class PasteFromClipboard extends Step {
  const PasteFromClipboard();
}

/// Emits: `set location lat, lng`. Simulator/emulator only.
class SetLocation extends Step {
  final double lat;
  final double lng;
  const SetLocation(this.lat, this.lng);
}

/// Emits: `verify external browser opened`.
class VerifyExternalBrowser extends Step {
  const VerifyExternalBrowser();
}

// ---- Biometric authentication (simulator/emulator only) ----

/// Emits: `enroll biometric`. Sets the simulator/emulator's biometric
/// enrollment state to "enrolled" so subsequent [BiometricMatch] or
/// [BiometricNoMatch] satisfy a pending Face ID / Touch ID / fingerprint
/// prompt in the app under test.
///
/// On iOS this posts the `com.apple.BiometricKit.enrollmentChanged` Darwin
/// notification via `xcrun simctl spawn booted notifyutil`. On Android the
/// fingerprint must already be enrolled in Settings; the step is a no-op
/// hint there. Skipped with a warning on physical devices.
class EnrollBiometric extends Step {
  const EnrollBiometric();
}

/// Emits: `biometric match`. Simulates a successful Face ID / Touch ID /
/// fingerprint capture, satisfying a pending biometric prompt.
///
/// iOS Simulator: posts `*_Sim.faceCapture.match` and `*_Sim.fingerTouch.match`
/// Darwin notifications so the same step works on Face ID and Touch ID
/// devices. Android emulator: `adb -s <serial> emu finger touch 1` (matches
/// the fingerprint enrolled with ID 1). Skipped on physical devices.
class BiometricMatch extends Step {
  const BiometricMatch();
}

/// Emits: `biometric no match`. Simulates a failed Face ID / Touch ID /
/// fingerprint capture so the app's "authentication failed" path can be
/// exercised.
///
/// iOS Simulator: posts `*_Sim.faceCapture.no-match` and `*_Sim.fingerTouch.no-match`.
/// Android emulator: `adb emu finger touch 9999` (an unregistered id).
/// Skipped on physical devices.
class BiometricNoMatch extends Step {
  const BiometricNoMatch();
}

// ---- Native-prompt signal API ----

/// Emits: `deliver signal "name"` or `deliver signal "name" "value"`.
///
/// Resolves a pending [awaitSignal] call in the Flutter app. Use to unblock
/// any OS-level interaction the probe cannot tap directly — permission dialogs
/// not in the widget tree, payment sheets, push notification prompts, etc.
///
/// ```dart
/// @ProbeSuite(tests: [
///   ProbeTest('push permission granted', steps: [
///     Open(),
///     DeliverSignal('push_permission'),
///     See('Notifications enabled'),
///   ]),
/// ])
/// class NotificationScreen extends StatelessWidget {}
/// ```
class DeliverSignal extends Step {
  final String name;
  final String value;
  const DeliverSignal(this.name, {this.value = 'true'});
}

// ---- Diagnostics ----

/// Emits: `take screenshot "name"`.
class TakeScreenshot extends Step {
  final String name;
  const TakeScreenshot(this.name);
}

/// Emits: `compare screenshot "name"` — visual regression compare.
class CompareScreenshot extends Step {
  final String name;
  const CompareScreenshot(this.name);
}

/// Emits: `dump widget tree`.
class DumpWidgetTree extends Step {
  const DumpWidgetTree();
}

/// Emits: `save logs`.
class SaveLogs extends Step {
  const SaveLogs();
}

/// Emits: `pause`. Pauses execution until manually resumed.
class Pause extends Step {
  const Pause();
}

/// Emits: `log "message"`.
class Log extends Step {
  final String message;
  const Log(this.message);
}

// ---- Variables / data flow ----

/// Emits: `store "value" as varName`. Captures a value for later substitution.
class Store extends Step {
  final String value;
  final String as;
  const Store(this.value, {required this.as});
}

// ---- Assertions ----

/// State checks for [See] assertions.
enum SeeState { none, enabled, disabled, checked, focused }

/// Emits ProbeScript assertions of the form:
///
///   see "X"
///   see exactly N "X"
///   see "X" is enabled
///   see "X" contains "Y"
///   see "X" matching "regex"
///   see #id is focused
///
/// `state`, `containing`, and `matching` are all suffixes that can coexist
/// — `See('Welcome', state: SeeState.enabled, containing: 'world')` emits
/// `see "Welcome" is enabled contains "world"`. Use the [See.id] /
/// [See.selector] factories to target by `ValueKey` or arbitrary selector
/// instead of visible text.
class See extends Step {
  final String? text;
  final String? id;
  final Selector? selector;
  final SeeState state;
  final String? containing;
  final String? matching;
  final int? exactly;
  const See(
    String this.text, {
    this.state = SeeState.none,
    this.containing,
    this.matching,
    this.exactly,
  })  : id = null,
        selector = null;

  /// Target a widget by its `ValueKey` or `Semantics.identifier`.
  const See.id(
    String this.id, {
    this.state = SeeState.none,
    this.containing,
    this.matching,
    this.exactly,
  })  : text = null,
        selector = null;

  /// Target a widget via an arbitrary [Selector] — ordinal, positional,
  /// relational, or by widget type.
  const See.selector(
    Selector this.selector, {
    this.state = SeeState.none,
    this.containing,
    this.matching,
    this.exactly,
  })  : text = null,
        id = null;
}

/// Emits negative assertions:
///
///   don't see "X"
///   don't see #id
///
/// Like [See], supports targeting by text, id, or arbitrary [Selector].
class DontSee extends Step {
  final String? text;
  final String? id;
  final Selector? selector;
  final SeeState state;
  final String? containing;
  final String? matching;
  final int? exactly;
  const DontSee(
    String this.text, {
    this.state = SeeState.none,
    this.containing,
    this.matching,
    this.exactly,
  })  : id = null,
        selector = null;

  const DontSee.id(
    String this.id, {
    this.state = SeeState.none,
    this.containing,
    this.matching,
    this.exactly,
  })  : text = null,
        selector = null;

  const DontSee.selector(
    Selector this.selector, {
    this.state = SeeState.none,
    this.containing,
    this.matching,
    this.exactly,
  })  : text = null,
        id = null;
}

// ---- Wait variants ----

/// Emits: `wait N seconds` / `wait until "X" appears` /
/// `wait for the page to load` / `wait until network is idle` /
/// `wait for animations to end`.
sealed class Wait extends Step {
  const Wait();
}

class WaitFor extends Wait {
  final double seconds;
  const WaitFor.duration(this.seconds);
}

class WaitForPageLoad extends Wait {
  const WaitForPageLoad();
}

class WaitForNetworkIdle extends Wait {
  const WaitForNetworkIdle();
}

class WaitForAnimations extends Wait {
  const WaitForAnimations();
}

/// Wait until a target widget appears or disappears.
///
///   WaitUntil.appears('Dashboard')      → wait until "Dashboard" appears
///   WaitUntil.disappears('Loading')     → wait until "Loading" disappears
///   WaitUntil.idAppears('login_form')   → wait until #login_form appears
///   WaitUntil.idDisappears('spinner')   → wait until #spinner disappears
///
/// The id-based factories emit unquoted `#key` selectors and exercise the
/// Go parser's WaitSelector branch — more reliable than text matching
/// for ValueKey-tagged widgets.
class WaitUntil extends Wait {
  final String target;
  final bool appears;
  final bool byId;
  const WaitUntil._(this.target, this.appears, this.byId);
  const WaitUntil.appears(String target) : this._(target, true, false);
  const WaitUntil.disappears(String target) : this._(target, false, false);
  const WaitUntil.idAppears(String key) : this._(key, true, true);
  const WaitUntil.idDisappears(String key) : this._(key, false, true);
}

// ---- Control flow (block steps) ----

/// Emits an `if "condition" appears` block with a body and optional otherwise branch.
class If extends Step {
  final String condition;
  final List<Step> then;
  final List<Step> otherwise;
  const If(this.condition, {this.then = const [], this.otherwise = const []});
}

/// Emits a `repeat N times` block.
class Repeat extends Step {
  final int times;
  final List<Step> body;
  const Repeat(this.times, {this.body = const []});
}

// ---- Dart escape hatch ----

/// Emits: `run dart:` block with raw Dart code (verbatim).
class RunDart extends Step {
  final String code;
  const RunDart(this.code);
}

// ---- HTTP ----

enum HttpMethod { get, post, put, delete }

/// Emits a `when the app calls METHOD /path` mock block.
class Mock extends Step {
  final HttpMethod method;
  final String path;
  final int status;
  final String body;
  const Mock({
    required this.method,
    required this.path,
    this.status = 200,
    this.body = '',
  });
}

/// Emits: `call METHOD "url" with body "..."`.
class CallHttp extends Step {
  final HttpMethod method;
  final String url;
  final String? body;
  const CallHttp({required this.method, required this.url, this.body});
}

// ---- Recipe invocation ----

/// Invokes a recipe declared via `@ProbeRecipe`. Emits the recipe name and
/// quoted arguments, e.g. `sign in "alice@example.com" "hunter2"`.
class RecipeStep extends Step {
  final String name;
  final List<String> args;
  const RecipeStep(this.name, {this.args = const []});
}

// ---- Composite test steps (only valid inside ProbeCompositeTest.body) ----

/// Scopes a group of [steps] to a single device in a composite test. The
/// [alias] must match a [Device.alias] declared in the enclosing
/// `@ProbeCompositeTest.devices`.
///
/// Emits an `<alias>:` header followed by the indented step body. Multiple
/// `OnDevice` entries for the same alias accumulate, separated by [Sync]
/// barriers.
class OnDevice extends Step {
  final String alias;
  final List<Step> steps;
  const OnDevice(this.alias, {this.steps = const []});
}

/// Cross-device barrier in a composite test. All devices must reach the
/// same [Sync] label before any device proceeds past it.
///
/// Emits `sync "label"` at the composite body level.
class Sync extends Step {
  final String label;
  const Sync(this.label);
}
