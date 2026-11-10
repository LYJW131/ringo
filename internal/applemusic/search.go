package applemusic

import "strings"

// Suggestions of what a search being typed may be after.

func (s *Source) suggest(term string) ([]string, error) {
	s.warm()
	if strings.TrimSpace(term) == "" {
		return []string{}, nil
	}
	var b struct {
		Results struct {
			Suggestions []struct {
				DisplayTerm string `json:"displayTerm"`
				SearchTerm  string `json:"searchTerm"`
			} `json:"suggestions"`
		} `json:"results"`
	}
	if err := s.catalogInto("/search/suggestions", params{"term": term, "kinds": "terms", "limit": "10"}, &b); err != nil {
		return nil, err
	}
	out := []string{}
	for _, x := range b.Results.Suggestions {
		if t := orT(x.DisplayTerm, x.SearchTerm); t != "" {
			out = append(out, t)
		}
	}
	return out, nil
}
