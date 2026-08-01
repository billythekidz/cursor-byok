package promptengine

// ContentPart represents a structured content block within a message.
type ContentPart struct {
	Type  string        `json:"type"`
	Text  string        `json:"text,omitempty"`
	Image *ImageContent `json:"image,omitempty"`
}

// ImageContent represents an image carried in a message.
type ImageContent struct {
	MIMEType string `json:"mime_type,omitempty"`
	Path     string `json:"path,omitempty"`
	Data     []byte `json:"data,omitempty"`
}
