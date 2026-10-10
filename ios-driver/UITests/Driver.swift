import XCTest

/// Finds and drives iOS *system* dialogs (permission alerts, the StoreKit
/// "Sign in to Apple Account" sheet, the app's share sheet, ...) through the accessibility tree of
/// SpringBoard and of the system service apps that host such UI.
///
/// Secrets: `type` never echoes the text back in any response or log line.
final class Driver {
    static let version = "0.23.0"

    /// Processes that can present system UI. Only ones that are running are
    /// queried (asking an app that is not running for its UI would launch it).
    private var candidateBundleIDs: [String] {
        var ids = [
            "com.apple.springboard",
            "com.apple.StoreKitUIService",
            "com.apple.AuthKitUIService",
            "com.apple.AuthenticationServicesUI",
            "com.apple.PassbookUIService",
            "com.apple.SafariViewService",
        ]
        if let extra = ProcessInfo.processInfo.environment["PROBE_DRIVER_APPS"] {
            ids += extra.split(separator: ",").map { String($0).trimmingCharacters(in: .whitespaces) }
        }
        return ids
    }

    /// Button labels `dismiss` will tap, in preference order.
    private let dismissLabels = ["Cancel", "Don’t Allow", "Don't Allow", "Not Now", "Close", "Dismiss", "Later", "No Thanks"]

    // MARK: - Model

    private struct Dialog {
        let element: XCUIElement
        let bundleID: String
        let texts: [String]
        let buttons: [XCUIElement]
        /// Set for the activity (share) sheet of the app under test; `dismiss`
        /// has no Cancel-like button to rely on there.
        var shareSheet: XCUIApplication? = nil
        var buttonLabels: [String] { buttons.map { $0.label } }
        var title: String { texts.first ?? "" }

        func matches(title wanted: String?) -> Bool {
            guard let wanted = wanted, !wanted.isEmpty else { return true }
            let w = Driver.normalized(wanted)
            return texts.contains { Driver.normalized($0).contains(w) } || Driver.normalized(element.label).contains(w)
        }
    }

    private func runningApps() -> [XCUIApplication] {
        candidateBundleIDs.compactMap { id in
            let app = XCUIApplication(bundleIdentifier: id)
            return app.state == .notRunning ? nil : app
        }
    }

    private func dialogs(appID: String? = nil) -> [Dialog] {
        var found: [Dialog] = []
        if let sheet = shareSheet(appID: appID) { found.append(sheet) }
        for app in runningApps() {
            let id = app.label.isEmpty ? "app" : app.label
            _ = id
            var containers: [XCUIElement] = []
            containers += app.alerts.allElementsBoundByIndex
            containers += app.sheets.allElementsBoundByIndex
            // A system service app (StoreKit UI, AuthKit UI, ...) is itself the
            // dialog: its whole window is the container.
            if !(app.label == "SpringBoard" || app.identifierIsSpringBoard) && containers.isEmpty,
               app.state == .runningForeground {
                containers.append(app.windows.firstMatch)
            }
            for c in containers where c.exists {
                let texts = c.staticTexts.allElementsBoundByIndex.prefix(24).map { $0.label }.filter { !$0.isEmpty }
                let buttons = c.buttons.allElementsBoundByIndex.filter { $0.exists }
                found.append(Dialog(element: c, bundleID: app.debugBundleID, texts: texts, buttons: buttons))
            }
        }
        return found
    }

    // MARK: - Activity (share) sheet

    /// Accessibility identifiers the activity sheet exposes inside the app that
    /// presented it (the sheet is a remote view, so it is part of that app's
    /// hierarchy, not SpringBoard's). Checked in this order; the first is the
    /// outermost container.
    private let shareSheetIdentifiers = [
        "ShareSheet.RemoteContainerView", "UIActivityContentView", "ActivityListView",
        "shareSheet.activity.contentView", "activityCollectionView",
    ]

    /// The share sheet shown by the app under test, if any. `appID` is the
    /// bundle id of that app; without it only system apps are inspected.
    private func shareSheet(appID: String?) -> Dialog? {
        guard let id = appID, !id.isEmpty else { return nil }
        let app = XCUIApplication(bundleIdentifier: id)
        // Asking an app that is not running for its UI would launch it.
        guard app.state == .runningForeground else { return nil }
        var container: XCUIElement?
        for ident in shareSheetIdentifiers {
            let el = app.descendants(matching: .any).matching(identifier: ident).firstMatch
            if el.exists { container = el; break }
        }
        guard let c = container else { return nil }
        // Header: the shared item's caption (e.g. the text or file name) from the sheet's top bar.
        let bar = app.navigationBars["UIActivityContentView"]
        var header: [String] = []
        if bar.exists {
            header = bar.descendants(matching: .any).allElementsBoundByIndex.prefix(12)
                .filter { $0.elementType != .image && $0.elementType != .button }
                .map { $0.label }.filter { !$0.isEmpty }
        }
        // Buttons: Close (when the OS version draws one) and every activity / action cell.
        let bars = c.buttons.allElementsBoundByIndex.filter { $0.exists && !$0.label.isEmpty }
        let cells = c.cells.allElementsBoundByIndex.filter { $0.exists && !$0.label.isEmpty }
        var seen = Set<String>()
        var buttons: [XCUIElement] = []
        for b in bars + cells where seen.insert(b.label).inserted { buttons.append(b) }
        let title = header.first ?? "Share sheet"
        var texts = [title, "Share sheet"] + header.dropFirst()
        texts += buttons.map { $0.label }
        return Dialog(element: c, bundleID: id, texts: texts, buttons: buttons, shareSheet: app)
    }

