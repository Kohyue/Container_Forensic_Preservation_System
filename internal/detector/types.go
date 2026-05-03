package detector

// FalcoAlert is the JSON payload Falco sends to a webhook output.
// Ref: https://falco.org/docs/alerts/formatting/
type FalcoAlert struct {
	Output   string            `json:"output"`
	Priority string            `json:"priority"`
	Rule     string            `json:"rule"`
	Time     string            `json:"time"`
	Fields   map[string]string `json:"output_fields"`
}

// ContainerID extracts the container ID from Falco output fields.
func (a *FalcoAlert) ContainerID() string {
	for _, key := range []string{"container.id", "container_id"} {
		if v := a.Fields[key]; v != "" && v != "<NA>" && v != "host" {
			return v
		}
	}
	return ""
}

// ContainerName extracts the container name from Falco output fields.
func (a *FalcoAlert) ContainerName() string {
	for _, key := range []string{"container.name", "container_name"} {
		if v := a.Fields[key]; v != "" && v != "<NA>" {
			return v
		}
	}
	return ""
}

// priorityRank maps Falco priority strings to numeric severity (higher = more severe).
var priorityRank = map[string]int{
	"debug":         0,
	"informational": 1,
	"notice":        2,
	"warning":       3,
	"error":         4,
	"critical":      5,
	"alert":         6,
	"emergency":     7,
}

// MeetsMinPriority returns true if the alert priority is at least minPriority.
func (a *FalcoAlert) MeetsMinPriority(minPriority string) bool {
	alertRank, ok := priorityRank[a.Priority]
	if !ok {
		return true // unknown priority: let it through
	}
	minRank, ok := priorityRank[minPriority]
	if !ok {
		return true
	}
	return alertRank >= minRank
}
