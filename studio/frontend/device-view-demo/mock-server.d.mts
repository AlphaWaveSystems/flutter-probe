export const SCREEN: { w: number; h: number; dpr: number };
export const NODES: Array<{
  i: number;
  p: number;
  type: string;
  key: string | null;
  text: string | null;
  label: string | null;
  x: number;
  y: number;
  w: number;
  h: number;
  interactive: boolean;
}>;
export function generateTestPng(): Buffer;
export function buildFrame(seq: number, w: number, h: number, fmt: string, image: Buffer): Buffer;
export function startMockServer(o?: {
  port?: number;
  token?: string;
  fps?: number;
  image?: Buffer;
}): Promise<{
  port: number;
  token: string;
  url: string;
  received: Array<Record<string, unknown>>;
  clientCount(): number;
  dropClients(): void;
  close(): Promise<void>;
}>;
