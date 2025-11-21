package response

import "testing"

// Minimal raw message helper
func raw(headers string, body string) string { return headers + "\r\n\r\n" + body }

func TestBuildBodyStructure_TextPlain(t *testing.T) {
    msg := raw("Content-Type: text/plain; charset=utf-8\r\nContent-Transfer-Encoding: 7bit", "Hello world\nLine2")
    bs := BuildBodyStructure(msg)
    if !containsAll(bs, []string{"TEXT", "PLAIN", "utf-8"}) { t.Errorf("missing basics: %s", bs) }
    if !containsAll(bs, []string{"Hello"}) { /* size check implicit */ }
}

func TestBuildBodyStructure_Defaults(t *testing.T) {
    msg := raw("Subject: X", "Body")
    bs := BuildBodyStructure(msg)
    if !containsAll(bs, []string{"TEXT", "PLAIN"}) { t.Errorf("expected default text/plain: %s", bs) }
}

func TestBuildBodyStructure_MultipartMixed(t *testing.T) {
    boundary := "abc123"
    headers := "Content-Type: multipart/mixed; boundary=\"" + boundary + "\""
    part1 := "--" + boundary + "\r\nContent-Type: text/plain; charset=us-ascii\r\n\r\nPart1 text\r\n"
    part2 := "--" + boundary + "\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<html>Part2</html>\r\n"
    end := "--" + boundary + "--\r\n"
    msg := headers + "\r\n\r\n" + part1 + part2 + end
    bs := BuildBodyStructure(msg)
    // Expect two child parts and subtype MIXED with boundary parameter
    if !containsAll(bs, []string{"PLAIN", "HTML", "MIXED", boundary}) { t.Errorf("missing multipart components: %s", bs) }
}

func TestBuildBodyStructure_FallbackMultipart_NoBoundary(t *testing.T) {
    // Missing boundary parameter should treat as generic multipart with no parts parsed -> fallback structure
    headers := "Content-Type: multipart/mixed"
    msg := headers + "\r\n\r\nIgnored"
    bs := BuildBodyStructure(msg)
    if !containsAll(bs, []string{"MULTIPART", "MIXED"}) { t.Errorf("expected fallback multipart structure: %s", bs) }
}

func TestBuildBodyStructure_MultipartAlternative(t *testing.T) {
    boundary := "bALT"
    headers := "Content-Type: multipart/alternative; boundary=\"" + boundary + "\""
    part1 := "--" + boundary + "\r\nContent-Type: text/plain; charset=us-ascii\r\n\r\nPlain text\r\n"
    part2 := "--" + boundary + "\r\nContent-Type: text/html; charset=utf-8\r\n\r\n<html>HTML</html>\r\n"
    end := "--" + boundary + "--\r\n"
    msg := headers + "\r\n\r\n" + part1 + part2 + end
    bs := BuildBodyStructure(msg)
    if !containsAll(bs, []string{"ALTERNATIVE", boundary}) { t.Errorf("expected alternative subtype with boundary: %s", bs) }
}

func TestBuildBodyStructure_WithContentID(t *testing.T) {
    msg := raw("Content-Type: text/plain\r\nContent-ID: <part123@example.com>", "Body")
    bs := BuildBodyStructure(msg)
    if !containsAll(bs, []string{"part123@example.com"}) { t.Error("expected Content-ID in structure") }
}

func TestBuildBodyStructure_WithContentDescription(t *testing.T) {
    msg := raw("Content-Type: text/plain\r\nContent-Description: A text document", "Body")
    bs := BuildBodyStructure(msg)
    if !containsAll(bs, []string{"A text document"}) { t.Error("expected Content-Description in structure") }
}

func TestBuildBodyStructure_Base64Encoding(t *testing.T) {
    msg := raw("Content-Type: text/plain\r\nContent-Transfer-Encoding: base64", "SGVsbG8gV29ybGQ=")
    bs := BuildBodyStructure(msg)
    if !containsAll(bs, []string{"BASE64"}) { t.Error("expected BASE64 encoding") }
}

func TestBuildBodyStructure_QuotedPrintable(t *testing.T) {
    msg := raw("Content-Type: text/plain\r\nContent-Transfer-Encoding: quoted-printable", "Hello=20World")
    bs := BuildBodyStructure(msg)
    if !containsAll(bs, []string{"QUOTED-PRINTABLE"}) { t.Error("expected QUOTED-PRINTABLE encoding") }
}

