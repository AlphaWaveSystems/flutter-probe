# Plan: Native Agent Architecture — Android (Kotlin) + iOS (Swift)

## Context

FlutterProbe's Go CLI and ProbeScript language are 85% app-agnostic. The bottleneck for native app support is the **on-device agent** — currently a Dart library (`probe_agent/`) that walks Flutter's widget tree via `WidgetsBinding.instance.rootElement`. Native Android/iOS apps have no Flutter widget tree — they need agents built on platform-native UI automation APIs.

**Goal**: Design two new agent implementations (Kotlin for Android, Swift for iOS) that speak the same JSON-RPC 2.0 protocol as the Dart agent, so the Go CLI and ProbeScript work unchanged.

**Non-goal**: Renaming to AnyProbe (deferred until native agents ship).

---

## Architecture Overview

```
                    ┌──────────────────────┐
                    │   probe CLI (Go)     │  ← Unchanged
                    │   ProbeScript parser │
                    │   Test runner        │
                    └────────┬─────────────┘
                             │ WebSocket + JSON-RPC 2.0
                ┌────────────┼────────────────┐
                │            │                │
     ┌──────────▼──┐  ┌─────▼────────┐  ┌────▼──────────┐
     │ Dart Agent   │  │ Kotlin Agent │  │ Swift Agent   │
     │ (Flutter)    │  │ (Android)    │  │ (iOS)         │
     │ Widget tree  │  │ UIAutomator  │  │ XCUITest      │
     └──────────────┘  └──────────────┘  └───────────────┘
```

All three agents implement the **same JSON-RPC protocol** (22 methods defined in `internal/probelink/protocol.go`). The CLI doesn't need to know which agent is running — it sends the same commands regardless.

---

## Part 1: Shared Protocol Contract

The agents must implement these JSON-RPC methods (from `protocol.go`):

### Core Commands (must implement)
| Method | Params | Description |
|--------|--------|-------------|
| `probe.ping` | none | Health check, return `{ok: true}` |
| `probe.open` | `{screen?}` | Launch/bring app to foreground |
| `probe.tap` | `{selector}` | Tap an element |
| `probe.type` | `{selector, text}` | Type text into a field |
| `probe.see` | `{selector, negated, count?, check?, pattern?}` | Assert element visibility |
| `probe.wait` | `{kind, target?, duration?, timeout?}` | Wait for condition |
| `probe.swipe` | `{direction, selector?}` | Swipe gesture |
| `probe.scroll` | `{direction, selector?}` | Scroll gesture |
| `probe.long_press` | `{selector}` | Long press |
| `probe.double_tap` | `{selector}` | Double tap |
| `probe.clear` | `{selector}` | Clear text field |
| `probe.screenshot` | `{name}` | Take screenshot, return `{path}` |
| `probe.dump_tree` | none | Return UI tree as string |
| `probe.settled` | none | Wait for UI idle |

### Selector Types (from `SelectorParam`)
| Kind | Description | Flutter | Android Native | iOS Native |
|------|-------------|---------|---------------|------------|
| `text` | Match by visible text | `Text` widget content | `AccessibilityNodeInfo.text` | `XCUIElement.label` |
| `id` | Match by identifier | `ValueKey('id')` | `resourceId` / `contentDescription` | `accessibilityIdentifier` |
| `type` | Match by widget/view type | Widget class name | View class name | Element type |
| `ordinal` | Nth occurrence | `1st "Item"` | Nth match in tree | Nth match in tree |
| `positional` | Element inside container | `"text" in "Container"` | Nested accessibility tree | Nested element query |

### Optional Commands (can defer)
| Method | Notes |
|--------|-------|
| `probe.drag` | Complex gesture — defer |
| `probe.run_dart` | Flutter-only, return "not supported" for native |
| `probe.mock` | HTTP mocking — requires different approach per platform |
| `probe.device_action` | rotate, dark mode — defer to CLI-level ADB/simctl |
| `probe.start_recording` / `probe.stop_recording` | Recording — phase 2 |

---

## Part 2: Android Native Agent (Kotlin)

### File: `probe_agent_android/`

```
probe_agent_android/
├── build.gradle.kts
├── src/main/kotlin/com/flutterprobe/agent/
│   ├── ProbeServer.kt          # WebSocket server + token auth
│   ├── ProbeExecutor.kt        # JSON-RPC dispatcher
│   ├── ProbeFinder.kt          # UI element finder (UIAutomator)
│   ├── ProbeGestures.kt        # Tap, swipe, scroll, long press
│   ├── ProbeSync.kt            # Wait for UI idle
│   ├── ProbeProtocol.kt        # JSON-RPC message types
│   └── ProbeRecorder.kt        # Accessibility event recording (phase 2)
└── src/test/kotlin/             # Unit tests
```

