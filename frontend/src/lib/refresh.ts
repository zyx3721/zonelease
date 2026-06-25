const EVENT = 'zonelease-runtime-refresh';

export function emitZoneLeaseRefresh() {
  if (typeof window === 'undefined') return;
  window.dispatchEvent(new CustomEvent(EVENT));
}

export function onZoneLeaseRefresh(callback: () => void) {
  if (typeof window === 'undefined') return () => undefined;
  window.addEventListener(EVENT, callback);
  return () => window.removeEventListener(EVENT, callback);
}

export function runtimeEventsUrl() {
  return '/api/events';
}
