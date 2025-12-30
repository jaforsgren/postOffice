package postman

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	defaultBaseURL   = "https://api.example.com"
	postmanSchemaURL = "https://schema.getpostman.com/json/collection/v2.1.0/collection.json"
)

func ConvertOpenAPIToCollection(spec *OpenAPISpec) (*Collection, error) {
	if spec == nil {
		return nil, fmt.Errorf("spec cannot be nil")
	}

	baseURL := getBaseURL(spec.Servers)

	var securitySchemes map[string]OpenAPISecurityScheme
	if spec.Components != nil {
		securitySchemes = spec.Components.SecuritySchemes
	}

	items := convertPaths(spec.Paths, baseURL, securitySchemes, spec.Security)

	collection := &Collection{
		Info:  convertInfo(spec.Info),
		Items: items,
	}

	return collection, nil
}

func convertInfo(oaInfo OpenAPIInfo) Info {
	return Info{
		Name:        oaInfo.Title,
		Description: oaInfo.Description,
		Schema:      postmanSchemaURL,
	}
}

func getBaseURL(servers []OpenAPIServer) string {
	if len(servers) > 0 && servers[0].URL != "" {
		return templateServerVariables(servers[0].URL)
	}
	return defaultBaseURL
}

func templateServerVariables(serverURL string) string {
	re := regexp.MustCompile(`\{([^}]+)\}`)
	return re.ReplaceAllString(serverURL, "{{$1}}")
}

func convertPaths(paths map[string]OpenAPIPathItem, baseURL string, securitySchemes map[string]OpenAPISecurityScheme, globalSecurity []map[string][]string) []Item {
	var items []Item

	sortedPaths := make([]string, 0, len(paths))
	for path := range paths {
		sortedPaths = append(sortedPaths, path)
	}
	sort.Strings(sortedPaths)

	for _, path := range sortedPaths {
		pathItem := paths[path]

		operations := map[string]*OpenAPIOperation{
			"GET":     pathItem.Get,
			"POST":    pathItem.Post,
			"PUT":     pathItem.Put,
			"DELETE":  pathItem.Delete,
			"PATCH":   pathItem.Patch,
			"OPTIONS": pathItem.Options,
			"HEAD":    pathItem.Head,
		}

		for _, method := range []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS", "HEAD"} {
			op := operations[method]
			if op == nil {
				continue
			}

			allParams := append([]OpenAPIParameter{}, pathItem.Parameters...)
			allParams = append(allParams, op.Parameters...)

			item := convertOperation(method, path, op, allParams, baseURL, securitySchemes, globalSecurity)
			items = append(items, *item)
		}
	}

	return organizeByTags(items)
}

func convertOperation(method, path string, op *OpenAPIOperation, params []OpenAPIParameter, baseURL string, securitySchemes map[string]OpenAPISecurityScheme, globalSecurity []map[string][]string) *Item {
	name := op.OperationID
	if name == "" {
		name = op.Summary
	}
	if name == "" {
		name = fmt.Sprintf("%s %s", method, path)
	}

	pathParams, queryParams, headerParams := categorizeParameters(params)

	url := buildURL(baseURL, path, pathParams, queryParams)

	headers := convertParametersToHeaders(headerParams)

	security := op.Security
	if len(security) == 0 {
		security = globalSecurity
	}
	securityHeaders := convertSecurity(security, securitySchemes)
	headers = append(headers, securityHeaders...)

	var body *Body
	if op.RequestBody != nil {
		body = convertRequestBody(op.RequestBody)
	}

	description := op.Description
	if len(op.Tags) > 0 {
		description = fmt.Sprintf("[tag:%s] %s", op.Tags[0], description)
	}

	return &Item{
		Name:        name,
		Description: description,
		Request: &Request{
			Method: method,
			Header: headers,
			Body:   body,
			URL:    url,
		},
	}
}

func categorizeParameters(params []OpenAPIParameter) (pathParams, queryParams, headerParams []OpenAPIParameter) {
	for _, param := range params {
		switch param.In {
		case "path":
			pathParams = append(pathParams, param)
		case "query":
			queryParams = append(queryParams, param)
		case "header":
			headerParams = append(headerParams, param)
		}
	}
	return
}

