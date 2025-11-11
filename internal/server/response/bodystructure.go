package response

import (
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"strings"
)

// BuildBodyStructure builds a BODYSTRUCTURE response from a raw message
// BODYSTRUCTURE format follows RFC 3501 Section 7.4.2
func BuildBodyStructure(rawMsg string) string {
	// Extract Content-Type header
	contentType := extractHeader(rawMsg, "Content-Type")
	if contentType == "" {
		contentType = "text/plain; charset=us-ascii"
	}

	// Parse content type and parameters
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = "text/plain"
		params = map[string]string{"charset": "us-ascii"}
	}

	// Split media type into type and subtype
	typeParts := strings.SplitN(mediaType, "/", 2)
	mainType := "TEXT"
	subType := "PLAIN"
	if len(typeParts) == 2 {
		mainType = strings.ToUpper(typeParts[0])
		subType = strings.ToUpper(typeParts[1])
	}

	// Handle multipart messages separately
	if strings.HasPrefix(strings.ToLower(mediaType), "multipart/") {
		boundary := params["boundary"]
		if boundary != "" {
			return buildMultipartBodyStructure(rawMsg, mainType, subType, boundary)
		}
	}

	// For non-multipart messages, return basic body structure
	// Get message body
	headerEnd := strings.Index(rawMsg, "\r\n\r\n")
	if headerEnd == -1 {
		headerEnd = strings.Index(rawMsg, "\n\n")
	}
	body := ""
	if headerEnd != -1 {
		body = rawMsg[headerEnd+4:]
	}

	// Get encoding
	encoding := extractHeader(rawMsg, "Content-Transfer-Encoding")
	if encoding == "" {
		encoding = "7BIT"
	}
	encoding = strings.ToUpper(encoding)

	// Build parameters list
	paramList := buildParamList(params)

	// Get Content-ID and Content-Description
	contentID := extractHeader(rawMsg, "Content-ID")
	contentDesc := extractHeader(rawMsg, "Content-Description")

	// Count lines for text types
	lines := 0
	if mainType == "TEXT" {
		lines = strings.Count(body, "\n")
	}

	// Format: (type subtype (params) id description encoding size [lines for text])
	if mainType == "TEXT" {
		return fmt.Sprintf("BODYSTRUCTURE (%s %s %s %s %s %s %d %d)",
			QuoteOrNIL(mainType),
			QuoteOrNIL(subType),
			paramList,
			QuoteOrNIL(contentID),
			QuoteOrNIL(contentDesc),
			QuoteOrNIL(encoding),
			len(body),
			lines,
		)
	}

	return fmt.Sprintf("BODYSTRUCTURE (%s %s %s %s %s %s %d)",
		QuoteOrNIL(mainType),
		QuoteOrNIL(subType),
		paramList,
		QuoteOrNIL(contentID),
		QuoteOrNIL(contentDesc),
		QuoteOrNIL(encoding),
		len(body),
	)
}

// buildMultipartBodyStructure builds BODYSTRUCTURE for multipart messages
func buildMultipartBodyStructure(rawMsg, mainType, subType, boundary string) string {
	// Get the body part (after headers)
	headerEnd := strings.Index(rawMsg, "\r\n\r\n")
	if headerEnd == -1 {
		headerEnd = strings.Index(rawMsg, "\n\n")
		if headerEnd == -1 {
			// No headers found, return fallback
			return buildFallbackBodyStructure(mainType, subType)
		}
		headerEnd += 2
	} else {
		headerEnd += 4
	}
	body := rawMsg[headerEnd:]

	// Normalize line endings for multipart parsing
	if !strings.Contains(body, "\r\n") {
		body = strings.ReplaceAll(body, "\n", "\r\n")
	}

	// Parse multipart body using Go's multipart.Reader
	var parts []string
	mr := multipart.NewReader(strings.NewReader(body), boundary)

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Fall back to manual parsing if multipart.Reader fails completely
			if len(parts) == 0 {
				return buildFallbackMultipartBodyStructure(rawMsg, mainType, subType, boundary)
			}
			break
		}

		// Read part content
		partContent, err := io.ReadAll(part)
		if err != nil {
			continue
		}

		// Build part headers
		var partHeaders strings.Builder
		for key, values := range part.Header {
			for _, value := range values {
				partHeaders.WriteString(fmt.Sprintf("%s: %s\r\n", key, value))
			}
		}
		partHeaders.WriteString("\r\n")
		fullPart := partHeaders.String() + string(partContent)

		// Build BODYSTRUCTURE for this part
		partStruct := buildPartStructure(fullPart)
		parts = append(parts, partStruct)
	}

	if len(parts) == 0 {
		// Fallback to manual parsing if multipart.Reader failed
		return buildFallbackMultipartBodyStructure(rawMsg, mainType, subType, boundary)
	}

	// Multipart BODYSTRUCTURE format: BODYSTRUCTURE ((part1)(part2)... subtype)
	// Note: Each part is already a complete structure without BODYSTRUCTURE keyword
	return fmt.Sprintf("BODYSTRUCTURE (%s %s)", strings.Join(parts, " "), QuoteOrNIL(subType))
}

