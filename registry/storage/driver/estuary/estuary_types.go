package estuary

import "time"

type ContentElement struct {
	Content struct {
		ID           int       `json:"id"`
		UpdatedAt    time.Time `json:"updatedAt"`
		Cid          string    `json:"cid"`
		Name         string    `json:"name"`
		UserID       int       `json:"userId"`
		Description  string    `json:"description"`
		Size         int       `json:"size"`
		Type         int       `json:"type"`
		Active       bool      `json:"active"`
		Offloaded    bool      `json:"offloaded"`
		Replication  int       `json:"replication"`
		AggregatedIn int       `json:"aggregatedIn"`
		Aggregate    bool      `json:"aggregate"`
		Pinning      bool      `json:"pinning"`
		PinMeta      string    `json:"pinMeta"`
		Failed       bool      `json:"failed"`
		Location     string    `json:"location"`
		DagSplit     bool      `json:"dagSplit"`
		SplitFrom    int       `json:"splitFrom"`
	} `json:"content"`
	AggregatedIn struct {
		ID           int       `json:"id"`
		UpdatedAt    time.Time `json:"updatedAt"`
		Cid          string    `json:"cid"`
		Name         string    `json:"name"`
		UserID       int       `json:"userId"`
		Description  string    `json:"description"`
		Size         int       `json:"size"`
		Type         int       `json:"type"`
		Active       bool      `json:"active"`
		Offloaded    bool      `json:"offloaded"`
		Replication  int       `json:"replication"`
		AggregatedIn int       `json:"aggregatedIn"`
		Aggregate    bool      `json:"aggregate"`
		Pinning      bool      `json:"pinning"`
		PinMeta      string    `json:"pinMeta"`
		Failed       bool      `json:"failed"`
		Location     string    `json:"location"`
		DagSplit     bool      `json:"dagSplit"`
		SplitFrom    int       `json:"splitFrom"`
	} `json:"aggregatedIn"`
	Selector string        `json:"selector"`
	Deals    []interface{} `json:"deals"`
}

type PinnedElement struct {
	Requestid string    `json:"requestid"`
	Status    string    `json:"status"`
	Created   time.Time `json:"created"`
	Pin       struct {
		Cid     string      `json:"cid"`
		Name    string      `json:"name"`
		Origins interface{} `json:"origins"`
		Meta    interface{} `json:"meta"`
	} `json:"pin"`
	Delegates []string    `json:"delegates"`
	Info      interface{} `json:"info"`
}

type PinningResponse struct {
	Count   int             `json:"count"`
	Results []PinnedElement `json:"results"`
}

type AddContentResponse struct {
	Cid       string   `json:"cid"`
	EstuaryID int      `json:"estuaryId"`
	Providers []string `json:"providers"`
}
