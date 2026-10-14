// Public entry of the device view component. No Wails or VS Code imports.
export { mountDeviceView } from "./view.ts";
export type { DeviceViewHandle, DeviceViewOptions } from "./view.ts";
export type {
  Caps,
  ClientMessage,
  Config,
  DecodedFrame,
  DeviceInfo,
  DeviceNode,
  FrameHeader,
  ServerMessage,
  Status,
  StatusState,
} from "./protocol.ts";
export { selectorFor, quoteString } from "./selector.ts";
