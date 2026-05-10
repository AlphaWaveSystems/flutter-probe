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

/// Emits: `see "X"`, `see exactly N "X"`, `see "X" enabled`,
/// `see "X" containing "Y"`, `see "X" matching "regex"`.
class See extends Step {
  final String text;
  final SeeState state;
  final String? containing;
  final String? matching;
  final int? exactly;
  const See(
    this.text, {
    this.state = SeeState.none,
    this.containing,
    this.matching,
    this.exactly,
  });
}

/// Emits: `don't see "X"`.
class DontSee extends Step {
  final String text;
  const DontSee(this.text);
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

class WaitUntil extends Wait {
  final String target;
  final bool appears; // false → disappears
  const WaitUntil._(this.target, this.appears);
  const WaitUntil.appears(String target) : this._(target, true);
  const WaitUntil.disappears(String target) : this._(target, false);
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
