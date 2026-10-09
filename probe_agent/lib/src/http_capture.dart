import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:math' as math;

/// One request/response pair seen by the app's `dart:io` HttpClient.
class ProbeHttpEntry {
  ProbeHttpEntry({
    required this.seq,
    required this.method,
    required this.url,
    required this.startedAt,
  });

  final int seq;
  final String method;
  final String url;
  final DateTime startedAt;
  final Map<String, String> requestHeaders = {};
  final List<int> _reqBody = [];
  int? status;
  final Map<String, String> responseHeaders = {};
  final List<int> _respBody = [];
  int respBytes = 0;
  int reqBytes = 0;
  int? durationMs;
  String? error;
  bool mocked = false;
  bool done = false;

  Map<String, dynamic> toJson({bool bodies = true}) => {
        'seq': seq,
        'method': method,
        'url': url,
        'status': status,
        'durationMs': durationMs,
        'mocked': mocked,
        if (error != null) 'error': error,
        'requestHeaders': requestHeaders,
        'responseHeaders': responseHeaders,
        if (bodies) 'requestBody': utf8.decode(_reqBody, allowMalformed: true),
        if (bodies) 'responseBody': utf8.decode(_respBody, allowMalformed: true),
        'requestBytes': reqBytes,
        'responseBytes': respBytes,
      };
}

/// A registered mock: requests matching [method] + [pattern] get this answer
/// instead of reaching the network.
class ProbeMock {
  ProbeMock({
    required this.method,
    required this.pattern,
    this.status = 200,
    this.body = '',
    this.headers = const {},
    this.delayMs = 0,
    this.fail = false,
  });

  final String method; // "" = any
  final String pattern;
  final int status;
  final String body;
  final Map<String, String> headers;
  final int delayMs;
  final bool fail;
}

/// Matches a request against `GET "/api/orders"` style references.
///
/// [pattern] with "://" compares the whole URL (without query unless the
/// pattern has a "?"); otherwise it compares the path. `*` matches any run of
/// characters. The whole path must match: "/api/orders" does not match
/// "/api/orders/42" (use "/api/orders/*").
bool probeUrlMatches(String pattern, Uri uri) {
  var subject = uri.path;
  if (pattern.contains('://')) {
    subject = '${uri.scheme}://${uri.authority}${uri.path}';
    if (pattern.contains('?') && uri.hasQuery) subject += '?${uri.query}';
  } else if (pattern.contains('?')) {
    subject = uri.path + (uri.hasQuery ? '?${uri.query}' : '');
  }
  if (pattern.isEmpty) return true;
  final re = RegExp('^${pattern.split('*').map(RegExp.escape).join('.*')}\$');
  return re.hasMatch(subject);
}

/// The process-wide recorder and mock registry behind [ProbeHttpOverrides].
class ProbeHttpLog {
  ProbeHttpLog._();
  static final ProbeHttpLog instance = ProbeHttpLog._();

  /// Changes whenever the agent process restarts, so the CLI can tell that
  /// sequence numbers started over.
  final String epoch = _randomId();

  static const int maxEntries = 300;
  static const int maxBodyBytes = 64 * 1024;

  final List<ProbeHttpEntry> _entries = [];
  final List<ProbeMock> _mocks = [];
  final List<Completer<void>> _waiters = [];
  int _seq = 0;
  HttpServer? _mockServer;
  final Map<String, ProbeMock> _mockById = {};
  final Map<ProbeMock, String> _mockIds = {};

  bool enabled = const bool.fromEnvironment('PROBE_HTTP_CAPTURE', defaultValue: true);

  static String _randomId() {
    final r = math.Random.secure();
    return List.generate(8, (_) => r.nextInt(36).toRadixString(36)).join();
  }

  ProbeHttpEntry begin(String method, Uri uri) {
    final e = ProbeHttpEntry(seq: ++_seq, method: method.toUpperCase(), url: uri.toString(), startedAt: DateTime.now());
    _entries.add(e);
    if (_entries.length > maxEntries) _entries.removeAt(0);
    return e;
  }

  void finish(ProbeHttpEntry e) {
    e.done = true;
    e.durationMs = DateTime.now().difference(e.startedAt).inMilliseconds;
    final waiters = List<Completer<void>>.from(_waiters);
    _waiters.clear();
    for (final w in waiters) {
      if (!w.isCompleted) w.complete();
    }
  }

  /// Completes the next time any exchange finishes (used to wait without polling).
  Future<void> nextFinish(Duration timeout) {
    final c = Completer<void>();
    _waiters.add(c);
    return c.future.timeout(timeout, onTimeout: () {});
  }

  void clear({bool mocks = false}) {
    _entries.clear();
    if (mocks) _mocks.clear();
  }

