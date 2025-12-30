package postman

type OpenAPISpec struct {
	OpenAPI    string                        `json:"openapi"`
	Swagger    string                        `json:"swagger"`
	Info       OpenAPIInfo                   `json:"info"`
	Servers    []OpenAPIServer               `json:"servers"`
	Paths      map[string]OpenAPIPathItem    `json:"paths"`
	Components *OpenAPIComponents            `json:"components"`
	Security   []map[string][]string         `json:"security"`
}

type OpenAPIInfo struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Version     string `json:"version"`
}

type OpenAPIServer struct {
	URL         string `json:"url"`
	Description string `json:"description"`
}

type OpenAPIPathItem struct {
	Get        *OpenAPIOperation   `json:"get"`
	Post       *OpenAPIOperation   `json:"post"`
	Put        *OpenAPIOperation   `json:"put"`
	Delete     *OpenAPIOperation   `json:"delete"`
	Patch      *OpenAPIOperation   `json:"patch"`
	Options    *OpenAPIOperation   `json:"options"`
	Head       *OpenAPIOperation   `json:"head"`
	Parameters []OpenAPIParameter  `json:"parameters"`
}

type OpenAPIOperation struct {
	OperationID string                `json:"operationId"`
	Summary     string                `json:"summary"`
	Description string                `json:"description"`
	Tags        []string              `json:"tags"`
	Parameters  []OpenAPIParameter    `json:"parameters"`
	RequestBody *OpenAPIRequestBody   `json:"requestBody"`
	Security    []map[string][]string `json:"security"`
}

type OpenAPIParameter struct {
	Name        string          `json:"name"`
	In          string          `json:"in"`
	Description string          `json:"description"`
	Required    bool            `json:"required"`
	Schema      *OpenAPISchema  `json:"schema"`
}

type OpenAPIRequestBody struct {
	Description string                       `json:"description"`
	Required    bool                         `json:"required"`
	Content     map[string]OpenAPIMediaType  `json:"content"`
}

type OpenAPIMediaType struct {
	Schema  *OpenAPISchema `json:"schema"`
	Example interface{}    `json:"example"`
}

type OpenAPISchema struct {
	Type    string      `json:"type"`
	Format  string      `json:"format"`
	Example interface{} `json:"example"`
}

type OpenAPIComponents struct {
	SecuritySchemes map[string]OpenAPISecurityScheme `json:"securitySchemes"`
}

type OpenAPISecurityScheme struct {
	Type        string `json:"type"`
	Name        string `json:"name"`
	In          string `json:"in"`
	Scheme      string `json:"scheme"`
	Description string `json:"description"`
}
