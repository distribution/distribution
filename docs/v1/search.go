package v1

type (
	SearchResult struct {
		NumberOfResults int           `json:"num_results,omitempty"`
		NumberOfPages   int           `json:"num_pages,omitempty"`
		Query           string        `json:"query,omitempty"`
		Results         []*Repository `json:"results,omitempty"`
	}
)
