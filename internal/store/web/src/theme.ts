import type { Theme } from "@napplet/nap/theme/types";

type RGB = [number, number, number];

// Hosts only send background, text and primary, so derive the rest of the palette from them.
export function parseColor(value: string): RGB | null {
  const hex = value.trim().match(/^#([0-9a-f]{3}|[0-9a-f]{6})(?:[0-9a-f]{2})?$/i)?.[1];
  if (hex) {
    const full = hex.length === 3 ? [...hex].map(c => c + c).join("") : hex;
    return [0, 2, 4].map(i => parseInt(full.slice(i, i + 2), 16)) as RGB;
  }
  const rgb = value.trim().match(/^rgba?\(\s*(\d+)[\s,]+(\d+)[\s,]+(\d+)/i);
  return rgb ? [Number(rgb[1]), Number(rgb[2]), Number(rgb[3])] : null;
}

function luminance([r, g, b]: RGB): number {
  const channel = (c: number) => { c /= 255; return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4; };
  return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b);
}

function mix(a: RGB, b: RGB, amount: number): string {
  const c = a.map((v, i) => Math.round(v + (b[i] - v) * amount));
  return "#" + c.map(v => v.toString(16).padStart(2, "0")).join("");
}

function alpha([r, g, b]: RGB, a: number): string {
  return `rgba(${r}, ${g}, ${b}, ${a})`;
}

export function themePalette(theme: Theme): Record<string, string> | null {
  const bg = parseColor(theme.colors.background);
  const text = parseColor(theme.colors.text);
  const accent = parseColor(theme.colors.primary);
  if (!bg || !text || !accent) return null;
  const light = luminance(bg) > 0.4;
  const black: RGB = [0, 0, 0], white: RGB = [255, 255, 255];
  return {
    "color-scheme": light ? "light" : "dark",
    "--bg": theme.colors.background,
    "--text": theme.colors.text,
    "--accent": theme.colors.primary,
    "--focus": theme.colors.primary,
    "--accent-ink": luminance(accent) > 0.35 ? mix(accent, black, 0.88) : mix(accent, white, 0.94),
    "--accent-soft": alpha(accent, light ? 0.18 : 0.15),
    "--panel": mix(bg, text, light ? 0.05 : 0.06),
    "--panel-2": mix(bg, text, light ? 0.1 : 0.12),
    "--muted": mix(text, bg, light ? 0.38 : 0.42),
    "--header-top": mix(bg, text, light ? 0.06 : 0.03),
    "--chip": alpha(text, 0.08),
    "--shimmer": alpha(text, 0.12),
    "--scrim": alpha(bg, 0.55),
    "--hero-brightness": light ? "1.2" : ".45",
    "--ok": light ? "#1e8a45" : "#5ccf7a",
    "--bad": light ? "#c93535" : "#ff6b6b",
  };
}

export function applyTheme(theme: Theme, root: HTMLElement = document.documentElement): void {
  const palette = themePalette(theme);
  if (!palette) return;
  for (const [key, value] of Object.entries(palette)) root.style.setProperty(key, value);
  root.dataset.theme = palette["color-scheme"];
}
