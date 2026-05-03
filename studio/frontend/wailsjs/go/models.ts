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
	export class RunResult {
	    name: string;
	    file: string;
	    passed: boolean;
	    skipped: boolean;
	    durationMs: number;
	    error?: string;
	
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