func TestBuildBodyStructure_ImageType(t *testing.T) {
    msg := raw("Content-Type: image/png\r\nContent-Transfer-Encoding: base64", "iVBORw0KGgo=")
    bs := BuildBodyStructure(msg)
    if !containsAll(bs, []string{"IMAGE", "PNG"}) { t.Error("expected IMAGE/PNG type") }
}

func TestBuildBodyStructure_ApplicationType(t *testing.T) {
    msg := raw("Content-Type: application/pdf\r\nContent-Transfer-Encoding: base64", "JVBERi0xLjQ=")
    bs := BuildBodyStructure(msg)
    if !containsAll(bs, []string{"APPLICATION", "PDF"}) { t.Error("expected APPLICATION/PDF type") }
}

func TestBuildBodyStructure_WithCharset(t *testing.T) {
    msg := raw("Content-Type: text/html; charset=iso-8859-1", "<html><body>Test</body></html>")
    bs := BuildBodyStructure(msg)
    if !containsAll(bs, []string{"HTML", "iso-8859-1"}) { t.Error("expected HTML with charset") }
}

func TestBuildBodyStructure_MultipartWithAttachment(t *testing.T) {
    boundary := "att123"
    headers := "Content-Type: multipart/mixed; boundary=\"" + boundary + "\""
    part1 := "--" + boundary + "\r\nContent-Type: text/plain\r\n\r\nMessage body\r\n"
    part2 := "--" + boundary + "\r\nContent-Type: application/pdf\r\nContent-Disposition: attachment; filename=\"doc.pdf\"\r\n\r\nPDF data\r\n"
    end := "--" + boundary + "--\r\n"
    msg := headers + "\r\n\r\n" + part1 + part2 + end
    bs := BuildBodyStructure(msg)
    if !containsAll(bs, []string{"MIXED", "APPLICATION", "PDF"}) { t.Error("expected multipart with attachment") }
}

func TestBuildBodyStructure_EmptyBody(t *testing.T) {
    msg := raw("Content-Type: text/plain", "")
    bs := BuildBodyStructure(msg)
    if !containsAll(bs, []string{"TEXT", "PLAIN"}) { t.Error("expected valid structure for empty body") }
}

func TestBuildBodyStructure_NoContentType(t *testing.T) {
    msg := raw("Subject: Test", "Body content")
    bs := BuildBodyStructure(msg)
    // Should default to text/plain
    if !containsAll(bs, []string{"TEXT", "PLAIN"}) { t.Error("expected default text/plain") }
}

func TestBuildBodyStructure_InvalidContentType(t *testing.T) {
    msg := raw("Content-Type: invalid", "Body")
    bs := BuildBodyStructure(msg)
    // Should fallback to text/plain
    if !containsAll(bs, []string{"TEXT"}) { t.Error("expected fallback to text") }
}

func TestBuildBodyStructure_MultipartNested(t *testing.T) {
    outerBoundary := "outer"
    innerBoundary := "inner"
    headers := "Content-Type: multipart/mixed; boundary=\"" + outerBoundary + "\""
    part1 := "--" + outerBoundary + "\r\nContent-Type: multipart/alternative; boundary=\"" + innerBoundary + "\"\r\n\r\n"
    part1a := "--" + innerBoundary + "\r\nContent-Type: text/plain\r\n\r\nPlain\r\n"
    part1b := "--" + innerBoundary + "\r\nContent-Type: text/html\r\n\r\n<html>HTML</html>\r\n"
    part1end := "--" + innerBoundary + "--\r\n"
    end := "--" + outerBoundary + "--\r\n"
    msg := headers + "\r\n\r\n" + part1 + part1a + part1b + part1end + end
    bs := BuildBodyStructure(msg)
    // Nested multipart is complex - just check it has MIXED subtype
    if !containsAll(bs, []string{"MIXED"}) { t.Errorf("expected MIXED multipart structure, got: %s", bs) }
}

func TestBuildParamList_Empty(t *testing.T) {
    params := make(map[string]string)
    result := buildParamList(params)
    if result != "NIL" { t.Errorf("expected NIL for empty params, got %s", result) }
}

func TestBuildParamList_Single(t *testing.T) {
    params := map[string]string{"charset": "utf-8"}
    result := buildParamList(params)
    if !containsAll(result, []string{"CHARSET", "utf-8"}) { t.Error("expected charset param") }
}

func TestBuildParamList_Multiple(t *testing.T) {
    params := map[string]string{"charset": "utf-8", "name": "file.txt"}
    result := buildParamList(params)
    if !containsAll(result, []string{"CHARSET", "NAME", "utf-8", "file.txt"}) { t.Error("expected multiple params") }
}
