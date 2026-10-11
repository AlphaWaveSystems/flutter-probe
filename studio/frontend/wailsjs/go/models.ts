export namespace main {
	
	export class ChatMessage {
	    role: string;
	    content: string;
	
	    static createFrom(source: any = {}) {
	        return new ChatMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	    }
	}
	export class ChatResponse {
	    content: string;
	    inputTokens: number;
	    outputTokens: number;
	    costUSD: number;
	
	    static createFrom(source: any = {}) {
	        return new ChatResponse(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.content = source["content"];
	        this.inputTokens = source["inputTokens"];
	        this.outputTokens = source["outputTokens"];
	        this.costUSD = source["costUSD"];
	    }
	}
	export class ConnectionStatus {
	    connected: boolean;
	    deviceId: string;
	    deviceName: string;
	    platform: string;
	
	    static createFrom(source: any = {}) {
	        return new ConnectionStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.connected = source["connected"];
	        this.deviceId = source["deviceId"];
	        this.deviceName = source["deviceName"];
	        this.platform = source["platform"];
	    }
	}
	export class DeviceInfo {
	    id: string;
	    name: string;
	    platform: string;
	    kind: string;
	    state: string;
	    osVersion: string;
	    booted: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DeviceInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.platform = source["platform"];
	        this.kind = source["kind"];
	        this.state = source["state"];
	        this.osVersion = source["osVersion"];
	        this.booted = source["booted"];
	    }
	}
	export class Diagnostic {
	    severity: number;
	    message: string;
	    startLineNumber: number;
	    startColumn: number;
	    endLineNumber: number;
	    endColumn: number;
	
	    static createFrom(source: any = {}) {
	        return new Diagnostic(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.severity = source["severity"];
	        this.message = source["message"];
	        this.startLineNumber = source["startLineNumber"];
	        this.startColumn = source["startColumn"];
	        this.endLineNumber = source["endLineNumber"];
	        this.endColumn = source["endColumn"];
	    }
	}
	export class FileEntry {
	    name: string;
	    path: string;
	    isDir: boolean;
	
	    static createFrom(source: any = {}) {
	        return new FileEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.isDir = source["isDir"];
	    }
	}
	export class RunPlanTest {
	    name: string;
	    line: number;
	    steps: number;
	
	    static createFrom(source: any = {}) {
	        return new RunPlanTest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.line = source["line"];
	        this.steps = source["steps"];
	    }
	}
	export class RunPlan {
	    tests: RunPlanTest[];
	
	    static createFrom(source: any = {}) {
	        return new RunPlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tests = this.convertValues(source["tests"], RunPlanTest);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class RunResult {
	    name: string;
	    file: string;
	    passed: boolean;
	    skipped: boolean;
	    durationMs: number;
	    error?: string;
	    perf?: perf.Metrics[];
	
	    static createFrom(source: any = {}) {
	        return new RunResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.file = source["file"];
	        this.passed = source["passed"];
	        this.skipped = source["skipped"];
	        this.durationMs = source["durationMs"];
	        this.error = source["error"];
	        this.perf = this.convertValues(source["perf"], perf.Metrics);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RunStep {
	    file: string;
	    test: string;
	    line: number;
	    description: string;
	    status: string;
	    durationMs: number;
	    error?: string;
	    attempt: number;
	    depth: number;
	
	    static createFrom(source: any = {}) {
	        return new RunStep(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.file = source["file"];
	        this.test = source["test"];
	        this.line = source["line"];
	        this.description = source["description"];
	        this.status = source["status"];
	        this.durationMs = source["durationMs"];
	        this.error = source["error"];
	        this.attempt = source["attempt"];
	        this.depth = source["depth"];
	    }
	}
	export class RunState {
	    running: boolean;
	    file?: string;
	    plan: RunPlan;
	    current?: RunStep;
	    steps: RunStep[];
	    results: RunResult[];
	    finished: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new RunState(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.file = source["file"];
	        this.plan = this.convertValues(source["plan"], RunPlan);
	        this.current = this.convertValues(source["current"], RunStep);
	        this.steps = this.convertValues(source["steps"], RunStep);
	        this.results = this.convertValues(source["results"], RunResult);
	        this.finished = source["finished"];
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class WorkspaceSettings {
	    agentPort: number;
	    defaultsTimeout: string;
	    iosDeviceId: string;
	    androidDeviceId: string;
	
	    static createFrom(source: any = {}) {
	        return new WorkspaceSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.agentPort = source["agentPort"];
	        this.defaultsTimeout = source["defaultsTimeout"];
	        this.iosDeviceId = source["iosDeviceId"];
	        this.androidDeviceId = source["androidDeviceId"];
	    }
	}

}

export namespace perf {
	
	export class Metrics {
	    name: string;
	    duration_ms: number;
	    cpu_available: boolean;
	    cpu_avg_pct: number;
	    cpu_peak_pct: number;
	    mem_start_mb: number;
	    mem_peak_mb: number;
	    mem_end_mb: number;
	    mem_growth_mb: number;
	    frames: number;
	    frame_budget_ms?: number;
	    slow_frame_pct: number;
	    frame_p95_ms: number;
	    frame_max_ms: number;
	    build_avg_ms: number;
	    raster_avg_ms: number;
	    window_spans_restart?: boolean;
	    requests: number;
	    data_kb: number;
	    slowest_request_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new Metrics(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.duration_ms = source["duration_ms"];
	        this.cpu_available = source["cpu_available"];
	        this.cpu_avg_pct = source["cpu_avg_pct"];
	        this.cpu_peak_pct = source["cpu_peak_pct"];
	        this.mem_start_mb = source["mem_start_mb"];
	        this.mem_peak_mb = source["mem_peak_mb"];
	        this.mem_end_mb = source["mem_end_mb"];
	        this.mem_growth_mb = source["mem_growth_mb"];
	        this.frames = source["frames"];
	        this.frame_budget_ms = source["frame_budget_ms"];
	        this.slow_frame_pct = source["slow_frame_pct"];
	        this.frame_p95_ms = source["frame_p95_ms"];
	        this.frame_max_ms = source["frame_max_ms"];
	        this.build_avg_ms = source["build_avg_ms"];
	        this.raster_avg_ms = source["raster_avg_ms"];
	        this.window_spans_restart = source["window_spans_restart"];
	        this.requests = source["requests"];
	        this.data_kb = source["data_kb"];
	        this.slowest_request_ms = source["slowest_request_ms"];
	    }
	}

}

