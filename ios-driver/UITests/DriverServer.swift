import Foundation
import Network

/// A deliberately tiny HTTP/1.1 server (loopback only): one request per
/// connection, JSON in, JSON out. Handlers run on the main queue because
/// XCUITest must be driven from the main thread.
final class DriverServer {
    private let port: UInt16
    private let driver: Driver
    private var listener: NWListener?
    private(set) var shouldStop = false

    init(port: UInt16, driver: Driver) {
        self.port = port
        self.driver = driver
    }

    func start() throws {
        let params = NWParameters.tcp
        // Loopback only: this server can tap and type on the simulator, and the
        // CLI sends secrets to it.
        params.requiredInterfaceType = .loopback
        let listener = try NWListener(using: params, on: NWEndpoint.Port(rawValue: port)!)
        listener.newConnectionHandler = { [weak self] conn in self?.accept(conn) }
        listener.start(queue: .global(qos: .userInitiated))
        self.listener = listener
    }

    func stop() {
        listener?.cancel()
        listener = nil
    }

    private func accept(_ conn: NWConnection) {
        conn.start(queue: .global(qos: .userInitiated))
        receive(conn, buffer: Data())
    }

    private func receive(_ conn: NWConnection, buffer: Data) {
        conn.receive(minimumIncompleteLength: 1, maximumLength: 64 * 1024) { [weak self] data, _, isComplete, error in
            guard let self = self else { return }
            var buf = buffer
            if let data = data { buf.append(data) }
            if let request = HTTPRequest.parse(buf) {
                DispatchQueue.main.async {
                    let response = self.route(request)
                    self.send(conn, response)
                }
            } else if isComplete || error != nil {
                conn.cancel()
            } else {
                self.receive(conn, buffer: buf)
            }
        }
    }

    private func route(_ req: HTTPRequest) -> (Int, [String: Any]) {
        var body: [String: Any] = [:]
        if !req.body.isEmpty,
           let obj = try? JSONSerialization.jsonObject(with: req.body) as? [String: Any] {
            body = obj
        }
        switch (req.method, req.path) {
        case ("GET", "/health"):
            return (200, ["ok": true, "driver": "flutter-probe-ios-driver", "version": Driver.version])
        case ("POST", "/shutdown"):
            shouldStop = true
            return (200, ["ok": true])
        case ("POST", "/see"):
            return driver.see(body)
        case ("POST", "/wait"):
            return driver.wait(body)
        case ("POST", "/tap"):
            return driver.tap(body)
        case ("POST", "/type"):
            return driver.type(body)
        case ("POST", "/dismiss"):
            return driver.dismiss(body)
        case ("POST", "/dialogs"):
            return driver.dialogs(body)
        case ("POST", "/tree"):
            return driver.tree(body)
        default:
            return (404, ["ok": false, "error": "unknown route \(req.method) \(req.path)"])
        }
    }

    private func send(_ conn: NWConnection, _ response: (Int, [String: Any])) {
        let (status, obj) = response
        let payload = (try? JSONSerialization.data(withJSONObject: obj)) ?? Data("{}".utf8)
        let head = "HTTP/1.1 \(status) \(status == 200 ? "OK" : "Error")\r\n" +
            "Content-Type: application/json\r\nContent-Length: \(payload.count)\r\nConnection: close\r\n\r\n"
        var out = Data(head.utf8)
        out.append(payload)
        conn.send(content: out, completion: .contentProcessed { _ in conn.cancel() })
    }
}

struct HTTPRequest {
    let method: String
    let path: String
    let body: Data

    /// Returns nil until a complete request (headers plus Content-Length body) is buffered.
    static func parse(_ data: Data) -> HTTPRequest? {
        guard let range = data.range(of: Data("\r\n\r\n".utf8)) else { return nil }
        let headData = data.subdata(in: data.startIndex..<range.lowerBound)
        guard let head = String(data: headData, encoding: .utf8) else { return nil }
        let lines = head.components(separatedBy: "\r\n")
        let first = lines.first?.split(separator: " ") ?? []
        guard first.count >= 2 else { return nil }
        var length = 0
        for line in lines.dropFirst() {
            let parts = line.split(separator: ":", maxSplits: 1)
            if parts.count == 2, parts[0].lowercased() == "content-length" {
                length = Int(parts[1].trimmingCharacters(in: .whitespaces)) ?? 0
            }
        }
        let bodyStart = range.upperBound
        guard data.count - (bodyStart - data.startIndex) >= length else { return nil }
        let body = data.subdata(in: bodyStart..<(bodyStart + length))
        let path = String(first[1]).components(separatedBy: "?").first ?? String(first[1])
        return HTTPRequest(method: String(first[0]), path: path, body: body)
    }
}
