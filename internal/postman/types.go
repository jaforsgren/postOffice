package postman

import (
	"encoding/json"
	"strings"
)

type Collection struct {
	Info      Info       `json:"info"`
	Items     []Item     `json:"item"`
	Variables []Variable `json:"variable,omitempty"`
	Events    []Event    `json:"event,omitempty"`
	unknown   unknownFields
}

type Info struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Schema      string `json:"schema"`
	unknown     unknownFields
}

type Item struct {
	ID                      string                 `json:"id,omitempty"`
	Name                    string                 `json:"name"`
	Request                 *Request               `json:"request,omitempty"`
	Items                   []Item                 `json:"item,omitempty"`
	Description             string                 `json:"description,omitempty"`
	Variables               []Variable             `json:"variable,omitempty"`
	Events                  []Event                `json:"event,omitempty"`
	ProtocolProfileBehavior map[string]interface{} `json:"protocolProfileBehavior,omitempty"`
	unknown                 unknownFields
}

type Request struct {
	Method  string   `json:"method"`
	Header  []Header `json:"header"`
	Body    *Body    `json:"body,omitempty"`
	URL     URL      `json:"url"`
	unknown unknownFields
}

type Header struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Type    string `json:"type,omitempty"`
	unknown unknownFields
}

const (
	BodyModeRaw       = "raw"
	BodyModeFormData  = "formdata"
	FormParamTypeFile = "file"
)

type Body struct {
	Mode     string      `json:"mode"`
	Raw      string      `json:"raw,omitempty"`
	FormData []FormParam `json:"formdata,omitempty"`
	unknown  unknownFields
}

// FormParam is one field of a formdata body. Src names the file(s) of a file
// field: Postman writes one path as a string and several as an array.
type FormParam struct {
	Key         string          `json:"key"`
	Value       string          `json:"value,omitempty"`
	Type        string          `json:"type,omitempty"`
	Src         json.RawMessage `json:"src,omitempty"`
	Disabled    bool            `json:"disabled,omitempty"`
	ContentType string          `json:"contentType,omitempty"`
	unknown     unknownFields
}

type URL struct {
	Raw      string   `json:"raw"`
	Host     []string `json:"host,omitempty"`
	Path     []string `json:"path,omitempty"`
	Port     string   `json:"port,omitempty"`
	Protocol string   `json:"protocol,omitempty"`
	unknown  unknownFields
}

func (i *Item) IsFolder() bool {
	return i.Request == nil && len(i.Items) > 0
}

func (i *Item) IsRequest() bool {
	return i.Request != nil
}

func (i *Item) IsGRPC() bool {
	if i.Request == nil {
		return false
	}
	lower := strings.ToLower(i.Request.URL.Raw)
	return strings.EqualFold(i.Request.Method, "GRPC") ||
		strings.HasPrefix(lower, "grpc://") ||
		strings.HasPrefix(lower, "grpcs://")
}

func (i *Item) IsServiceBus() bool {
	if i.Request == nil {
		return false
	}
	lower := strings.ToLower(i.Request.URL.Raw)
	return strings.HasPrefix(lower, "asb://")
}

func (i *Item) IsAMQP() bool {
	if i.Request == nil {
		return false
	}
	lower := strings.ToLower(i.Request.URL.Raw)
	return strings.HasPrefix(lower, "amqp://") || strings.HasPrefix(lower, "amqps://")
}

type Environment struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Values  []EnvVariable `json:"values"`
	unknown unknownFields
}

// EnvVariable and Variable keep a choice variable's chosen value in Value,
// so tools that don't know Choices still read the chosen one.
type EnvVariable struct {
	Key     string   `json:"key"`
	Value   string   `json:"value"`
	Choices []string `json:"choices,omitempty"`
	Enabled bool     `json:"enabled"`
	Type    string   `json:"type"`
	unknown unknownFields
}

type Variable struct {
	Key     string   `json:"key"`
	Value   string   `json:"value"`
	Choices []string `json:"choices,omitempty"`
	Type    string   `json:"type,omitempty"`
	Enabled bool     `json:"enabled,omitempty"`
	unknown unknownFields
}

type Event struct {
	Listen  string `json:"listen"`
	Script  Script `json:"script"`
	unknown unknownFields
}

type Script struct {
	Type    string   `json:"type"`
	Exec    []string `json:"exec"`
	unknown unknownFields
}
