package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SkillMeta represents skill metadata from a remote registry
type SkillMeta struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	InstallURL  string `json:"install_url"`
	Version     string `json:"version"`
	Author      string `json:"author"`
}

// SkillRegistryClient is the interface for searching skills from a remote registry
type SkillRegistryClient interface {
	Search(ctx context.Context, query string) ([]SkillMeta, error)
	GetByID(ctx context.Context, id string) (*SkillMeta, error)
}

// RemoteRegistry implements SkillRegistryClient by calling an external JSON API
type RemoteRegistry struct {
	baseURL    string
	httpClient *http.Client
}

// NewRemoteRegistry creates a new remote registry client
func NewRemoteRegistry(baseURL string) *RemoteRegistry {
	return &RemoteRegistry{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// searchResponse is the expected JSON response from registry API
type searchResponse struct {
	Skills []SkillMeta `json:"skills"`
}

// clawHubSkill is ClawHub-style skill format (tagline, category, homepage as alternatives)
type clawHubSkill struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	InstallURL  string `json:"install_url"`
	Tagline     string `json:"tagline"`
	Homepage    string `json:"homepage"`
	Version     string `json:"version"`
	Author      string `json:"author"`
}

// Search searches for skills by keyword
func (r *RemoteRegistry) Search(ctx context.Context, query string) ([]SkillMeta, error) {
	// Try search endpoint first: {base}/search?q=...
	searchURL := r.baseURL + "/search?q=" + url.QueryEscape(query)
	skills, err := r.fetchSkills(ctx, searchURL)
	if err == nil {
		return skills, nil
	}

	// Fallback: fetch all and filter client-side (for static JSON index)
	allURL := r.baseURL
	if !strings.Contains(r.baseURL, "?") {
		allURL = r.baseURL + "/skills"
	}
	allSkills, err := r.fetchSkills(ctx, allURL)
	if err != nil {
		return nil, err
	}

	query = strings.ToLower(query)
	var filtered []SkillMeta
	for _, s := range allSkills {
		if strings.Contains(strings.ToLower(s.Name), query) ||
			strings.Contains(strings.ToLower(s.Description), query) ||
			strings.Contains(strings.ToLower(s.ID), query) {
			filtered = append(filtered, s)
		}
	}
	return filtered, nil
}

// GetByID retrieves a skill by ID from the registry
func (r *RemoteRegistry) GetByID(ctx context.Context, id string) (*SkillMeta, error) {
	// Try direct lookup endpoint first: {base}/skills/{id}
	directURL := r.baseURL + "/skills/" + url.PathEscape(id)
	skills, err := r.fetchSkills(ctx, directURL)
	if err == nil && len(skills) > 0 {
		return &skills[0], nil
	}

	// Fallback: search and filter by exact ID
	allSkills, err := r.fetchSkills(ctx, r.baseURL+"/skills")
	if err != nil {
		return nil, err
	}
	for _, s := range allSkills {
		if s.ID == id {
			return &s, nil
		}
	}
	return nil, fmt.Errorf("skill %s not found in registry", id)
}

func (r *RemoteRegistry) fetchSkills(ctx context.Context, u string) ([]SkillMeta, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("registry request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned status %d", resp.StatusCode)
	}

	// Decode: support both {"skills": [...]} and single {...} object
	var raw json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to decode registry response: %w", err)
	}

	// Try {"skills": [...]} format first
	var result searchResponse
	if err := json.Unmarshal(raw, &result); err == nil && len(result.Skills) > 0 {
		return result.Skills, nil
	}

	// Try ClawHub-style format: {"skills": [{"tagline": "...", "homepage": "..."}]}
	var clawHubResult struct {
		Skills []clawHubSkill `json:"skills"`
	}
	if err := json.Unmarshal(raw, &clawHubResult); err == nil && len(clawHubResult.Skills) > 0 {
		skills := make([]SkillMeta, 0, len(clawHubResult.Skills))
		for _, s := range clawHubResult.Skills {
			meta := SkillMeta{ID: s.ID, Name: s.Name, Description: s.Description, InstallURL: s.InstallURL, Version: s.Version, Author: s.Author}
			if meta.Description == "" && s.Tagline != "" {
				meta.Description = s.Tagline
			}
			if meta.InstallURL == "" && s.Homepage != "" {
				meta.InstallURL = s.Homepage
			}
			if meta.ID != "" || meta.Name != "" {
				skills = append(skills, meta)
			}
		}
		if len(skills) > 0 {
			return skills, nil
		}
	}

	// Try single skill object (for /skills/{id} endpoint)
	var single SkillMeta
	if err := json.Unmarshal(raw, &single); err == nil && single.ID != "" {
		return []SkillMeta{single}, nil
	}

	// Try single ClawHub-style skill
	var singleClaw clawHubSkill
	if err := json.Unmarshal(raw, &singleClaw); err == nil && (singleClaw.ID != "" || singleClaw.Name != "") {
		meta := SkillMeta{ID: singleClaw.ID, Name: singleClaw.Name, Description: singleClaw.Description, InstallURL: singleClaw.InstallURL, Version: singleClaw.Version, Author: singleClaw.Author}
		if meta.Description == "" && singleClaw.Tagline != "" {
			meta.Description = singleClaw.Tagline
		}
		if meta.InstallURL == "" && singleClaw.Homepage != "" {
			meta.InstallURL = singleClaw.Homepage
		}
		return []SkillMeta{meta}, nil
	}

	return result.Skills, nil
}
