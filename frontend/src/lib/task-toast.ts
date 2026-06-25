export const taskToastOptions = {
  position: 'top-right' as const,
  duration: Infinity,
};

export const taskToastDoneOptions = {
  duration: 3000,
};

export function taskToastOptionsFor(id?: string | number) {
  return {
    ...taskToastOptions,
    ...(id !== undefined ? { id } : {}),
  };
}

export function taskToastDoneOptionsFor(id?: string | number) {
  return {
    ...taskToastOptions,
    ...taskToastDoneOptions,
    ...(id !== undefined ? { id } : {}),
  };
}
