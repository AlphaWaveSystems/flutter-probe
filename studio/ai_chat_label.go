package main

import "errors"

const (
	chatLabelOff = "AI is off"
	chatLabelFix = "AI needs a fix, see Settings"
)

// onThisMachine says whether an endpoint is this machine. It is the decision checkLocalEndpoint makes with no confirmation, so
// the header can never call an address local that the settings refuse as not local, and there is no second parser to drift.
func onThisMachine(endpoint string) bool {
	return checkLocalEndpoint(endpoint, false) == nil
}

// chatLabel is the short line in the chat header: where the next message would go. It shows the provider and the model id and
// nothing else: no endpoint, no key, no key source and no path. The model id comes from probe.yaml, so it passes the gate with
// the exact key removed, and it is cut to 200 bytes, like every provider string. err is the refusal aiProvider returned, if any.
func chatLabel(s aiProviderSettings, err error) string {
	if err != nil {
		if errors.Is(err, errAIOff) || errors.Is(err, errNoAPIKey) {
			return chatLabelOff
		}
		return chatLabelFix
	}
	switch s.Provider {
	case "anthropic":
		model := s.Model
		if model == "" {
			model = chatModel
		}
		return "Anthropic, " + modelID(model, s.APIKey)
	case "openai":
		return "OpenAI, " + modelID(s.Model, s.APIKey)
	case "local":
		where := "remote endpoint"
		if onThisMachine(s.Endpoint) {
			where = "on this machine"
		}
		return modelID(s.Model, s.APIKey) + ", " + where
	}
	return chatLabelOff
}

// AIChatLabel returns the chat header's line. It reads the same resolved settings as Chat and PreviewAIChat, so the header
// names the destination a message would go to, or says that AI is off or needs a fix. It makes no request.
func (a *App) AIChatLabel() string {
	s, err := a.aiProvider()
	return chatLabel(s, err)
}