  /// Entries with seq > [since] matching [method] / [pattern] that have finished
  /// (a request still in flight has no response to look at).
  List<ProbeHttpEntry> query({String method = '', String pattern = '', int since = 0, bool includeInFlight = false}) {
    return _entries.where((e) {
      if (e.seq <= since) return false;
      if (!includeInFlight && !e.done) return false;
      if (method.isNotEmpty && e.method != method.toUpperCase()) return false;
      return probeUrlMatches(pattern, Uri.parse(e.url));
    }).toList();
  }

  int get latest => _seq;

  // ---- mocks ----

  void addMock(ProbeMock m) {
    _mocks.removeWhere((x) => x.method == m.method && x.pattern == m.pattern);
    _mocks.add(m);
  }

  ProbeMock? mockFor(String method, Uri uri) {
    for (final m in _mocks.reversed) {
      if (m.method.isNotEmpty && m.method != method.toUpperCase()) continue;
      if (probeUrlMatches(m.pattern, uri)) return m;
    }
    return null;
  }

  /// A loopback server that answers mocked requests with real HTTP, so the
  /// app's client code sees genuine responses, headers and failures.
  Future<Uri> mockUriFor(ProbeMock m) async {
    _mockServer ??= await _startMockServer();
    final id = _mockIds.putIfAbsent(m, () => '${_mockIds.length}');
    _mockById[id] = m;
    return Uri.parse('http://127.0.0.1:${_mockServer!.port}/$id');
  }

  Future<HttpServer> _startMockServer() async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    server.listen((req) async {
      final id = req.uri.pathSegments.isNotEmpty ? req.uri.pathSegments.first : '';
      final m = _mockById[id];
      await req.drain<void>();
      if (m == null) {
        req.response.statusCode = 404;
        await req.response.close();
        return;
      }
      if (m.delayMs > 0) await Future<void>.delayed(Duration(milliseconds: m.delayMs));
      if (m.fail) {
        final socket = await req.response.detachSocket(writeHeaders: false);
        socket.destroy();
        return;
      }
      req.response.statusCode = m.status;
      m.headers.forEach(req.response.headers.set);
      if (m.body.isNotEmpty && m.headers.keys.every((k) => k.toLowerCase() != 'content-type')) {
        req.response.headers.contentType = ContentType.json;
      }
      req.response.write(m.body);
      await req.response.close();
    });
    return server;
  }
}

// ---------------------------------------------------------------------------

const _redacted = {'authorization', 'proxy-authorization', 'cookie', 'set-cookie', 'x-api-key'};

Map<String, String> _flatten(HttpHeaders h) {
  final out = <String, String>{};
  h.forEach((name, values) {
    out[name] = _redacted.contains(name.toLowerCase()) ? '<redacted>' : values.join(', ');
  });
  return out;
}

/// Installs request capture and mocking for every `HttpClient` created after
/// [install] (so call it before the app creates its clients: `ProbeAgent.start()`
/// does). Native SDK traffic, WebViews and `dart:html` are not seen.
class ProbeHttpOverrides extends HttpOverrides {
  ProbeHttpOverrides._(this._previous);

  final HttpOverrides? _previous;

  static bool _installed = false;

  static void install() {
    if (_installed || !ProbeHttpLog.instance.enabled) return;
    _installed = true;
    HttpOverrides.global = ProbeHttpOverrides._(HttpOverrides.current);
  }

  @override
  HttpClient createHttpClient(SecurityContext? context) {
    final inner = _previous != null ? _previous.createHttpClient(context) : super.createHttpClient(context);
    return _ProbeHttpClient(inner);
  }

  @override
  String findProxyFromEnvironment(Uri url, Map<String, String>? environment) =>
      _previous != null ? _previous.findProxyFromEnvironment(url, environment) : super.findProxyFromEnvironment(url, environment);
}

class _ProbeHttpClient implements HttpClient {
  _ProbeHttpClient(this._c);
  final HttpClient _c;

  @override
  bool get autoUncompress => _c.autoUncompress;
  @override
  set autoUncompress(bool v) => _c.autoUncompress = v;
  @override
  Duration? get connectionTimeout => _c.connectionTimeout;
  @override
  set connectionTimeout(Duration? v) => _c.connectionTimeout = v;
  @override
  Duration get idleTimeout => _c.idleTimeout;
  @override
  set idleTimeout(Duration v) => _c.idleTimeout = v;
  @override
  int? get maxConnectionsPerHost => _c.maxConnectionsPerHost;
  @override
  set maxConnectionsPerHost(int? v) => _c.maxConnectionsPerHost = v;
  @override
  String? get userAgent => _c.userAgent;
  @override
  set userAgent(String? v) => _c.userAgent = v;

