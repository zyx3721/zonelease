import { api } from './auth';

export type PasswordResetCaptcha = {
  token: string;
  question: string;
  expiresAt: string;
};

export type PasswordResetChannel = {
  id: string;
  name: string;
  maskedTo: string;
  requiresTo: boolean;
};

export function fetchPasswordResetCaptcha() {
  return api<PasswordResetCaptcha>('/api/auth/password-reset/captcha', { auth: false });
}

export function verifyPasswordResetIdentity(body: {
  username: string;
  captchaToken: string;
  captchaAnswer: string;
}) {
  return api<{ verificationToken: string; channels: PasswordResetChannel[] }>(
    '/api/auth/password-reset/verify',
    { method: 'POST', auth: false, body: JSON.stringify(body) }
  );
}

export function sendPasswordResetCode(body: {
  username: string;
  verificationToken: string;
  channel: string;
  verifyEmail: string;
  to?: string;
}) {
  return api<{ cooldownSeconds: number; devCode?: string }>('/api/auth/password-reset/send', {
    method: 'POST',
    auth: false,
    body: JSON.stringify(body),
  });
}

export function confirmPasswordReset(body: {
  username: string;
  verificationToken: string;
  code: string;
  newPassword: string;
  confirmPassword: string;
}) {
  return api<{ status: string }>('/api/auth/password-reset/confirm', {
    method: 'POST',
    auth: false,
    body: JSON.stringify(body),
  });
}