func buildURL(baseURL, path string, pathParams, queryParams []OpenAPIParameter) URL {
	templatedPath := path
	for _, param := range pathParams {
		placeholder := "{" + param.Name + "}"
		template := "{{" + param.Name + "}}"
		templatedPath = strings.ReplaceAll(templatedPath, placeholder, template)
	}

	rawURL := baseURL + templatedPath

	if len(queryParams) > 0 {
		queryParts := make([]string, 0, len(queryParams))
		for _, param := range queryParams {
			queryParts = append(queryParts, fmt.Sprintf("%s={{%s}}", param.Name, param.Name))
		}
		rawURL += "?" + strings.Join(queryParts, "&")
	}

	return URL{
		Raw: rawURL,
	}
}

func convertParametersToHeaders(headerParams []OpenAPIParameter) []Header {
	headers := make([]Header, 0, len(headerParams))
	for _, param := range headerParams {
		headers = append(headers, Header{
			Key:   param.Name,
			Value: fmt.Sprintf("{{%s}}", param.Name),
		})
	}
	return headers
}

func convertSecurity(opSecurity []map[string][]string, securitySchemes map[string]OpenAPISecurityScheme) []Header {
	var headers []Header

	for _, secReq := range opSecurity {
		for schemeName := range secReq {
			scheme, exists := securitySchemes[schemeName]
			if !exists {
				continue
			}

			switch scheme.Type {
			case "apiKey":
				if scheme.In == "header" {
					headers = append(headers, Header{
						Key:   scheme.Name,
						Value: fmt.Sprintf("{{%s}}", schemeName),
					})
				}

			case "http":
				if scheme.Scheme == "bearer" {
					headers = append(headers, Header{
						Key:   "Authorization",
						Value: fmt.Sprintf("Bearer {{%s}}", schemeName),
					})
				} else if scheme.Scheme == "basic" {
					headers = append(headers, Header{
						Key:   "Authorization",
						Value: fmt.Sprintf("Basic {{%s}}", schemeName),
					})
				}
			}
		}
	}

	return headers
}

func convertRequestBody(reqBody *OpenAPIRequestBody) *Body {
	if reqBody == nil {
		return nil
	}

	var contentType string
	var mediaType OpenAPIMediaType

	if mt, ok := reqBody.Content["application/json"]; ok {
		contentType = "application/json"
		mediaType = mt
	} else {
		for ct, mt := range reqBody.Content {
			contentType = ct
			mediaType = mt
			break
		}
	}

	var rawBody string
	if mediaType.Example != nil {
		jsonBytes, err := json.MarshalIndent(mediaType.Example, "", "  ")
		if err == nil {
			rawBody = string(jsonBytes)
		}
	} else if mediaType.Schema != nil && mediaType.Schema.Example != nil {
		jsonBytes, err := json.MarshalIndent(mediaType.Schema.Example, "", "  ")
		if err == nil {
			rawBody = string(jsonBytes)
		}
	} else {
		rawBody = "{}"
	}

	if contentType == "" {
		return &Body{
			Mode: "raw",
			Raw:  rawBody,
		}
	}

	return &Body{
		Mode: "raw",
		Raw:  rawBody,
	}
}

func organizeByTags(items []Item) []Item {
	tagMap := make(map[string][]Item)
	var untagged []Item

	for _, item := range items {
		tag := extractFirstTag(item)
		if tag != "" {
			tagMap[tag] = append(tagMap[tag], item)
		} else {
			untagged = append(untagged, item)
		}
	}

	if len(tagMap) == 0 {
		return items
	}

	var organized []Item

	sortedTags := make([]string, 0, len(tagMap))
	for tag := range tagMap {
		sortedTags = append(sortedTags, tag)
	}
	sort.Strings(sortedTags)

	for _, tag := range sortedTags {
		taggedItems := tagMap[tag]
		for i := range taggedItems {
			taggedItems[i].Description = strings.TrimPrefix(taggedItems[i].Description, fmt.Sprintf("[tag:%s] ", tag))
		}

		organized = append(organized, Item{
			Name:  tag,
			Items: taggedItems,
		})
	}

	organized = append(organized, untagged...)

	return organized
}

func extractFirstTag(item Item) string {
	if !strings.HasPrefix(item.Description, "[tag:") {
		return ""
	}

	endIdx := strings.Index(item.Description, "]")
	if endIdx == -1 {
		return ""
	}

	tag := item.Description[5:endIdx]
	return tag
}