### Integration Model

The Kotlin agent runs as an **Android Instrumentation test** that starts a WebSocket server inside the test process. This gives it access to `UiAutomation` and `Instrumentation` APIs.

```kotlin
// Entry point: runs as androidTest
@RunWith(AndroidJUnit4::class)
class ProbeAgentRunner {
    @Test
    fun startAgent() {
        val server = ProbeServer(port = 48686)
        server.start()  // blocks until CLI disconnects
    }
}
```

**Why Instrumentation?** UIAutomator requires an `Instrumentation` context. Running as an `androidTest` APK gives full access to `UiAutomation`, `UiDevice`, and `Instrumentation` — without modifying the target app's source code.

**App integration**: Unlike the Flutter agent (embedded in the app), the Kotlin agent is a **separate test APK** installed alongside the target app. The CLI installs both: `adb install app.apk && adb install probe-agent-test.apk && adb shell am instrument ...`

### Key Implementation Details

#### ProbeFinder.kt — Element Finding
```kotlin
class ProbeFinder(private val device: UiDevice) {

    fun find(selector: SelectorParam): UiObject2? {
        return when (selector.kind) {
            "text" -> device.findObject(By.text(selector.text))
                ?: device.findObject(By.desc(selector.text))  // fallback to contentDescription
            "id" -> device.findObject(By.res(selector.text))
                ?: device.findObject(By.desc(selector.text))
            "type" -> device.findObject(By.clazz(selector.text))
            "ordinal" -> {
                val all = device.findObjects(By.text(selector.text))
                all.getOrNull(selector.ordinal - 1)
            }
            "positional" -> {
                val container = device.findObject(By.text(selector.container))
                container?.findObject(By.text(selector.text))
            }
            else -> null
        }
    }

    fun exists(selector: SelectorParam): Boolean = find(selector) != null
}
```

#### ProbeGestures.kt — Gesture Execution
```kotlin
class ProbeGestures(private val device: UiDevice) {

    fun tap(element: UiObject2) {
        element.click()
    }

    fun typeText(element: UiObject2, text: String) {
        element.click()  // focus
        element.text = text
    }

    fun swipe(direction: String, element: UiObject2? = null) {
        val target = element ?: return device.swipe(...)
        when (direction) {
            "up" -> target.swipe(Direction.UP, 0.8f)
            "down" -> target.swipe(Direction.DOWN, 0.8f)
            "left" -> target.swipe(Direction.LEFT, 0.8f)
            "right" -> target.swipe(Direction.RIGHT, 0.8f)
        }
    }

    fun longPress(element: UiObject2) {
        element.longClick()
    }
}
```

#### ProbeSync.kt — UI Idle Detection
```kotlin
class ProbeSync(private val device: UiDevice) {

    fun waitForIdle(timeoutMs: Long = 5000) {
        device.waitForIdle(timeoutMs)
    }

    fun waitForElement(selector: SelectorParam, timeoutMs: Long = 10000): Boolean {
        val condition = Until.findObject(selectorToBy(selector))
        return device.wait(condition, timeoutMs) != null
    }
}
```

#### ProbeServer.kt — WebSocket Server
```kotlin
// Uses NanoHTTPD or ktor-server-netty for WebSocket
class ProbeServer(private val port: Int = 48686) {
    private val token = generateToken(32)

    fun start() {
        // Print token for CLI pickup (same pattern as Dart agent)
        println("PROBE_TOKEN=$token")

        // Write token to file for fast-path reading
        File("/data/local/tmp/probe/token").apply {
            parentFile?.mkdirs()
            writeText(token)
        }

        // Start WebSocket server on port, authenticate via token query param
        // Dispatch JSON-RPC messages to ProbeExecutor
    }
}
```

### Dependencies
```kotlin
dependencies {
    androidTestImplementation("androidx.test.uiautomator:uiautomator:2.3.0")
    androidTestImplementation("androidx.test:runner:1.6.2")
    androidTestImplementation("org.nanohttpd:nanohttpd-websocket:2.3.1")
    // OR: io.ktor:ktor-server-netty + ktor-websockets
}
```

### CLI Changes for Android Native
```go
// internal/runner/device_context.go — add native agent launch
func (dc *DeviceContext) launchNativeAgent(ctx context.Context) error {
    // Install test APK
    dc.adb("install", "-t", probeAgentAPK)
    // Start instrumentation
    dc.adb("shell", "am", "instrument", "-w",
        "-e", "class", "com.flutterprobe.agent.ProbeAgentRunner",
        "com.flutterprobe.agent.test/androidx.test.runner.AndroidJUnitRunner")
    return nil
}
```

---

## Part 3: iOS Native Agent (Swift)

