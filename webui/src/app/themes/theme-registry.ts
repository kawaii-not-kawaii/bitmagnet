export type ThemeInfo<Key extends string = string> = {
  key: Key;
  label: string;
  dark: boolean;
  colors: {
    bg: string;
    surface: string;
    accent: string;
  };
};

export type Themes<Keys extends string = string> = {
  [key in Keys]: ThemeInfo<key>;
};

const dark = true;
const light = false;

const _themes = {
  "bitmagnet-dark": theme(
    "bitmagnet-dark",
    "Bitmagnet Dark",
    dark,
    "#131319",
    "#1b1b23",
    "#8b7cf0",
  ),
  catppuccin: theme(
    "catppuccin",
    "Catppuccin",
    dark,
    "#181825",
    "#1e1e2e",
    "#cba6f7",
  ),
  "tokyo-night": theme(
    "tokyo-night",
    "Tokyo Night",
    dark,
    "#16161e",
    "#1a1b26",
    "#7aa2f7",
  ),
  dracula: theme("dracula", "Dracula", dark, "#21222c", "#282a36", "#bd93f9"),
  nord: theme("nord", "Nord", dark, "#2e3440", "#343b4a", "#88c0d0"),
  gruvbox: theme("gruvbox", "Gruvbox", dark, "#282828", "#32302f", "#fabd2f"),
  "one-dark": theme(
    "one-dark",
    "One Dark",
    dark,
    "#21252b",
    "#282c34",
    "#61afef",
  ),
  solarized: theme(
    "solarized",
    "Solarized",
    dark,
    "#002b36",
    "#073642",
    "#268bd2",
  ),
  "rose-pine": theme(
    "rose-pine",
    "Rosé Pine",
    dark,
    "#191724",
    "#1f1d2e",
    "#c4a7e7",
  ),
  kanagawa: theme(
    "kanagawa",
    "Kanagawa",
    dark,
    "#1f1f28",
    "#2a2a37",
    "#7e9cd8",
  ),
  vesper: theme("vesper", "Vesper", dark, "#101010", "#161616", "#ffc799"),
  terminal: theme(
    "terminal",
    "Terminal",
    dark,
    "#000000",
    "#0c0c0c",
    "#33d17a",
  ),
  "bitmagnet-light": theme(
    "bitmagnet-light",
    "Bitmagnet Light",
    light,
    "#f6f5f2",
    "#ffffff",
    "#5b4fd6",
  ),
  "catppuccin-latte": theme(
    "catppuccin-latte",
    "Catppuccin Latte",
    light,
    "#e6e9ef",
    "#eff1f5",
    "#8839ef",
  ),
  "tokyo-night-day": theme(
    "tokyo-night-day",
    "Tokyo Night Day",
    light,
    "#e1e2e7",
    "#eaebef",
    "#2e7de9",
  ),
  "gruvbox-light": theme(
    "gruvbox-light",
    "Gruvbox Light",
    light,
    "#f2e5bc",
    "#fbf1c7",
    "#b57614",
  ),
  "one-light": theme(
    "one-light",
    "One Light",
    light,
    "#eaeaeb",
    "#fafafa",
    "#4078f2",
  ),
  "solarized-light": theme(
    "solarized-light",
    "Solarized Light",
    light,
    "#eee8d5",
    "#fdf6e3",
    "#268bd2",
  ),
  "rose-pine-dawn": theme(
    "rose-pine-dawn",
    "Rosé Pine Dawn",
    light,
    "#faf4ed",
    "#fffaf3",
    "#907aa9",
  ),
  "kanagawa-lotus": theme(
    "kanagawa-lotus",
    "Kanagawa Lotus",
    light,
    "#f2ecbc",
    "#e7dba0",
    "#4d699b",
  ),
};

function theme<Key extends string>(
  key: Key,
  label: string,
  isDark: boolean,
  bg: string,
  surface: string,
  accent: string,
): ThemeInfo<Key> {
  return { key, label, dark: isDark, colors: { bg, surface, accent } };
}

export type ThemeKey = keyof typeof _themes;
export const themes: Themes<ThemeKey> = _themes;
export const defaultLightTheme = "bitmagnet-light";
export const defaultDarkTheme = "bitmagnet-dark";