    /// Closes the share sheet: its Close button if the OS draws one, otherwise a
    /// tap on the dimmed area above it, then a swipe down. Returns how it was closed.
    private func dismissShareSheet(_ d: Dialog, app: XCUIApplication) -> String? {
        func gone() -> Bool {
            RunLoop.current.run(mode: .default, before: Date(timeIntervalSinceNow: 0.8))
            return !d.element.exists
        }
        for label in ["Close", "Cancel", "Done"] {
            if let b = d.buttons.first(where: { $0.elementType == .button && $0.label.caseInsensitiveCompare(label) == .orderedSame }) {
                b.tap()
                if gone() { return label }
            }
        }
        let frame = d.element.frame
        let top = max(frame.minY, 0)
        if top > 60 {
            let origin = app.coordinate(withNormalizedOffset: CGVector(dx: 0, dy: 0))
            origin.withOffset(CGVector(dx: frame.midX, dy: top / 2)).tap()
            if gone() { return "tap outside" }
        }
        d.element.swipeDown(velocity: .fast)
        if gone() { return "swipe down" }
        return nil
    }

    private func find(title: String?, appID: String? = nil) -> Dialog? {
        dialogs(appID: appID).first { $0.matches(title: title) }
    }

    // MARK: - Endpoints (each returns (httpStatus, json))

    func dialogs(_ body: [String: Any]) -> (Int, [String: Any]) {
        let list = dialogs(appID: body["app"] as? String).map { d -> [String: Any] in
            ["title": d.title, "texts": d.texts, "buttons": d.buttonLabels, "app": d.bundleID,
             "fields": fieldLabels(in: d.element)]
        }
        return (200, ["ok": true, "dialogs": list])
    }

    func see(_ body: [String: Any]) -> (Int, [String: Any]) {
        let title = body["title"] as? String
        let d = find(title: title, appID: body["app"] as? String)
        return (200, ["ok": true, "found": d != nil, "title": d?.title ?? "", "buttons": d?.buttonLabels ?? []])
    }

    func wait(_ body: [String: Any]) -> (Int, [String: Any]) {
        let title = body["title"] as? String
        let appear = (body["appear"] as? Bool) ?? true
        let timeout = (body["timeout"] as? Double) ?? 10
        let deadline = Date(timeIntervalSinceNow: timeout)
        repeat {
            let present = find(title: title, appID: body["app"] as? String) != nil
            if present == appear { return (200, ["ok": true, "found": present]) }
            RunLoop.current.run(mode: .default, before: Date(timeIntervalSinceNow: 0.25))
        } while Date() < deadline
        return (200, ["ok": true, "found": !appear, "timedOut": true])
    }

    func tap(_ body: [String: Any]) -> (Int, [String: Any]) {
        guard let wanted = body["button"] as? String, !wanted.isEmpty else {
            return (400, ["ok": false, "error": "missing button"])
        }
        let title = body["title"] as? String
        guard let d = find(title: title, appID: body["app"] as? String) else {
            return (200, ["ok": false, "error": "no system dialog found" + titleSuffix(title)])
        }
        guard let button = match(wanted, in: d.buttons, by: { $0.label }) else {
            return (200, ["ok": false, "error": "no button \"\(wanted)\" in the dialog", "buttons": d.buttonLabels, "title": d.title])
        }
        let label = button.label
        let signature = d.buttonLabels
        button.tap()
        if stillShowing(title: title, signature: signature, appID: body["app"] as? String) {
            // The tap was reported by XCUITest but the dialog did not go away (seen with
            // SpringBoard's "Open in <app>?" confirmation). Tap the button's centre point
            // instead of asking the element, then check again.
            if let again = find(title: title, appID: body["app"] as? String),
               let b = match(wanted, in: again.buttons, by: { $0.label }) {
                b.coordinate(withNormalizedOffset: CGVector(dx: 0.5, dy: 0.5)).tap()
            }
            if stillShowing(title: title, signature: signature, appID: body["app"] as? String) {
                return (200, ["ok": false, "error": "tapped \"\(label)\" but the dialog is still showing",
                              "buttons": signature, "title": d.title])
            }
        }
        return (200, ["ok": true, "tapped": label])
    }

    /// True when a dialog with the same buttons is still on screen after a tap (polled
    /// for up to ~1.2 s, since dialogs animate away).
    private func stillShowing(title: String?, signature: [String], appID: String?) -> Bool {
        let deadline = Date(timeIntervalSinceNow: 1.2)
        repeat {
            RunLoop.current.run(mode: .default, before: Date(timeIntervalSinceNow: 0.2))
            guard let d = find(title: title, appID: appID), d.buttonLabels == signature else { return false }
        } while Date() < deadline
        return true
    }