  @override
  void addCredentials(Uri url, String realm, HttpClientCredentials credentials) => _c.addCredentials(url, realm, credentials);
  @override
  void addProxyCredentials(String host, int port, String realm, HttpClientCredentials credentials) =>
      _c.addProxyCredentials(host, port, realm, credentials);
  @override
  set authenticate(Future<bool> Function(Uri url, String scheme, String? realm)? f) => _c.authenticate = f;
  @override
  set authenticateProxy(Future<bool> Function(String host, int port, String scheme, String? realm)? f) => _c.authenticateProxy = f;
  @override
  set badCertificateCallback(bool Function(X509Certificate cert, String host, int port)? cb) => _c.badCertificateCallback = cb;
  @override
  set connectionFactory(Future<ConnectionTask<Socket>> Function(Uri url, String? proxyHost, int? proxyPort)? f) =>
      _c.connectionFactory = f;
  @override
  set findProxy(String Function(Uri url)? f) => _c.findProxy = f;
  @override
  set keyLog(Function(String line)? cb) => _c.keyLog = cb;
  @override
  void close({bool force = false}) => _c.close(force: force);

  @override
  Future<HttpClientRequest> open(String method, String host, int port, String path) {
    final String query;
    final int hashMark = path.indexOf('#');
    var p = hashMark >= 0 ? path.substring(0, hashMark) : path;
    final int queryMark = p.indexOf('?');
    if (queryMark >= 0) {
      query = p.substring(queryMark + 1);
      p = p.substring(0, queryMark);
    } else {
      query = '';
    }
    return openUrl(method, Uri(scheme: 'http', host: host, port: port, path: p, query: query.isEmpty ? null : query));
  }

  @override
  Future<HttpClientRequest> get(String host, int port, String path) => open('get', host, port, path);
  @override
  Future<HttpClientRequest> post(String host, int port, String path) => open('post', host, port, path);
  @override
  Future<HttpClientRequest> put(String host, int port, String path) => open('put', host, port, path);
  @override
  Future<HttpClientRequest> delete(String host, int port, String path) => open('delete', host, port, path);
  @override
  Future<HttpClientRequest> patch(String host, int port, String path) => open('patch', host, port, path);
  @override
  Future<HttpClientRequest> head(String host, int port, String path) => open('head', host, port, path);
  @override
  Future<HttpClientRequest> getUrl(Uri url) => openUrl('get', url);
  @override
  Future<HttpClientRequest> postUrl(Uri url) => openUrl('post', url);
  @override
  Future<HttpClientRequest> putUrl(Uri url) => openUrl('put', url);
  @override
  Future<HttpClientRequest> deleteUrl(Uri url) => openUrl('delete', url);
  @override
  Future<HttpClientRequest> patchUrl(Uri url) => openUrl('patch', url);
  @override
  Future<HttpClientRequest> headUrl(Uri url) => openUrl('head', url);

  @override
  Future<HttpClientRequest> openUrl(String method, Uri url) async {
    final log = ProbeHttpLog.instance;
    final entry = log.begin(method, url);
    final mock = log.mockFor(method, url);
    Uri target = url;
    if (mock != null) {
      entry.mocked = true;
      target = await log.mockUriFor(mock);
    }
    try {
      final req = await _c.openUrl(method, target);
      return _ProbeRequest(req, entry);
    } catch (e) {
      entry.error = e.toString();
      log.finish(entry);
      rethrow;
    }
  }
}

class _ProbeRequest implements HttpClientRequest {
  _ProbeRequest(this._r, this._e);
  final HttpClientRequest _r;
  final ProbeHttpEntry _e;

  void _tee(List<int> data) {
    _e.reqBytes += data.length;
    final room = ProbeHttpLog.maxBodyBytes - _e._reqBody.length;
    if (room > 0) _e._reqBody.addAll(data.length <= room ? data : data.sublist(0, room));
  }

  @override
  Future<HttpClientResponse> close() async {
    _e.requestHeaders.addAll(_flatten(_r.headers));
    try {
      final resp = await _r.close();
      _e.status = resp.statusCode;
      _e.responseHeaders.addAll(_flatten(resp.headers));
      // A protocol upgrade (WebSocket) detaches the socket: hand it through untouched.
      if (_r.headers.value(HttpHeaders.upgradeHeader) != null || resp.statusCode == HttpStatus.switchingProtocols) {
        ProbeHttpLog.instance.finish(_e);
        return resp;
      }
      return _ProbeResponse(resp, _e);
    } catch (err) {
      _e.error = err.toString();
      ProbeHttpLog.instance.finish(_e);
      rethrow;
    }
  }

