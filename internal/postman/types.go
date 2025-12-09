package postman

type Collection struct {
	Info  Info   `json:"info"`
	Items []Item `json:"item"`
}

type Info struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Schema      string `json:"schema"`
}

type Item struct {
	Name        string   `json:"name"`
	Request     *Request `json:"request,omitempty"`
	Items       []Item   `json:"item,omitempty"`
	Description string   `json:"description,omitempty"`
}

type Request struct {
	Method string   `json:"method"`
	Header []Header `json:"header"`
	Body   *Body    `json:"body,omitempty"`
	URL    URL      `json:"url"`
}

type Header struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Type  string `json:"type,omitempty"`
}

type Body struct {
	Mode string `json:"mode"`
	Raw  string `json:"raw,omitempty"`
}

type URL struct {
	Raw  string   `json:"raw"`
	Host []string `json:"host,omitempty"`
	Path []string `json:"path,omitempty"`
}

func (i *Item) IsFolder() bool {
	return i.Request == nil && len(i.Items) > 0
}

func (i *Item) IsRequest() bool {
	return i.Request != nil
}

type Environment struct {
	ID     string        `json:"id"`
	Name   string        `json:"name"`
	Values []EnvVariable `json:"values"`
}

type EnvVariable struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
	Type    string `json:"type"`
}
