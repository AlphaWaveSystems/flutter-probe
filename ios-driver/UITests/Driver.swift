import XCTest

/// Finds and drives iOS *system* dialogs (permission alerts, the StoreKit
/// "Sign in to Apple Account" sheet, ...) through the accessibility tree of
/// SpringBoard and of the system service apps that host such UI.
///
/// Secrets: `type` never echoes the text back in any response or log line.
final class Driver {
    static let version = "0.16.4"

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
        var buttonLabels: [String] { buttons.map { $0.label } }
        var title: String { texts.first ?? "" }

        func matches(title wanted: String?) -> Bool {
            guard let wanted = wanted, !wanted.isEmpty else { return true }
            let w = wanted.lowercased()
            return texts.contains { $0.lowercased().contains(w) } || element.label.lowercased().contains(w)
        }
    }

    private func runningApps() -> [XCUIApplication] {
        candidateBundleIDs.compactMap { id in
            let app = XCUIApplication(bundleIdentifier: id)
            return app.state == .notRunning ? nil : app
        }
    }

    private func dialogs() -> [Dialog] {
        var found: [Dialog] = []
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

    private func find(title: String?) -> Dialog? {
        dialogs().first { $0.matches(title: title) }
    }

    // MARK: - Endpoints (each returns (httpStatus, json))

    func dialogs(_ body: [String: Any]) -> (Int, [String: Any]) {
        let list = dialogs().map { d -> [String: Any] in
            ["title": d.title, "texts": d.texts, "buttons": d.buttonLabels, "app": d.bundleID,
             "fields": fieldLabels(in: d.element)]
        }
        return (200, ["ok": true, "dialogs": list])
    }

    func see(_ body: [String: Any]) -> (Int, [String: Any]) {
        let title = body["title"] as? String
        let d = find(title: title)
        return (200, ["ok": true, "found": d != nil, "title": d?.title ?? "", "buttons": d?.buttonLabels ?? []])
    }

    func wait(_ body: [String: Any]) -> (Int, [String: Any]) {
        let title = body["title"] as? String
        let appear = (body["appear"] as? Bool) ?? true
        let timeout = (body["timeout"] as? Double) ?? 10
        let deadline = Date(timeIntervalSinceNow: timeout)
        repeat {
            let present = find(title: title) != nil
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
        guard let d = find(title: title) else {
            return (200, ["ok": false, "error": "no system dialog found" + titleSuffix(title)])
        }
        guard let button = match(wanted, in: d.buttons, by: { $0.label }) else {
            return (200, ["ok": false, "error": "no button \"\(wanted)\" in the dialog", "buttons": d.buttonLabels, "title": d.title])
        }
        let label = button.label
        button.tap()
        return (200, ["ok": true, "tapped": label])
    }

    func dismiss(_ body: [String: Any]) -> (Int, [String: Any]) {
        let title = body["title"] as? String
        guard let d = find(title: title) else {
            return (200, ["ok": true, "dismissed": false])   // idempotent: nothing to dismiss
        }
        for label in dismissLabels {
            if let b = d.buttons.first(where: { $0.label.caseInsensitiveCompare(label) == .orderedSame }) {
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
        guard let d = find(title: title) else {
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
        return (200, ["ok": true, "tree": out])
    }

    // MARK: - Helpers

    private func titleSuffix(_ title: String?) -> String {
        guard let t = title, !t.isEmpty else { return "" }
        return " matching \"\(t)\""
    }

    /// Exact (case-insensitive) match first, then "contains".
    private func match(_ wanted: String, in els: [XCUIElement], by label: (XCUIElement) -> String) -> XCUIElement? {
        let w = wanted.lowercased()
        return els.first { label($0).lowercased() == w } ?? els.first { label($0).lowercased().contains(w) }
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
