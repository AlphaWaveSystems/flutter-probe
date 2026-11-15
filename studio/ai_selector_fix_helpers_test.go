package main

// suggestFix is the two calls a window makes, back to back: the preview, then the send of what it returned.
func suggestFix(a *App, file string) (SelectorFix, error) {
	p, err := a.PreviewSelectorFix(file)
	if err != nil {
		return SelectorFix{}, err
	}
	return a.SelectorFixPreviewed(p.ID)
}