### File: `probe_agent_ios/`

```
probe_agent_ios/
├── Package.swift
├── Sources/ProbeAgent/
│   ├── ProbeServer.swift        # WebSocket server + token auth
│   ├── ProbeExecutor.swift      # JSON-RPC dispatcher
│   ├── ProbeFinder.swift        # XCUIElement queries
│   ├── ProbeGestures.swift      # Tap, swipe, scroll
│   ├── ProbeSync.swift          # XCTest synchronization
│   └── ProbeProtocol.swift      # JSON-RPC types
└── Tests/
```

### Integration Model

The Swift agent runs as an **XCUITest** that starts a WebSocket server. XCUITest runs in a separate process from the target app, with full access to `XCUIApplication` for UI automation.

```swift
class ProbeAgentTests: XCTestCase {
    func testStartAgent() {
        let app = XCUIApplication()
        app.launch()

        let server = ProbeServer(port: 48686, app: app)
        server.start()  // blocks until CLI disconnects
    }
}
```

**App integration**: Like Android, the Swift agent is a **separate test target** — no source code changes to the target app. The CLI runs `xcodebuild test` with the ProbeAgent test target.

### Key Implementation Details

#### ProbeFinder.swift
```swift
class ProbeFinder {
    let app: XCUIApplication

    func find(_ selector: SelectorParam) -> XCUIElement? {
        switch selector.kind {
        case "text":
            let predicate = NSPredicate(format: "label == %@", selector.text)
            let match = app.descendants(matching: .any).matching(predicate).firstMatch
            return match.exists ? match : nil

        case "id":
            let element = app.descendants(matching: .any)[selector.text]
            return element.exists ? element : nil

        case "type":
            let type = xcuiElementType(from: selector.text)
            let elements = app.descendants(matching: type)
            return elements.count > 0 ? elements.firstMatch : nil

        case "ordinal":
            let predicate = NSPredicate(format: "label == %@", selector.text)
            let matches = app.descendants(matching: .any).matching(predicate)
            let idx = selector.ordinal - 1
            return idx < matches.count ? matches.element(boundBy: idx) : nil

        case "positional":
            let container = app.descendants(matching: .any)
                .matching(NSPredicate(format: "label == %@", selector.container)).firstMatch
            return container.descendants(matching: .any)
                .matching(NSPredicate(format: "label == %@", selector.text)).firstMatch

        default:
            return nil
        }
    }
}
```

#### ProbeGestures.swift
```swift
class ProbeGestures {
    func tap(_ element: XCUIElement) {
        element.tap()
    }

    func typeText(_ element: XCUIElement, text: String) {
        element.tap()
        element.typeText(text)
    }

    func swipe(_ direction: String, element: XCUIElement? = nil) {
        let target = element ?? XCUIApplication()
        switch direction {
        case "up": target.swipeUp()
        case "down": target.swipeDown()
        case "left": target.swipeLeft()
        case "right": target.swipeRight()
        default: break
        }
    }

    func longPress(_ element: XCUIElement, duration: TimeInterval = 1.0) {
        element.press(forDuration: duration)
    }
}
```

#### ProbeSync.swift
```swift
class ProbeSync {
    let app: XCUIApplication

    func waitForElement(_ selector: SelectorParam, timeout: TimeInterval = 10) -> Bool {
        guard let element = ProbeFinder(app: app).find(selector) else { return false }
        return element.waitForExistence(timeout: timeout)
    }

    func waitForIdle() {
        RunLoop.current.run(until: Date(timeIntervalSinceNow: 0.5))
    }
}
```

### CLI Changes for iOS Native
```go
// internal/runner/device_context.go
func (dc *DeviceContext) launchNativeAgentIOS(ctx context.Context) error {
    dc.exec("xcodebuild", "test",
        "-project", appProject,
        "-scheme", "ProbeAgent",
        "-destination", fmt.Sprintf("platform=iOS Simulator,id=%s", dc.udid),
        "-only-testing", "ProbeAgentTests/ProbeAgentTests/testStartAgent")
    return nil
}
```

---

## Part 4: Go CLI Modifications

### Agent Type Detection

Add to `internal/config/config.go`:
```go
type AgentType string
const (
    AgentFlutter AgentType = "flutter"
    AgentAndroid AgentType = "android"
    AgentIOS     AgentType = "ios"
)
```

In `probe.yaml`:
```yaml
project:
  name: MyApp
  app: com.example.myapp
  agent_type: android    # flutter (default) | android | ios
```

CLI flag: `--agent-type flutter|android|ios` (overrides probe.yaml).

### Auto-Detection (future)

