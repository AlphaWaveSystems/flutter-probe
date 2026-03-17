import * as vscode from 'vscode';
import * as fs from 'fs';
import * as path from 'path';
import { Session, Device } from '../types';
import { listDevices, buildTestArgs, getProbeCommand } from '../probe';
import { runInTerminal } from '../terminal/probeTerminal';
import { getWorkspaceRoot } from '../config';

export class SessionManagerProvider implements vscode.TreeDataProvider<SessionTreeItem> {
  private _onDidChangeTreeData = new vscode.EventEmitter<SessionTreeItem | undefined>();
  readonly onDidChangeTreeData = this._onDidChangeTreeData.event;

  private sessions: Session[] = [];
  private storePath: string | undefined;

  constructor(context: vscode.ExtensionContext) {
    const ws = getWorkspaceRoot();
    if (ws) {
      this.storePath = path.join(ws, '.vscode', 'flutterprobe-sessions.json');
      this.loadSessions();
    }

    context.subscriptions.push(
      vscode.commands.registerCommand('flutterprobe.addSession', () => this.addSession()),
      vscode.commands.registerCommand('flutterprobe.removeSession', (item: SessionTreeItem) => this.removeSession(item)),
      vscode.commands.registerCommand('flutterprobe.editSession', (item: SessionTreeItem) => this.editSession(item)),
      vscode.commands.registerCommand('flutterprobe.runOnSession', (item: SessionTreeItem) => this.runOnSession(item)),
      vscode.commands.registerCommand('flutterprobe.runOnAllSessions', () => this.runOnAllSessions()),
      vscode.commands.registerCommand('flutterprobe.refreshSessions', () => this.refresh()),
      vscode.commands.registerCommand('flutterprobe.runFileOnSession', (item: SessionTreeItem) => this.runFileOnSession(item)),
    );
  }

  refresh(): void {
    this._onDidChangeTreeData.fire(undefined);
  }

  getTreeItem(element: SessionTreeItem): vscode.TreeItem {
    return element;
  }

  getChildren(element?: SessionTreeItem): SessionTreeItem[] {
    if (!element) {
      const items = this.sessions.map(s => this.sessionToTreeItem(s));
      items.push(new SessionTreeItem(
        '+ Add Session...',
        vscode.TreeItemCollapsibleState.None,
        { command: 'flutterprobe.addSession', title: 'Add Session' }
      ));
      return items;
    }

    if (element.session) {
      return this.sessionDetails(element.session);
    }

    return [];
  }

  private sessionToTreeItem(session: Session): SessionTreeItem {
    const statusIcon = session.status === 'connected' ? '●' :
                       session.status === 'running' ? '▶' :
                       session.status === 'connecting' ? '◌' : '○';
    const label = `${session.device.platform === 'ios' ? '📱' : '📱'} ${session.name} ${statusIcon}`;

    const item = new SessionTreeItem(label, vscode.TreeItemCollapsibleState.Collapsed);
    item.session = session;
    item.contextValue = 'session';
    item.tooltip = `${session.device.name} (${session.device.serial})\nPort: ${session.port}\nStatus: ${session.status}`;
    return item;
  }

  private sessionDetails(session: Session): SessionTreeItem[] {
    const items: SessionTreeItem[] = [];
    items.push(new SessionTreeItem(`Device: ${session.device.name} (${session.device.serial})`));
    items.push(new SessionTreeItem(`Port: ${session.port}`));
    if (session.config) {
      items.push(new SessionTreeItem(`Config: ${path.basename(session.config)}`));
    }
    if (session.lastResult) {
      const r = session.lastResult;
      const icon = r.outcome === 'passed' ? '✓' : '✗';
      items.push(new SessionTreeItem(`Last run: ${r.passed}/${r.total} passed ${icon}`));
    }
    return items;
  }

