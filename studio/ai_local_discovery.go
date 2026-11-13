package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	defaultLocalEndpoint = "http://127.0.0.1:11434/v1" // Ollama's default
	discoveryTimeout     = 2 * time.Second             // per target: a server on this machine answers at once or not at all
	discoveryBodyLimit   = 1 << 20
	maxDiscoveredModels  = 100
)

// localDiscoveryTarget is the endpoint Find local models asks besides the one in the form. It is the default, and only the
// headless build can change it, through PROBE_STUDIO_LOCAL_ENDPOINT, so the browser suite never asks a real Ollama.
var localDiscoveryTarget = defaultLocalEndpoint

// localDiscoveryEndpoint decides the default target. The override is read by the headless build only, and it must pass the same
// loopback decision as every local endpoint, so it can never point discovery at another machine. When it is ignored, note says
// why, and main writes it to stderr.
func localDiscoveryEndpoint(headless bool, env string) (endpoint, note string) {
	if env == "" {
		return defaultLocalEndpoint, ""
	}
	if !headless {
		return defaultLocalEndpoint, "PROBE_STUDIO_LOCAL_ENDPOINT is read by the headless build only, so it was ignored"
	}
	if err := checkLocalEndpoint(env, false); err != nil {
		return defaultLocalEndpoint, "PROBE_STUDIO_LOCAL_ENDPOINT was ignored: " + err.Error()
	}
	return env, ""
}

// LocalModels is what Find local models found: the endpoint that answered, its host for the status line, and its model ids.
type LocalModels struct {
	Endpoint string   `json:"endpoint"`
	Host     string   `json:"host"`
	Models   []string `json:"models"`
	Skipped  string   `json:"skipped,omitempty"` // why the endpoint in the form was not asked, when it was not
}

// FindLocalModels asks the machine's own model server which models it has, for a person who has not set up a provider yet. It
// asks only loopback addresses: the endpoint in the form, when it passes checkLocalEndpoint with no confirmation, and then the
// default one. It sends a GET for the model list and nothing else: no body, no key, no redirect followed, 2 seconds per
// address. It writes nothing and pulls nothing. Each id passes the same cleaning as every provider string.
func (a *App) FindLocalModels(formEndpoint string) (LocalModels, error) {
	var targets []string
	var out LocalModels
	add := func(e string) {
		e = strings.TrimSuffix(strings.TrimSpace(e), "/")
		for _, t := range targets {
			if t == e {
				return
			}
		}
		targets = append(targets, e)
	}
	if e := strings.TrimSpace(formEndpoint); e != "" {
		if checkLocalEndpoint(e, false) == nil {
			add(e)
		} else {
			out.Skipped = "the endpoint in the form is not this machine, so it was not asked"
		}
	}
	add(localDiscoveryTarget)

	var reasons []string
	for _, target := range targets {
		ids, err := listLocalModels(target)
		u, _ := url.Parse(target)
		host := ""
		if u != nil {
			host = u.Host
		}
		if err != nil {
			reasons = append(reasons, host+" ("+err.Error()+")")
			continue
		}
		out.Endpoint, out.Host, out.Models = target, host, ids
		return out, nil
	}
	return out, fmt.Errorf("no local model server gave a model list: %s. Start your local model server, such as Ollama, and try again", strings.Join(reasons, "; "))
}

// listLocalModels asks one endpoint for its models, defensively: the answer is limited to 1 MB, a status other than 200 or an
// answer that is not a model list is an error that does not repeat the body, and each id is cleaned and the list is capped.
func listLocalModels(endpoint string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), discoveryTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(endpoint, "/")+"/models", nil)
	if err != nil {
		return nil, err
	}
	resp, err := newAIClient(discoveryTimeout).Do(req)
	if err != nil {
		return nil, fmt.Errorf("no answer")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("answered %d, not a model list", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, discoveryBodyLimit+1))
	if err != nil || len(body) > discoveryBodyLimit {
		return nil, fmt.Errorf("the answer is not a model list: it is too large or cut off")
	}
	var listing struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &listing); err != nil || listing.Data == nil {
		return nil, fmt.Errorf("answered, but not with a model list") // not JSON, or JSON with no data field
	}
	seen := map[string]bool{}
	var ids []string
	for _, m := range listing.Data {
		id := strings.TrimSpace(strings.Map(func(r rune) rune {
			if unicode.IsControl(r) {
				return -1
			}
			return r
		}, m.ID))
		if id == "" {
			continue
		}
		id = modelID(id, "")
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if len(ids) > maxDiscoveredModels {
		ids = ids[:maxDiscoveredModels]
	}
	return ids, nil
}
