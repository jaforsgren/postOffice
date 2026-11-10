package http

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"

	"postOffice/internal/postman"
)

const fallbackFileContentType = "application/octet-stream"

var ErrFormFileUnreadable = errors.New("cannot read form file")

var quoteEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`)

// buildBody returns the body to send and, for formdata, the multipart
// Content-Type (with its boundary) it must be sent with.
func buildBody(body *postman.Body, variables []postman.VariableSource) (io.Reader, string, error) {
	switch {
	case body == nil:
		return nil, "", nil
	case body.Mode == postman.BodyModeFormData:
		return buildMultipartBody(body.FormData, variables)
	case body.Raw != "":
		return bytes.NewBufferString(postman.ResolveVariables(body.Raw, variables)), "", nil
	default:
		return nil, "", nil
	}
}

func buildMultipartBody(params []postman.FormParam, variables []postman.VariableSource) (io.Reader, string, error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	for _, param := range params {
		if param.Disabled {
			continue
		}
		name := postman.ResolveVariables(param.Key, variables)
		var err error
		if param.Type == postman.FormParamTypeFile {
			err = writeFileParts(writer, name, param, variables)
		} else {
			err = writeTextPart(writer, name, postman.ResolveVariables(param.Value, variables), param.ContentType)
		}
		if err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("failed to finish multipart body: %w", err)
	}
	return &buffer, writer.FormDataContentType(), nil
}

func writeTextPart(writer *multipart.Writer, name, value, contentType string) error {
	if contentType == "" {
		return writer.WriteField(name, value)
	}
	part, err := writer.CreatePart(partHeader(name, "", contentType))
	if err != nil {
		return fmt.Errorf("failed to add form field %q: %w", name, err)
	}
	_, err = io.WriteString(part, value)
	return err
}

// writeFileParts adds one part per file; a file field with no file picked adds nothing, as in Postman.
func writeFileParts(writer *multipart.Writer, name string, param postman.FormParam, variables []postman.VariableSource) error {
	paths, err := param.FilePaths()
	if err != nil {
		return err
	}
	for _, path := range paths {
		resolved, err := postman.ExpandPath(postman.ResolveVariables(path, variables))
		if err != nil {
			return fmt.Errorf("%w %q: %v", ErrFormFileUnreadable, path, err)
		}
		content, err := os.ReadFile(resolved)
		if err != nil {
			return fmt.Errorf("%w %q: %v", ErrFormFileUnreadable, resolved, err)
		}
		contentType := param.ContentType
		if contentType == "" {
			contentType = fileContentType(resolved)
		}
		part, err := writer.CreatePart(partHeader(name, filepath.Base(resolved), contentType))
		if err != nil {
			return fmt.Errorf("failed to add form file %q: %w", resolved, err)
		}
		if _, err := part.Write(content); err != nil {
			return fmt.Errorf("failed to add form file %q: %w", resolved, err)
		}
	}
	return nil
}

func partHeader(name, fileName, contentType string) textproto.MIMEHeader {
	disposition := fmt.Sprintf(`form-data; name="%s"`, quoteEscaper.Replace(name))
	if fileName != "" {
		disposition += fmt.Sprintf(`; filename="%s"`, quoteEscaper.Replace(fileName))
	}
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", disposition)
	header.Set("Content-Type", contentType)
	return header
}

func fileContentType(path string) string {
	if detected := mime.TypeByExtension(filepath.Ext(path)); detected != "" {
		return detected
	}
	return fallbackFileContentType
}