The CLI could auto-detect by checking:
1. If `pubspec.yaml` exists → Flutter
2. If `build.gradle.kts` + no `pubspec.yaml` → Android native
3. If `*.xcodeproj` + no `pubspec.yaml` → iOS native

### Runner Changes

`internal/runner/device_context.go` — The only changes needed:

```go
func (dc *DeviceContext) installAndLaunchAgent(ctx context.Context) error {
    switch dc.config.AgentType {
    case config.AgentFlutter:
        // Current behavior: app has ProbeAgent embedded
        return dc.launchFlutterApp(ctx)
    case config.AgentAndroid:
        // Install test APK, start instrumentation
        return dc.launchNativeAndroidAgent(ctx)
    case config.AgentIOS:
        // Build and run XCUITest
        return dc.launchNativeIOSAgent(ctx)
    }
}
```

After the agent starts, **everything else is identical** — WebSocket connect, JSON-RPC dispatch, test execution, reporting.

### Protocol Changes

**None.** The `probe.run_dart` method returns an error for native agents:
```json
{"error": {"code": -32601, "message": "run_dart not supported for native agents"}}
```

The runner already handles RPC errors gracefully.

---

## Part 5: What `probe.run_dart` Becomes for Native

For Flutter, `run dart:` blocks execute arbitrary Dart code on the agent. For native:

- **Android**: `probe.run_code` → executes Kotlin/Java via `ScriptEngine` or returns "not supported"
- **iOS**: `probe.run_code` → not supported (no runtime code execution in XCUITest)
- **Alternative**: convert common `dart:` patterns to platform-level commands (e.g., `Platform.isAndroid` → always true for Android agent)

This is a ProbeScript compatibility issue — tests using `run dart:` blocks won't work on native. Document this as a known limitation.

---

## Part 6: Implementation Phases

### Phase 1: Android Agent MVP (2-3 weeks)
1. Scaffold `probe_agent_android/` Kotlin project
2. Implement `ProbeServer.kt` (WebSocket + token auth)
3. Implement `ProbeFinder.kt` (text, id, type selectors)
4. Implement `ProbeGestures.kt` (tap, type, swipe, long press)
5. Implement `ProbeSync.kt` (waitForIdle, waitForElement)
6. Implement `ProbeExecutor.kt` (dispatch 14 core methods)
7. Add `--agent-type android` to Go CLI
8. Add `launchNativeAndroidAgent()` to device_context.go
9. Test against a sample Kotlin app

### Phase 2: iOS Agent MVP (2-3 weeks)
1. Scaffold `probe_agent_ios/` Swift package
2. Implement same 6 components using XCUITest APIs
3. Add `launchNativeIOSAgent()` to device_context.go
4. Test against a sample Swift app

### Phase 3: Polish & Parity (2 weeks)
1. Ordinal and positional selector support
2. Screenshot implementation
3. Widget/view tree dump
4. Recording support (phase 2 feature)
5. Auto-detection of agent type
6. Documentation and examples

### Phase 4: Rename Consideration
- Once both native agents are production-quality
- Rename CLI binary from `probe` to `anyprobe` (or keep `probe`)
- Update branding, website, cloud dashboard
- Keep `flutter-probe` repo but add `probe-agent-android` and `probe-agent-ios` repos

---

## Part 7: Key Files to Modify/Create

| File | Action | Description |
|------|--------|-------------|
| `probe_agent_android/` | **Create** | New Kotlin module (6 source files) |
| `probe_agent_ios/` | **Create** | New Swift package (6 source files) |
| `internal/config/config.go` | Modify | Add `AgentType` field |
| `internal/runner/device_context.go` | Modify | Add native agent launch methods |
| `internal/cli/test.go` | Modify | Add `--agent-type` flag |
| `probe_agent/` | Unchanged | Flutter agent stays as-is |

---

## Part 8: Risk Assessment

| Risk | Impact | Mitigation |
|------|--------|------------|
| UIAutomator selector accuracy vs Flutter finder | Medium | Extensive selector fallback chain (text → desc → resource-id) |
| XCUITest WebSocket server in test process | High | May need to use XCTest's built-in HTTP hooks or a sidecar process |
| `run dart:` incompatibility | Low | Document as limitation, offer `run shell:` alternative |
| Maintenance burden (3 agents) | High | Strict protocol contract — all agents pass the same test suite |
| Token auth differences | Low | Same mechanism works (stdout + file) for all platforms |

---

## Verification

1. **Protocol compliance**: Create a Go test suite that connects to any agent and runs all 14 core methods — all three agents must pass the same suite
2. **ProbeScript compatibility**: Run the same `.probe` test file against a Flutter app, a Kotlin app, and a Swift app with equivalent UIs — all must pass
3. **CLI transparency**: User runs `probe test tests/ --device <UDID> -v -y` and it works regardless of agent type