  @override
  void add(List<int> data) {
    _tee(data);
    _r.add(data);
  }

  @override
  Future<dynamic> addStream(Stream<List<int>> stream) => _r.addStream(stream.map((d) {
        _tee(d);
        return d;
      }));

  @override
  void write(Object? object) {
    _tee(utf8.encode('$object'));
    _r.write(object);
  }

  @override
  void writeAll(Iterable<Object?> objects, [String separator = '']) {
    _tee(utf8.encode(objects.join(separator)));
    _r.writeAll(objects, separator);
  }

  @override
  void writeCharCode(int charCode) {
    _tee(utf8.encode(String.fromCharCode(charCode)));
    _r.writeCharCode(charCode);
  }

  @override
  void writeln([Object? object = '']) {
    _tee(utf8.encode('$object\n'));
    _r.writeln(object);
  }

  @override
  bool get bufferOutput => _r.bufferOutput;
  @override
  set bufferOutput(bool v) => _r.bufferOutput = v;
  @override
  int get contentLength => _r.contentLength;
  @override
  set contentLength(int v) => _r.contentLength = v;
  @override
  Encoding get encoding => _r.encoding;
  @override
  set encoding(Encoding v) => _r.encoding = v;
  @override
  bool get followRedirects => _r.followRedirects;
  @override
  set followRedirects(bool v) => _r.followRedirects = v;
  @override
  int get maxRedirects => _r.maxRedirects;
  @override
  set maxRedirects(int v) => _r.maxRedirects = v;
  @override
  bool get persistentConnection => _r.persistentConnection;
  @override
  set persistentConnection(bool v) => _r.persistentConnection = v;
  @override
  HttpConnectionInfo? get connectionInfo => _r.connectionInfo;
  @override
  List<Cookie> get cookies => _r.cookies;
  @override
  Future<HttpClientResponse> get done => _r.done;
  @override
  HttpHeaders get headers => _r.headers;
  @override
  String get method => _r.method;
  @override
  Uri get uri => _r.uri;
  @override
  void abort([Object? exception, StackTrace? stackTrace]) => _r.abort(exception, stackTrace);
  @override
  void addError(Object error, [StackTrace? stackTrace]) => _r.addError(error, stackTrace);
  @override
  Future<dynamic> flush() => _r.flush();
}

class _ProbeResponse extends Stream<List<int>> implements HttpClientResponse {
  _ProbeResponse(this._r, this._e) {
    // A caller that never reads the body (HEAD, fire-and-forget) would leave the
    // exchange "in flight" forever: count it as finished once the headers are in.
    Timer(const Duration(seconds: 2), () {
      if (!_e.done) ProbeHttpLog.instance.finish(_e);
    });
  }
  final HttpClientResponse _r;
  final ProbeHttpEntry _e;

  late final Stream<List<int>> _tapped = _r.transform(StreamTransformer<List<int>, List<int>>.fromHandlers(
    handleData: (data, sink) {
      _e.respBytes += data.length;
      final room = ProbeHttpLog.maxBodyBytes - _e._respBody.length;
      if (room > 0) _e._respBody.addAll(data.length <= room ? data : data.sublist(0, room));
      sink.add(data);
    },
    handleError: (err, st, sink) {
      _e.error = err.toString();
      ProbeHttpLog.instance.finish(_e);
      sink.addError(err, st);
    },
    handleDone: (sink) {
      ProbeHttpLog.instance.finish(_e);
      sink.close();
    },
  ));

  @override
  StreamSubscription<List<int>> listen(void Function(List<int> event)? onData,
          {Function? onError, void Function()? onDone, bool? cancelOnError}) =>
      _tapped.listen(onData, onError: onError, onDone: onDone, cancelOnError: cancelOnError);

  @override
  X509Certificate? get certificate => _r.certificate;
  @override
  HttpClientResponseCompressionState get compressionState => _r.compressionState;
  @override
  HttpConnectionInfo? get connectionInfo => _r.connectionInfo;
  @override
  int get contentLength => _r.contentLength;
  @override
  List<Cookie> get cookies => _r.cookies;
  @override
  Future<Socket> detachSocket() => _r.detachSocket();
  @override
  HttpHeaders get headers => _r.headers;
  @override
  bool get isRedirect => _r.isRedirect;
  @override
  bool get persistentConnection => _r.persistentConnection;
  @override
  String get reasonPhrase => _r.reasonPhrase;
  @override
  Future<HttpClientResponse> redirect([String? method, Uri? url, bool? followLoops]) => _r.redirect(method, url, followLoops);
  @override
  List<RedirectInfo> get redirects => _r.redirects;
  @override
  int get statusCode => _r.statusCode;
}