  private async addSession(): Promise<void> {
    const devices = await listDevices();
    if (devices.length === 0) {
      vscode.window.showWarningMessage('No devices found. Start a device first.');
      return;
    }

    const picked = await vscode.window.showQuickPick(
      devices.map(d => ({
        label: d.name,
        description: `${d.platform} — ${d.serial}`,
        device: d,
      })),
      { placeHolder: 'Select device for this session' }
    );
    if (!picked) { return; }

    const portStr = await vscode.window.showInputBox({
      prompt: 'Agent host port (unique per session for parallel testing)',
      value: String(48686 + this.sessions.length),
    });
    if (!portStr) { return; }
    const port = parseInt(portStr, 10);

    const name = await vscode.window.showInputBox({
      prompt: 'Session name',
      value: `${picked.device.name}`,
    });
    if (!name) { return; }

    const configFiles = await vscode.workspace.findFiles('probe*.yaml', '**/node_modules/**', 10);
    let config: string | undefined;
    if (configFiles.length > 1) {
      const configPick = await vscode.window.showQuickPick(
        [
          { label: 'Default (probe.yaml)', path: undefined },
          ...configFiles.map(f => ({ label: path.basename(f.fsPath), path: f.fsPath })),
        ],
        { placeHolder: 'Select config file (optional)' }
      );
      config = configPick?.path;
    }

    const session: Session = {
      id: `session-${Date.now()}`,
      name,
      device: picked.device,
      port,
      devicePort: port,
      status: 'disconnected',
      config,
    };

    this.sessions.push(session);
    this.saveSessions();
    this.refresh();
  }

  private removeSession(item: SessionTreeItem): void {
    if (!item.session) { return; }
    this.sessions = this.sessions.filter(s => s.id !== item.session!.id);
    this.saveSessions();
    this.refresh();
  }

  private async editSession(item: SessionTreeItem): Promise<void> {
    if (!item.session) { return; }
    const session = item.session;

    const name = await vscode.window.showInputBox({
      prompt: 'Session name',
      value: session.name,
    });
    if (name) { session.name = name; }

    const portStr = await vscode.window.showInputBox({
      prompt: 'Agent host port',
      value: String(session.port),
    });
    if (portStr) { session.port = parseInt(portStr, 10); }

    this.saveSessions();
    this.refresh();
  }

  private runOnSession(item: SessionTreeItem): void {
    if (!item.session) { return; }
    const session = item.session;
    const ws = getWorkspaceRoot();
    if (!ws) { return; }

    const args = buildTestArgs(ws, {
      device: session.device.serial,
      port: session.port,
      config: session.config,
      autoConfirm: true,
    });

    runInTerminal(`Probe: ${session.name}`, getProbeCommand(), args, ws);
    session.status = 'running';
    this.refresh();
  }

  private runFileOnSession(item: SessionTreeItem): void {
    if (!item.session) { return; }
    const editor = vscode.window.activeTextEditor;
    if (!editor) { return; }

    const session = item.session;
    const args = buildTestArgs(editor.document.uri.fsPath, {
      device: session.device.serial,
      port: session.port,
      config: session.config,
      autoConfirm: true,
    });

    runInTerminal(`Probe: ${session.name}`, getProbeCommand(), args, getWorkspaceRoot());
    session.status = 'running';
    this.refresh();
  }

  private runOnAllSessions(): void {
    const ws = getWorkspaceRoot();
    if (!ws) { return; }

    if (this.sessions.length === 0) {
      vscode.window.showWarningMessage('No sessions configured. Add a session first.');
      return;
    }

    for (const session of this.sessions) {
      const args = buildTestArgs(ws, {
        device: session.device.serial,
        port: session.port,
        config: session.config,
        autoConfirm: true,
      });

      runInTerminal(`Probe: ${session.name}`, getProbeCommand(), args, ws);
      session.status = 'running';
    }

    this.refresh();
    vscode.window.showInformationMessage(`Running tests on ${this.sessions.length} session(s) in parallel.`);
  }

  getSessions(): Session[] {
    return this.sessions;
  }

  private loadSessions(): void {
    if (!this.storePath || !fs.existsSync(this.storePath)) { return; }
    try {
      const data = fs.readFileSync(this.storePath, 'utf-8');
      this.sessions = JSON.parse(data);
      // Reset runtime state
      for (const s of this.sessions) {
        s.status = 'disconnected';
        s.activeTest = undefined;
      }
    } catch {
      this.sessions = [];
    }
  }

  private saveSessions(): void {
    if (!this.storePath) { return; }
    const dir = path.dirname(this.storePath);
    if (!fs.existsSync(dir)) {
      fs.mkdirSync(dir, { recursive: true });
    }
    const data = this.sessions.map(s => ({
      id: s.id,
      name: s.name,
      device: s.device,
      config: s.config,
      port: s.port,
      devicePort: s.devicePort,
    }));
    fs.writeFileSync(this.storePath, JSON.stringify(data, null, 2));
  }
}

export class SessionTreeItem extends vscode.TreeItem {
  session?: Session;

  constructor(
    label: string,
    collapsibleState: vscode.TreeItemCollapsibleState = vscode.TreeItemCollapsibleState.None,
    command?: vscode.Command,
  ) {
    super(label, collapsibleState);
    if (command) {
      this.command = command;
    }
  }
}
