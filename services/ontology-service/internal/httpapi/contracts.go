package httpapi

// HealthResponse is the small JSON body returned by GET /healthz.
type HealthResponse struct {
	OK bool `json:"ok"` // OK reports whether the ontology-service process can answer requests.

	Service string `json:"service"` // Service identifies which local Cubicle service answered the check.
}
