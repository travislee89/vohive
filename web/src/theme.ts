export type ThemeMode = 'light' | 'dark' | 'auto'

/** 点击按钮时循环的顺序：白色 → 深色 → 自动 → 白色 */
export const THEME_ORDER: ThemeMode[] = ['light', 'dark', 'auto']

export const THEME_KEY = 'theme'

export function resolveDark(theme: ThemeMode, systemDark: boolean): boolean {
  return theme === 'dark' || (theme === 'auto' && systemDark)
}

export function applyDarkClass(dark: boolean) {
  document.documentElement.classList.toggle('dark', dark)
}

export function getSystemDark(): boolean {
  return typeof window !== 'undefined' && !!window.matchMedia?.('(prefers-color-scheme: dark)')?.matches
}
