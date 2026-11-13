/**
 * latestWins wraps an async fetch and the step that shows its result. When two runs overlap, only the one that started last may
 * show its result, so two refreshes that finish out of order cannot leave an older answer on screen. A failure is handled the
 * same way: only the latest run's failure is shown.
 */
export function latestWins<T>(fetch: () => Promise<T>, show: (value: T) => void, showFailure: () => void): () => Promise<void> {
  let latest = 0;
  return async () => {
    const mine = ++latest;
    try {
      const value = await fetch();
      if (mine === latest) show(value);
    } catch {
      if (mine === latest) showFailure();
    }
  };
}
