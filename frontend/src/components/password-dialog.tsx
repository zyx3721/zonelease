import { type FormEvent, useState } from 'react';
import { CheckCircle2, Eye, EyeOff, KeyRound, Loader2, X } from 'lucide-react';
import { toast } from 'sonner';
import { changePassword } from '@/lib/auth';

type Field = 'old_password' | 'new_password' | 'confirm_password';

const emptyForm = {
  old_password: '',
  new_password: '',
  confirm_password: '',
};

const emptyFlags: Record<Field, boolean> = {
  old_password: false,
  new_password: false,
  confirm_password: false,
};

export function PasswordDialog({
  open,
  onClose,
  onSuccess,
}: {
  open: boolean;
  onClose: () => void;
  onSuccess?: () => void;
}) {
  const [form, setForm] = useState(emptyForm);
  const [errors, setErrors] = useState(emptyFlags);
  const [visible, setVisible] = useState(emptyFlags);
  const [message, setMessage] = useState('');
  const [submitting, setSubmitting] = useState(false);

  if (!open) return null;

  function close() {
    setForm(emptyForm);
    setErrors(emptyFlags);
    setVisible(emptyFlags);
    setMessage('');
    setSubmitting(false);
    onClose();
  }

  function update(field: Field, value: string) {
    setForm(current => ({ ...current, [field]: value }));
    setErrors(current => ({ ...current, [field]: false }));
    setMessage('');
  }

  function validate() {
    const next = { ...emptyFlags };
    if (form.old_password.length < 6) next.old_password = true;
    if (form.new_password.length < 6) next.new_password = true;
    if (form.confirm_password.length < 6) next.confirm_password = true;
    if (next.old_password || next.new_password || next.confirm_password) {
      setErrors(next);
      setMessage('密码至少 6 个字符');
      return false;
    }
    if (form.old_password === form.new_password) {
      setErrors({ ...emptyFlags, new_password: true });
      setMessage('新密码不能与旧密码相同');
      return false;
    }
    if (form.new_password !== form.confirm_password) {
      setErrors({ ...emptyFlags, confirm_password: true });
      setMessage('新密码与确认密码不一致');
      return false;
    }
    return true;
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!validate()) return;
    setSubmitting(true);
    try {
      await changePassword(form);
      toast.success('密码已修改，请重新登录');
      close();
      onSuccess?.();
    } catch (error) {
      const text = error instanceof Error ? error.message : '密码修改失败';
      if (text.includes('旧密码')) setErrors({ ...emptyFlags, old_password: true });
      setMessage(text);
      toast.error(text);
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div
      className="zl-dialog-backdrop fixed inset-0 z-[70] flex items-center justify-center px-4"
      role="presentation"
    >
      <form
        className="zl-dialog-panel w-full max-w-[440px] rounded-xl p-5"
        role="dialog"
        aria-modal="true"
        aria-labelledby="change-password-title"
        onSubmit={submit}
      >
        <div className="relative z-10">
          <div className="mb-5 flex items-start gap-3">
            <div className="grid h-11 w-11 shrink-0 place-items-center rounded-lg border border-blue-400/30 bg-blue-500/15 text-blue-300">
              <KeyRound size={21} />
            </div>
            <div className="min-w-0 flex-1">
              <h2
                id="change-password-title"
                className="text-base font-semibold"
                style={{ color: 'var(--zl-text)' }}
              >
                修改密码
              </h2>
              <p className="mt-1 text-sm" style={{ color: 'var(--zl-text-muted)' }}>
                输入旧密码后设置新的登录密码。
              </p>
            </div>
            <button
              type="button"
              className="zl-action-button grid h-8 w-8 place-items-center rounded-lg border"
              onClick={close}
              aria-label="关闭"
            >
              <X size={16} />
            </button>
          </div>

          <div className="space-y-4">
            {(
              [
                ['old_password', '旧密码', '请输入旧密码', 'current-password'],
                ['new_password', '新密码', '请输入新密码', 'new-password'],
                ['confirm_password', '确认密码', '请再次输入新密码', 'new-password'],
              ] as const
            ).map(([field, label, placeholder, autoComplete]) => (
              <label key={field} className="block">
                <span
                  className="mb-1.5 block text-xs font-medium"
                  style={{ color: errors[field] ? '#f87171' : 'var(--zl-text-muted)' }}
                >
                  {label}
                </span>
                <span className="relative block">
                  <input
                    value={form[field]}
                    onChange={event => update(field, event.target.value)}
                    type={visible[field] ? 'text' : 'password'}
                    autoComplete={autoComplete}
                    placeholder={placeholder}
                    className="zl-form-control h-10 w-full rounded-lg px-3 pr-10 text-sm outline-none"
                    style={{ borderColor: errors[field] ? 'rgba(248,113,113,0.7)' : undefined }}
                  />
                  <button
                    type="button"
                    className="absolute right-1.5 top-1/2 grid h-7 w-7 -translate-y-1/2 place-items-center rounded-md"
                    onClick={() =>
                      setVisible(current => ({ ...current, [field]: !current[field] }))
                    }
                    aria-label={visible[field] ? `隐藏${label}` : `显示${label}`}
                    style={{ color: 'var(--zl-text-muted)' }}
                  >
                    {visible[field] ? <EyeOff size={16} /> : <Eye size={16} />}
                  </button>
                </span>
              </label>
            ))}
          </div>

          {message ? (
            <div
              className="mt-4 rounded-lg px-3 py-2 text-sm"
              style={{
                color: '#fca5a5',
                background: 'rgba(239,68,68,0.1)',
                border: '1px solid rgba(239,68,68,0.25)',
              }}
            >
              {message}
            </div>
          ) : null}

          <div className="mt-6 flex justify-end gap-3">
            <button
              type="button"
              className="zl-action-button rounded-lg border px-4 py-2 text-sm"
              onClick={close}
            >
              取消
            </button>
            <button
              type="submit"
              disabled={submitting}
              className="zl-action-button flex items-center gap-2 rounded-lg border px-4 py-2 text-sm font-medium text-white disabled:opacity-60"
              style={{
                background: 'linear-gradient(135deg, #2563eb, #06b6d4)',
                borderColor: 'rgba(96,165,250,0.35)',
              }}
            >
              {submitting ? (
                <Loader2 size={16} className="zl-spinner" />
              ) : (
                <CheckCircle2 size={16} />
              )}
              {submitting ? '提交中...' : '确认修改'}
            </button>
          </div>
        </div>
      </form>
    </div>
  );
}
