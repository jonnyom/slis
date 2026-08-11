import { expect, test } from "bun:test";
import { requestTerminalFullRepaint } from "./repaint";

test("requests OpenTUI's next frame as a full repaint", () => {
  let renderRequests = 0;
  const renderer = {
    requestRender() {
      renderRequests++;
    },
  };

  requestTerminalFullRepaint(renderer);

  expect(Reflect.get(renderer, "forceFullRepaintRequested")).toBe(true);
  expect(renderRequests).toBe(1);
});
