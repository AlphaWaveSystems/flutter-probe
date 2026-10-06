import UIKit
import UserNotifications

// Minimal host app for the FlutterProbe iOS driver. XCUITest bundles need a
// host application; this one does nothing except, for the driver's own
// self-tests, optionally trigger a system permission alert on launch:
//   -probeRequestNotifications   asks for notification permission
@main
final class AppDelegate: UIResponder, UIApplicationDelegate {
    var window: UIWindow?

    func application(_ application: UIApplication,
                     didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil) -> Bool {
        let window = UIWindow(frame: UIScreen.main.bounds)
        let vc = UIViewController()
        let label = UILabel()
        label.text = "FlutterProbe iOS driver host"
        label.textAlignment = .center
        label.frame = vc.view.bounds
        label.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        vc.view.backgroundColor = .systemBackground
        vc.view.addSubview(label)
        window.rootViewController = vc
        window.makeKeyAndVisible()
        self.window = window

        if CommandLine.arguments.contains("-probeRequestNotifications") {
            UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound]) { _, _ in }
        }
        // Self-test only: an alert shaped like the StoreKit sandbox sign-in sheet
        // (an account field, a secure password field, Cancel/OK). On OK it records
        // what was typed in the app's tmp directory so tests can verify typing,
        // including into the secure field. Never shown unless asked for.
        if CommandLine.arguments.contains("-probeShowSignInAlert") {
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.5) { [weak self] in self?.showSignInAlert() }
        }
        return true
    }

    private func showSignInAlert() {
        let alert = UIAlertController(title: "Sign in to Apple Account", message: "Self-test sheet", preferredStyle: .alert)
        alert.addTextField { $0.placeholder = "Apple Account"; $0.autocapitalizationType = .none }
        alert.addTextField { $0.placeholder = "Password"; $0.isSecureTextEntry = true }
        alert.addAction(UIAlertAction(title: "Cancel", style: .cancel))
        alert.addAction(UIAlertAction(title: "OK", style: .default) { [weak alert] _ in
            let user = alert?.textFields?[0].text ?? ""
            let pass = alert?.textFields?[1].text ?? ""
            let path = NSTemporaryDirectory() + "probe_signin.txt"
            try? "\(user)|\(pass)".write(toFile: path, atomically: true, encoding: .utf8)
        })
        window?.rootViewController?.present(alert, animated: true)
    }
}