    func dismiss(_ body: [String: Any]) -> (Int, [String: Any]) {
        let title = body["title"] as? String
        guard let d = find(title: title, appID: body["app"] as? String) else {
            return (200, ["ok": true, "dismissed": false])   // idempotent: nothing to dismiss
        }
        if let app = d.shareSheet {
            if let how = dismissShareSheet(d, app: app) {
                return (200, ["ok": true, "dismissed": true, "tapped": how])
            }
            return (200, ["ok": false, "error": "could not close the share sheet", "buttons": d.buttonLabels, "title": d.title])
        }
        for label in dismissLabels {
            if let b = d.buttons.first(where: { Driver.normalized($0.label) == Driver.normalized(label) }) {
                let tappedLabel = b.label   // read before tapping: the element disappears with the dialog
                b.tap()
                return (200, ["ok": true, "dismissed": true, "tapped": tappedLabel])
            }
        }
        return (200, ["ok": false, "error": "no cancel-like button in the dialog", "buttons": d.buttonLabels, "title": d.title])
    }

    /// Types into a text or secure field. The response never contains the text.
    func type(_ body: [String: Any]) -> (Int, [String: Any]) {
        guard let field = body["field"] as? String, let text = body["text"] as? String else {
            return (400, ["ok": false, "error": "missing field or text"])
        }
        let title = body["title"] as? String
        guard let d = find(title: title, appID: body["app"] as? String) else {
            return (200, ["ok": false, "error": "no system dialog found" + titleSuffix(title)])
        }
        let fields = d.element.textFields.allElementsBoundByIndex + d.element.secureTextFields.allElementsBoundByIndex
        let target = match(field, in: fields, by: { fieldLabel($0) })
            ?? (fields.count == 1 ? fields.first : nil)
        guard let el = target else {
            return (200, ["ok": false, "error": "no field \"\(field)\" in the dialog", "fields": fieldLabels(in: d.element)])
        }
        el.tap()
        // Replace any existing content (the Apple Account field is prefilled
        // on some builds).
        if let current = el.value as? String, !current.isEmpty, current != fieldLabel(el),
           el.elementType != .secureTextField {
            el.typeText(String(repeating: XCUIKeyboardKey.delete.rawValue, count: current.count))
        }
        el.typeText(text)
        return (200, ["ok": true, "typed": true, "chars": text.count])
    }

    func tree(_ body: [String: Any]) -> (Int, [String: Any]) {
        var out: [String: String] = [:]
        for app in runningApps() {
            out[app.debugBundleID] = String(app.debugDescription.prefix(20_000))
        }
        if let id = body["app"] as? String, !id.isEmpty {
            let app = XCUIApplication(bundleIdentifier: id)
            if app.state != .notRunning { out[id] = String(app.debugDescription.prefix(40_000)) }
        }
        return (200, ["ok": true, "tree": out])
    }

    // MARK: - Helpers

    private func titleSuffix(_ title: String?) -> String {
        guard let t = title, !t.isEmpty else { return "" }
        return " matching \"\(t)\""
    }

    /// Case-, apostrophe- and whitespace-insensitive form of a label: iOS writes "Don’t Allow"
    /// (U+2019) where a script written for Android has "Don't allow".
    static func normalized(_ s: String) -> String {
        let folded = s.replacingOccurrences(of: "\u{2019}", with: "'")
            .replacingOccurrences(of: "\u{2018}", with: "'")
            .replacingOccurrences(of: "\u{02BC}", with: "'")
            .replacingOccurrences(of: "\u{00A0}", with: " ")
            .lowercased()
        return folded.split(whereSeparator: { $0.isWhitespace }).joined(separator: " ")
    }

    /// Exact (normalized) match first, then "contains".
    private func match(_ wanted: String, in els: [XCUIElement], by label: (XCUIElement) -> String) -> XCUIElement? {
        let w = Driver.normalized(wanted)
        return els.first { Driver.normalized(label($0)) == w } ?? els.first { Driver.normalized(label($0)).contains(w) }
    }

    private func fieldLabel(_ el: XCUIElement) -> String {
        if !el.label.isEmpty { return el.label }
        if !el.placeholderValue.orEmpty.isEmpty { return el.placeholderValue.orEmpty }
        return el.identifier
    }

    private func fieldLabels(in container: XCUIElement) -> [String] {
        (container.textFields.allElementsBoundByIndex + container.secureTextFields.allElementsBoundByIndex)
            .map { fieldLabel($0) }
    }
}

private extension Optional where Wrapped == String {
    var orEmpty: String { self ?? "" }
}

private extension XCUIApplication {
    var debugBundleID: String {
        // XCUIApplication has no public bundle-id getter; the debug description
        // carries it, which is enough for diagnostics.
        let d = debugDescription
        if let r = d.range(of: "com.apple.[A-Za-z0-9._]+", options: .regularExpression) { return String(d[r]) }
        return label
    }
    var identifierIsSpringBoard: Bool { debugBundleID == "com.apple.springboard" }
}
