import { legacyGamepads } from "./legacy";

type Action = "left" | "right" | "up" | "down" | "a" | "b" | "l1" | "r1";
type Pad = { buttons: ArrayLike<{ pressed: boolean }>; axes: ArrayLike<number> };
const directions = new Set<Action>(["left", "right", "up", "down"]);

function actionsFor(pads: Array<Pad | null>): Set<Action> {
  const now = new Set<Action>();
  for (const pad of pads) {
    if (!pad) continue;
    const b = (i: number) => pad.buttons[i]?.pressed;
    const ax = pad.axes[0] || 0, ay = pad.axes[1] || 0;
    if (b(14) || ax < -0.6) now.add("left");
    if (b(15) || ax > 0.6) now.add("right");
    if (b(12) || ay < -0.6) now.add("up");
    if (b(13) || ay > 0.6) now.add("down");
    if (b(0)) now.add("a");
    if (b(1)) now.add("b");
    if (b(4)) now.add("l1");
    if (b(5)) now.add("r1");
  }
  return now;
}

// The published @napplet/nap 0.33 SDK does not yet include draft GAMEPAD.
// Consume the runtime's binding directly until its SDK entrypoint is released.
// Focus loss and held buttons across focus regain must never click the store.
export function startController(handlers: Record<Action, () => void>): () => void {
  let pressed = new Set<Action>();
  let blocked = new Set<Action>();
  let focused = false;
  let repeatAt = 0;
  let frame = 0;
  let stopped = false;

  function poll(t: number): void {
    if (stopped) return;
    const domain = window.napplet?.gamepad;
    const active = !document.hidden && (domain ? domain.available && domain.focused :
      import.meta.env.MODE === "legacy" && document.hasFocus());
    if (!active) {
      pressed.clear();
      blocked.clear();
      focused = false;
    } else {
      const pads = domain ? domain.getGamepads() :
        import.meta.env.MODE === "legacy" ? legacyGamepads() : [];
      const now = actionsFor(pads);
      if (!focused) {
        blocked = new Set(now);
        focused = true;
        repeatAt = 0;
      }
      for (const action of blocked) if (!now.has(action)) blocked.delete(action);
      for (const action of now) {
        if (blocked.has(action)) continue;
        const fresh = !pressed.has(action);
        const repeat = directions.has(action) && t > repeatAt;
        if (!fresh && !repeat) continue;
        if (directions.has(action)) repeatAt = t + (fresh ? 380 : 120);
        handlers[action]();
      }
      pressed = now;
    }
    frame = requestAnimationFrame(poll);
  }
  frame = requestAnimationFrame(poll);
  return () => { stopped = true; cancelAnimationFrame(frame); };
}
