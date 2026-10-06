import XCTest

/// The FlutterProbe iOS driver. `testServe` never "tests" anything: it starts a
/// small HTTP server inside the XCUITest runner process (the only process that is
/// allowed to drive other apps' accessibility trees, SpringBoard included) and
/// then keeps the test alive until the CLI asks it to shut down.
///
/// The CLI starts it with:
///   xcodebuild test-without-building -xctestrun <file> -destination id=<udid>
///     -only-testing:ProbeDriverUITests/ProbeDriverTests/testServe
/// and passes the port as TEST_RUNNER_PROBE_DRIVER_PORT.
final class ProbeDriverTests: XCTestCase {
    func testServe() throws {
        let env = ProcessInfo.processInfo.environment
        let port = UInt16(env["PROBE_DRIVER_PORT"] ?? "") ?? 48790
        // Safety net: if the CLI dies without sending /shutdown, don't leave a
        // runner (and xcodebuild) hanging around on the simulator forever.
        let maxSeconds = Double(env["PROBE_DRIVER_MAX_SECONDS"] ?? "") ?? 3 * 60 * 60

        let server = DriverServer(port: port, driver: Driver())
        try server.start()
        let deadline = Date(timeIntervalSinceNow: maxSeconds)
        // The XCUITest APIs must be used from the main thread, so the server
        // dispatches handlers onto it and we just keep its run loop turning.
        while !server.shouldStop && Date() < deadline {
            RunLoop.current.run(mode: .default, before: Date(timeIntervalSinceNow: 0.05))
        }
        server.stop()
    }
}
