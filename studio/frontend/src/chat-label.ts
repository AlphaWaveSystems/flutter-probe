import { latestWins } from "./latest-wins.ts";

/** The chat header line in three parts: only the model comes from probe.yaml, so only it is shortened when it is long. */
export interface ChatLabelParts {
  prefix: string;
  model: string;
  suffix: string;
}

/** The whole line, as the title shows it. */
export function labelText(parts: ChatLabelParts): string {
  return parts.prefix + parts.model + parts.suffix;
}

/**
 * chatLabelRefresher fetches the label and shows it, with the latest call winning. A refresh that fails shows null, which the page
 * renders as a blank header, never the last label: an old destination must not stay on screen when it can no longer be known.
 */
export function chatLabelRefresher(fetch: () => Promise<ChatLabelParts>, show: (parts: ChatLabelParts | null) => void): () => Promise<void> {
  return latestWins(fetch, (parts) => show(parts), () => show(null));
}
