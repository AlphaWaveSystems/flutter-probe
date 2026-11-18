// Keyboard focus for a modal overlay: it takes the focus when it opens, Tab and Shift+Tab go round inside it (never to the page behind), Escape asks it to
// close, and the focus goes back to where it was when it closes. The part that decides where Tab goes is pure (nextFocusIndex) and has node tests; the
// rest touches the document and is covered by the browser specs.

/** The index Tab (or Shift+Tab, backwards) moves to among count tab stops, going round; current is -1 when the focus is outside them. */
export function nextFocusIndex(count: number, current: number, backwards: boolean): number {
  if (count <= 0) return -1;
  if (current < 0) return backwards ? count - 1 : 0;
  return backwards ? (current - 1 + count) % count : (current + 1) % count;
}

const TAB_STOPS = 'a[href], button, input, select, textarea, [tabindex="0"]';

/** The tab stops inside root, in document order: the ones that are shown and not disabled. */
export function tabStops(root: HTMLElement): HTMLElement[] {
  return Array.from(root.querySelectorAll<HTMLElement>(TAB_STOPS)).filter((el) => !(el as HTMLButtonElement).disabled && el.getClientRects().length > 0);
}

export interface ModalFocus {
  /** The element that has the focus when the overlay opens: the safe choice (Cancel), so Enter does not do the destructive or sending thing. */
  initial: HTMLElement;
  /** Escape was pressed: the overlay's own "no". */
  onEscape: () => void;
  /** Where the focus goes back to. The button that opened the overlay is disabled while the question is open, and a disabled button gives up the focus, so by the time
   * the overlay is shown the focus is on the page, not on it: the caller names it. Default: whatever had the focus. */
  returnTo?: HTMLElement | null;
  /** Where the focus goes when what it should go back to cannot take it (still disabled, or gone from the page), for example the chat's input while a reply streams. */
  fallback?: HTMLElement | null;
}

/**
 * Starts the focus handling of an overlay that has just been shown. It returns release, which stops it and puts the focus back where it was.
 * The overlay must be visible when this is called (its tab stops are looked up when Tab is pressed, not before).
 */
export function trapFocus(overlay: HTMLElement, m: ModalFocus): () => void {
  const opener = m.returnTo ?? (document.activeElement as HTMLElement | null);
  const onKey = (e: KeyboardEvent) => {
    // The keys this handles are not passed on: the window's own handler (which closes the help card, the settings and the WiFi list on Escape) must not also run.
    if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
      m.onEscape();
      return;
    }
    if (e.key !== "Tab") return;
    e.stopPropagation();
    const stops = tabStops(overlay);
    if (stops.length === 0) {
      e.preventDefault();
      return;
    }
    e.preventDefault();
    const at = stops.indexOf(document.activeElement as HTMLElement);
    stops[nextFocusIndex(stops.length, at, e.shiftKey)].focus();
  };
  document.addEventListener("keydown", onKey);
  m.initial.focus();
  return () => {
    document.removeEventListener("keydown", onKey);
    // After the caller's own code has run: the button that opened the overlay is disabled while the question is open and is enabled again when the
    // answer has been handled, and a disabled button cannot take the focus.
    setTimeout(() => {
      const target = opener && opener.isConnected && !(opener as HTMLButtonElement).disabled ? opener : m.fallback;
      target?.focus?.();
    }, 0);
  };
}
