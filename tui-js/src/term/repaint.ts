export function requestTerminalFullRepaint(renderer: {
  requestRender(): void;
}): void {
  Reflect.set(renderer, "forceFullRepaintRequested", true);
  renderer.requestRender();
}
