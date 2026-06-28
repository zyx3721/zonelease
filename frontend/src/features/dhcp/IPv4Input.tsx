import { useMemo, useRef, type KeyboardEvent } from 'react';

interface IPv4InputProps {
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  'aria-label'?: string;
}

function splitIPv4(value: string) {
  const parts = value.split('.');
  return Array.from({ length: 4 }, (_, index) => parts[index] ?? '');
}

export function IPv4Input({ value, onChange, disabled, 'aria-label': ariaLabel }: IPv4InputProps) {
  const parts = useMemo(() => splitIPv4(value), [value]);
  const inputRefs = useRef<Array<HTMLInputElement | null>>([]);

  function updatePart(index: number, rawValue: string) {
    const nextPart = rawValue.replace(/\D/g, '').slice(0, 3);
    const nextParts = [...parts];
    nextParts[index] = nextPart;
    onChange(nextParts.join('.'));
  }

  function focusPart(index: number, options: { select?: boolean } = {}) {
    const input = inputRefs.current[index];
    if (!input) return;
    input.focus();
    if (options.select) {
      input.select();
      return;
    }
    const end = input.value.length;
    input.setSelectionRange(end, end);
  }

  function handleKeyDown(index: number, event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === '.' || event.key === 'Decimal') {
      event.preventDefault();
      if (index < 3) focusPart(index + 1, { select: true });
      return;
    }
    if (event.key === 'ArrowLeft') {
      const input = event.currentTarget;
      if (input.selectionStart === 0 && input.selectionEnd === 0 && index > 0) {
        event.preventDefault();
        focusPart(index - 1);
      }
      return;
    }
    if (event.key === 'ArrowRight') {
      const input = event.currentTarget;
      const end = input.value.length;
      if (input.selectionStart === end && input.selectionEnd === end && index < 3) {
        event.preventDefault();
        const nextInput = inputRefs.current[index + 1];
        if (!nextInput) return;
        nextInput.focus();
        nextInput.setSelectionRange(0, 0);
      }
      return;
    }
    if (event.key !== 'Backspace') return;
    const input = event.currentTarget;
    if (input.value !== '' || input.selectionStart !== 0 || input.selectionEnd !== 0 || index === 0)
      return;
    event.preventDefault();
    focusPart(index - 1);
  }

  return (
    <div
      className="zl-form-control flex h-10 items-center rounded-lg px-2 transition-colors"
      aria-label={ariaLabel}
    >
      {parts.map((part, index) => (
        <span key={index} className="flex min-w-0 flex-1 items-center">
          <input
            type="text"
            inputMode="numeric"
            pattern="[0-9]*"
            maxLength={3}
            value={part}
            disabled={disabled}
            ref={element => {
              inputRefs.current[index] = element;
            }}
            onKeyDown={event => handleKeyDown(index, event)}
            onChange={event => updatePart(index, event.target.value)}
            className="h-8 min-w-0 flex-1 bg-transparent text-center text-sm outline-none disabled:cursor-not-allowed disabled:opacity-50"
            aria-label={`${ariaLabel ?? 'IPv4 地址'}第 ${index + 1} 段`}
          />
          {index < 3 ? <span className="px-1 text-sm text-muted-foreground">.</span> : null}
        </span>
      ))}
    </div>
  );
}
