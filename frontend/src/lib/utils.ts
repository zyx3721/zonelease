import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export type ZlTheme = 'dark' | 'light';

const themeStorageKey = 'zonelease.zl-theme';

export function getInitialZlTheme(): ZlTheme {
  if (typeof window === 'undefined') return 'dark';
  return window.localStorage.getItem(themeStorageKey) === 'light' ? 'light' : 'dark';
}

export function applyZlTheme(theme: ZlTheme) {
  if (typeof document === 'undefined') return;
  document.documentElement.dataset.zlTheme = theme;
  document.documentElement.style.colorScheme = theme;
}

export function persistZlTheme(theme: ZlTheme) {
  if (typeof window === 'undefined') return;
  window.localStorage.setItem(themeStorageKey, theme);
}

export function toggleZlTheme(theme: ZlTheme): ZlTheme {
  return theme === 'dark' ? 'light' : 'dark';
}