// buildPartStructure builds BODYSTRUCTURE for a single MIME part
func buildPartStructure(partMsg string) string {
	// Extract Content-Type
	contentType := extractHeader(partMsg, "Content-Type")
	if contentType == "" {
		contentType = "text/plain; charset=us-ascii"
	}

	// Parse media type
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = "text/plain"
		params = map[string]string{"charset": "us-ascii"}
	}

	typeParts := strings.SplitN(mediaType, "/", 2)
	mainType := "TEXT"
	subType := "PLAIN"
	if len(typeParts) == 2 {
		mainType = strings.ToUpper(typeParts[0])
		subType = strings.ToUpper(typeParts[1])
	}

	// Get encoding
	encoding := extractHeader(partMsg, "Content-Transfer-Encoding")
	if encoding == "" {
		encoding = "7BIT"
	}
	encoding = strings.ToUpper(encoding)

	// Get body
	headerEnd := strings.Index(partMsg, "\r\n\r\n")
	if headerEnd == -1 {
		headerEnd = strings.Index(partMsg, "\n\n")
		if headerEnd == -1 {
			headerEnd = 0
		} else {
			headerEnd += 2
		}
	} else {
		headerEnd += 4
	}
	body := ""
	if headerEnd < len(partMsg) {
		body = partMsg[headerEnd:]
	}

	// Build parameters
	paramList := buildParamList(params)

	// Get Content-ID and Content-Description
	contentID := extractHeader(partMsg, "Content-ID")
	contentDesc := extractHeader(partMsg, "Content-Description")

	// Get Content-Disposition and filename
	disposition := extractHeader(partMsg, "Content-Disposition")
	var dispList string
	if disposition != "" {
		dispType, dispParams, _ := mime.ParseMediaType(disposition)
		dispParamList := buildParamList(dispParams)
		dispList = fmt.Sprintf("(%s %s)", QuoteOrNIL(strings.ToUpper(dispType)), dispParamList)
	} else {
		dispList = "NIL"
	}

	// Count lines for text types
	lines := 0
	if mainType == "TEXT" {
		lines = strings.Count(body, "\n")
		return fmt.Sprintf("(%s %s %s %s %s %s %d %d NIL %s NIL)",
			QuoteOrNIL(mainType),
			QuoteOrNIL(subType),
			paramList,
			QuoteOrNIL(contentID),
			QuoteOrNIL(contentDesc),
			QuoteOrNIL(encoding),
			len(body),
			lines,
			dispList,
		)
	}

	return fmt.Sprintf("(%s %s %s %s %s %s %d NIL %s NIL)",
		QuoteOrNIL(mainType),
		QuoteOrNIL(subType),
		paramList,
		QuoteOrNIL(contentID),
		QuoteOrNIL(contentDesc),
		QuoteOrNIL(encoding),
		len(body),
		dispList,
	)
}

// buildParamList builds parameter list for BODYSTRUCTURE
func buildParamList(params map[string]string) string {
	if len(params) == 0 {
		return "NIL"
	}

	var paramPairs []string
	for key, value := range params {
		paramPairs = append(paramPairs, fmt.Sprintf("%s %s",
			QuoteOrNIL(strings.ToUpper(key)),
			QuoteOrNIL(value)))
	}

	return fmt.Sprintf("(%s)", strings.Join(paramPairs, " "))
}

// buildFallbackBodyStructure builds a simple fallback BODYSTRUCTURE
// Used when message parsing fails
func buildFallbackBodyStructure(mainType, subType string) string {
	return fmt.Sprintf("BODYSTRUCTURE (%s %s NIL NIL NIL \"7BIT\" 0)",
		QuoteOrNIL(mainType), QuoteOrNIL(subType))
}

// buildFallbackMultipartBodyStructure manually parses multipart messages
// when multipart.Reader fails. This is a fallback parser.
func buildFallbackMultipartBodyStructure(rawMsg, mainType, subType, boundary string) string {
	// Get the body part (after headers)
	headerEnd := strings.Index(rawMsg, "\r\n\r\n")
	if headerEnd == -1 {
		headerEnd = strings.Index(rawMsg, "\n\n")
		if headerEnd == -1 {
			return buildFallbackBodyStructure(mainType, subType)
		}
		headerEnd += 2
	} else {
		headerEnd += 4
	}
	body := rawMsg[headerEnd:]

	// Normalize line endings
	if !strings.Contains(body, "\r\n") {
		body = strings.ReplaceAll(body, "\n", "\r\n")
	}

	// Manually split by boundary
	boundaryMarker := "--" + boundary
	closeBoundary := "--" + boundary + "--"

	// Split the body into parts
	partSections := strings.Split(body, boundaryMarker)
	var parts []string

	for i, section := range partSections {
		// Skip the preamble (before first boundary) and epilogue (after final boundary)
		if i == 0 || strings.TrimSpace(section) == "" {
			continue
		}

		// Check if this is the closing boundary
		if strings.HasPrefix(strings.TrimSpace(section), "--") {
			break
		}

		// Remove the closing boundary if present
		section = strings.TrimPrefix(section, "\r\n")
		section = strings.TrimPrefix(section, "\n")

		// Check if this section ends with the closing boundary
		if idx := strings.Index(section, closeBoundary); idx != -1 {
			section = section[:idx]
		}

		// Parse this part's structure
		if strings.TrimSpace(section) != "" {
			partStruct := buildPartStructure(section)
			parts = append(parts, partStruct)
		}
	}

	if len(parts) == 0 {
		// Still no parts found, return simple fallback
		return buildFallbackBodyStructure(mainType, subType)
	}

	// Return the multipart structure
	return fmt.Sprintf("BODYSTRUCTURE (%s %s)", strings.Join(parts, " "), QuoteOrNIL(subType))
}
